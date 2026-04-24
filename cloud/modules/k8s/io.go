package k8s

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
)

// logs retrieves logs from a pod.
// Signature: k8s.logs(name, namespace="", container="", tail=0, since="", previous=False, timeout="")
func (c *K8sClient) logs(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Tail      int    `name:"tail"`
		Since     string `name:"since"`
		Previous  bool   `name:"previous"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	clientset, err := kubernetes.NewForConfig(c.restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs: %w", err)
	}

	opts := &corev1.PodLogOptions{
		Previous: p.Previous,
	}
	if p.Container != "" {
		opts.Container = p.Container
	}
	if p.Tail > 0 {
		lines := int64(p.Tail)
		opts.TailLines = &lines
	}
	if p.Since != "" {
		d, err := time.ParseDuration(p.Since)
		if err != nil {
			return nil, fmt.Errorf("k8s.logs: invalid since %q: %w", p.Since, err)
		}
		secs := int64(d.Seconds())
		opts.SinceSeconds = &secs
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs: %w", err)
	}
	defer cancel()

	req := clientset.CoreV1().Pods(ns).GetLogs(p.Name, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs: %w", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return nil, fmt.Errorf("k8s.logs: read: %w", err)
	}

	return starlark.String(buf.String()), nil
}

// logsFollow streams logs from a pod, calling handler for each line.
// Signature: k8s.logs_follow(name, namespace="", container="", handler=None, tail=0, timeout="")
func (c *K8sClient) logsFollow(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
		return nil, err
	}

	// handler is starlark.Callable — extract from kwargs before startype
	var handler starlark.Callable
	filteredKwargs := filterKwargCallable(kwargs, "handler", &handler)

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Tail      int    `name:"tail"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	if handler == nil {
		return nil, fmt.Errorf("k8s.logs_follow: handler is required")
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	clientset, err := kubernetes.NewForConfig(c.restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs_follow: %w", err)
	}

	logOpts := &corev1.PodLogOptions{
		Follow: true,
	}
	if p.Container != "" {
		logOpts.Container = p.Container
	}
	if p.Tail > 0 {
		lines := int64(p.Tail)
		logOpts.TailLines = &lines
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs_follow: %w", err)
	}
	defer cancel()

	req := clientset.CoreV1().Pods(ns).GetLogs(p.Name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("k8s.logs_follow: %w", err)
	}
	defer stream.Close()

	count := 0
	readBuf := make([]byte, 4096)
	var lineBuf bytes.Buffer

	for {
		n, readErr := stream.Read(readBuf)
		if n > 0 {
			lineBuf.Write(readBuf[:n])
			for {
				line, err := lineBuf.ReadString('\n')
				if err != nil {
					// Put back the partial line
					lineBuf.Reset()
					lineBuf.WriteString(line)
					break
				}
				line = strings.TrimRight(line, "\n")

				result, callErr := starlark.Call(thread, handler, starlark.Tuple{starlark.String(line)}, nil)
				if callErr != nil {
					return nil, fmt.Errorf("k8s.logs_follow: handler: %w", callErr)
				}
				count++

				if b, ok := result.(starlark.Bool); ok && !bool(b) {
					return starlark.MakeInt(count), nil
				}
			}
		}
		if readErr == io.EOF {
			// Flush remaining
			if lineBuf.Len() > 0 {
				starlark.Call(thread, handler, starlark.Tuple{starlark.String(lineBuf.String())}, nil)
				count++
			}
			return starlark.MakeInt(count), nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("k8s.logs_follow: read: %w", readErr)
		}
	}
}

// execCmd executes a command in a pod container.
// Signature: k8s.exec(name, command, namespace="", container="", timeout="")
// Returns: {"stdout": string, "stderr": string, "code": int}
func (c *K8sClient) execCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "exec", ""); err != nil {
		return nil, err
	}

	// command is starlark.Value at position 1 — extract before startype
	var command starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "command", &command)
	remaining := args
	if command == nil && len(args) > 1 {
		command = args[1]
		remaining = args[:1]
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if command == nil {
		return nil, fmt.Errorf("k8s.exec: missing required argument: command")
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	// Parse command: string → /bin/sh -c, list → direct
	var cmd []string
	switch v := command.(type) {
	case starlark.String:
		cmd = []string{"/bin/sh", "-c", string(v)}
	case *starlark.List:
		for i := 0; i < v.Len(); i++ {
			s, ok := starlark.AsString(v.Index(i))
			if !ok {
				return nil, fmt.Errorf("k8s.exec: command list elements must be strings")
			}
			cmd = append(cmd, s)
		}
	default:
		return nil, fmt.Errorf("k8s.exec: command must be string or list, got %s", command.Type())
	}

	clientset, err := kubernetes.NewForConfig(c.restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.exec: %w", err)
	}

	execOpts := &corev1.PodExecOptions{
		Command: cmd,
		Stdout:  true,
		Stderr:  true,
	}
	if p.Container != "" {
		execOpts.Container = p.Container
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(p.Name).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(execOpts, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("k8s.exec: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.exec: %w", err)
	}
	defer cancel()

	var stdout, stderr bytes.Buffer
	streamErr := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	exitCode := 0
	if streamErr != nil {
		exitCode = 1
	}

	result := starlark.NewDict(3)
	result.SetKey(starlark.String("stdout"), starlark.String(stdout.String()))
	result.SetKey(starlark.String("stderr"), starlark.String(stderr.String()))
	result.SetKey(starlark.String("code"), starlark.MakeInt(exitCode))

	return result, nil
}

// portForward establishes a port-forward to a pod.
// Signature: k8s.port_forward(name, port, local_port=0, namespace="")
// No timeout — long-lived by nature, uses stop() to terminate.
// Returns: PortForwardHandle with local_port and stop()
func (c *K8sClient) portForward(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "port_forward", ""); err != nil {
		return nil, err
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Port      int    `name:"port" position:"1" required:"true"`
		LocalPort int    `name:"local_port"`
		Namespace string `name:"namespace"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	localPort := p.LocalPort
	if localPort == 0 {
		localPort = p.Port
	}

	transport, upgrader, err := spdy.RoundTripperFor(c.restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.port_forward: %w", err)
	}

	reqURL, err := url.Parse(c.restCfg.Host + fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", ns, p.Name))
	if err != nil {
		return nil, fmt.Errorf("k8s.port_forward: invalid URL: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", reqURL)

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})

	ports := []string{fmt.Sprintf("%d:%d", localPort, p.Port)}

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("k8s.port_forward: %w", err)
	}

	// Start port forwarding in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- fw.ForwardPorts()
	}()

	// Wait for ready or error
	select {
	case <-readyCh:
		actualPorts, err := fw.GetPorts()
		if err == nil && len(actualPorts) > 0 {
			localPort = int(actualPorts[0].Local)
		}
	case err := <-errCh:
		return nil, fmt.Errorf("k8s.port_forward: %w", err)
	}

	return &PortForwardHandle{
		localPort: localPort,
		stopCh:    stopCh,
	}, nil
}
