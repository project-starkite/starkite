package k8s

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// drain cordons a node and evicts its pods.
// Signature: k8s.drain(node, force=False, ignore_daemonsets=True, timeout="3m")
func (c *K8sClient) drain(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Node             string `name:"node" position:"0" required:"true"`
		Force            bool   `name:"force"`
		IgnoreDaemonsets bool   `name:"ignore_daemonsets"`
		Timeout          string `name:"timeout"`
	}
	p.Timeout = "3m" // default
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.drain: %w", err)
	}
	defer cancel()

	// Step 1: Cordon the node
	patchObj := map[string]any{
		"spec": map[string]any{"unschedulable": true},
	}
	patchBytes, _ := json.Marshal(patchObj)

	nodeGVR, _, _ := c.resolver.Resolve("node")
	_, err = c.dynClient.Resource(nodeGVR).Patch(ctx, p.Node, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.drain: cordon %q: %w", p.Node, err)
	}

	// Step 2: List pods on the node
	podGVR, _, _ := c.resolver.Resolve("pod")
	podList, err := c.dynClient.Resource(podGVR).Namespace("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", p.Node),
	})
	if err != nil {
		return nil, fmt.Errorf("k8s.drain: list pods: %w", err)
	}

	var evicted []starlark.Value
	var errors []starlark.Value

	for _, pod := range podList.Items {
		podName := pod.GetName()
		podNs := pod.GetNamespace()

		// Skip DaemonSet pods if ignoreDaemonsets
		if p.IgnoreDaemonsets {
			ownerRefs, _, _ := unstructuredNestedSlice(pod.Object, "metadata", "ownerReferences")
			isDaemonSet := false
			for _, ref := range ownerRefs {
				if m, ok := ref.(map[string]any); ok {
					if kind, _ := m["kind"].(string); kind == "DaemonSet" {
						isDaemonSet = true
						break
					}
				}
			}
			if isDaemonSet {
				continue
			}
		}

		// Evict the pod
		err := c.dynClient.Resource(podGVR).Namespace(podNs).Delete(ctx, podName, metav1.DeleteOptions{})
		if err != nil {
			errors = append(errors, starlark.String(fmt.Sprintf("%s/%s: %v", podNs, podName, err)))
		} else {
			evicted = append(evicted, starlark.String(podNs+"/"+podName))
		}
	}

	return NewAttrDict(map[string]any{
		"node":    p.Node,
		"evicted": evicted,
		"errors":  errors,
	}), nil
}

// cordon marks a node as unschedulable.
// Signature: k8s.cordon(node, timeout="")
func (c *K8sClient) cordon(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Node    string `name:"node" position:"0" required:"true"`
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	patchObj := map[string]any{
		"spec": map[string]any{"unschedulable": true},
	}
	patchBytes, _ := json.Marshal(patchObj)

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.cordon: %w", err)
	}
	defer cancel()

	nodeGVR, _, _ := c.resolver.Resolve("node")
	patched, err := c.dynClient.Resource(nodeGVR).Patch(ctx, p.Node, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.cordon: %w", err)
	}

	return unstructuredToDict(patched)
}

// uncordon marks a node as schedulable.
// Signature: k8s.uncordon(node, timeout="")
func (c *K8sClient) uncordon(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Node    string `name:"node" position:"0" required:"true"`
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	patchObj := map[string]any{
		"spec": map[string]any{"unschedulable": false},
	}
	patchBytes, _ := json.Marshal(patchObj)

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.uncordon: %w", err)
	}
	defer cancel()

	nodeGVR, _, _ := c.resolver.Resolve("node")
	patched, err := c.dynClient.Resource(nodeGVR).Patch(ctx, p.Node, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.uncordon: %w", err)
	}

	return unstructuredToDict(patched)
}

// taint adds a taint to a node.
// Signature: k8s.taint(node, key, value="", effect="NoSchedule", timeout="")
func (c *K8sClient) taint(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Node    string `name:"node" position:"0" required:"true"`
		Key     string `name:"key" position:"1" required:"true"`
		Value   string `name:"value"`
		Effect  string `name:"effect"`
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	effect := p.Effect
	if effect == "" {
		effect = "NoSchedule"
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.taint: %w", err)
	}
	defer cancel()

	nodeGVR, _, _ := c.resolver.Resolve("node")

	// Get current node to read existing taints
	obj, err := c.dynClient.Resource(nodeGVR).Get(ctx, p.Node, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.taint: get node: %w", err)
	}

	taints, _, _ := unstructuredNestedSlice(obj.Object, "spec", "taints")
	taints = append(taints, map[string]any{
		"key":    p.Key,
		"value":  p.Value,
		"effect": effect,
	})

	patchObj := map[string]any{
		"spec": map[string]any{"taints": taints},
	}
	patchBytes, _ := json.Marshal(patchObj)

	patched, err := c.dynClient.Resource(nodeGVR).Patch(ctx, p.Node, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.taint: %w", err)
	}

	return unstructuredToDict(patched)
}

// untaint removes a taint from a node.
// Signature: k8s.untaint(node, key, timeout="")
func (c *K8sClient) untaint(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Node    string `name:"node" position:"0" required:"true"`
		Key     string `name:"key" position:"1" required:"true"`
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.untaint: %w", err)
	}
	defer cancel()

	nodeGVR, _, _ := c.resolver.Resolve("node")

	obj, err := c.dynClient.Resource(nodeGVR).Get(ctx, p.Node, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.untaint: get node: %w", err)
	}

	taints, _, _ := unstructuredNestedSlice(obj.Object, "spec", "taints")
	var newTaints []any
	for _, t := range taints {
		if m, ok := t.(map[string]any); ok {
			if k, _ := m["key"].(string); k != p.Key {
				newTaints = append(newTaints, t)
			}
		}
	}

	patchObj := map[string]any{
		"spec": map[string]any{"taints": newTaints},
	}
	patchBytes, _ := json.Marshal(patchObj)

	patched, err := c.dynClient.Resource(nodeGVR).Patch(ctx, p.Node, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.untaint: %w", err)
	}

	return unstructuredToDict(patched)
}

// topNodes returns resource usage for nodes using the metrics API.
// Signature: k8s.top_nodes(timeout="")
func (c *K8sClient) topNodes(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.top_nodes: %w", err)
	}
	defer cancel()

	// Fall back to getting node status if metrics API is not available
	nodeGVR, _, _ := c.resolver.Resolve("node")
	nodeList, err := c.dynClient.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.top_nodes: %w", err)
	}

	var results []starlark.Value
	for _, node := range nodeList.Items {
		var capMap, allocMap map[string]any
		capacity, _, _ := nestedField(node.Object, "status", "capacity")
		if cap, ok := capacity.(map[string]any); ok {
			capMap = cap
		}
		allocatable, _, _ := nestedField(node.Object, "status", "allocatable")
		if alloc, ok := allocatable.(map[string]any); ok {
			allocMap = alloc
		}

		d := NewAttrDict(map[string]any{
			"name":        node.GetName(),
			"capacity":    capMap,
			"allocatable": allocMap,
		})
		results = append(results, d)
	}

	return starlark.NewList(results), nil
}

// topPods returns resource usage for pods using the metrics API.
// Signature: k8s.top_pods(namespace="", sort_by="", timeout="")
func (c *K8sClient) topPods(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Namespace string `name:"namespace"`
		SortBy    string `name:"sort_by"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.top_pods: %w", err)
	}
	defer cancel()

	// List pods and their resource requests as a starting point
	podGVR, _, _ := c.resolver.Resolve("pod")
	podList, err := c.dynClient.Resource(podGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.top_pods: %w", err)
	}

	var results []starlark.Value
	for _, pod := range podList.Items {
		phase, _, _ := unstructuredNestedString(pod.Object, "status", "phase")

		// Extract resource requests from containers
		containers, _, _ := unstructuredNestedSlice(pod.Object, "spec", "containers")
		var totalCPU, totalMem string
		for _, c := range containers {
			if cm, ok := c.(map[string]any); ok {
				if resources, ok := cm["resources"].(map[string]any); ok {
					if requests, ok := resources["requests"].(map[string]any); ok {
						if cpu, ok := requests["cpu"].(string); ok {
							totalCPU = cpu
						}
						if mem, ok := requests["memory"].(string); ok {
							totalMem = mem
						}
					}
				}
			}
		}

		d := NewAttrDict(map[string]any{
			"name":           pod.GetName(),
			"namespace":      pod.GetNamespace(),
			"status":         phase,
			"cpu_request":    totalCPU,
			"memory_request": totalMem,
		})
		results = append(results, d)
	}

	return starlark.NewList(results), nil
}

// cp copies files between a pod and the local filesystem.
// Signature: k8s.cp(pod, src, dst, namespace="", container="", timeout="")
func (c *K8sClient) cp(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "exec", "exec", ""); err != nil {
		return nil, err
	}

	var p struct {
		Pod       string `name:"pod" position:"0" required:"true"`
		Src       string `name:"src" position:"1" required:"true"`
		Dst       string `name:"dst" position:"2" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	// Determine direction: a colon in src marks it as the pod-side path (copying FROM pod)
	isPodSrc := strings.Contains(p.Src, ":")
	isPodDst := strings.Contains(p.Dst, ":")

	if isPodSrc && isPodDst {
		return nil, fmt.Errorf("k8s.cp: cannot copy between two pods")
	}

	if isPodSrc {
		localDst := libkite.CanonicalizePath(p.Dst)
		if err := libkite.Check(thread, "fs", "write", "cp", localDst); err != nil {
			return nil, err
		}
		if c.config != nil && c.config.DryRun {
			return NewAttrDict(map[string]any{
				"files":     0,
				"bytes":     int64(0),
				"direction": "from_pod",
				"dry_run":   true,
			}), nil
		}

		parts := strings.SplitN(p.Src, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, fmt.Errorf("k8s.cp: invalid source format, expected pod:path")
		}
		remotePath := parts[1]

		clientset, err := c.getClientset()
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}

		ctx, cancel, err := c.contextWithTimeout(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}
		defer cancel()

		cleanRemote := path.Clean(remotePath)
		remoteDir := path.Dir(cleanRemote)
		remoteBase := path.Base(cleanRemote)
		cmd := []string{"tar", "cf", "-", "-C", remoteDir, remoteBase}

		execOpts := &corev1.PodExecOptions{
			Command: cmd,
			Stdout:  true,
			Stderr:  true,
		}
		if p.Container != "" {
			execOpts.Container = p.Container
		}

		req := clientset.CoreV1().RESTClient().Post().
			Resource("pods").Name(p.Pod).Namespace(ns).
			SubResource("exec").
			VersionedParams(execOpts, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: exec: %w (%s)", err, stderr.String())
		}

		count, totalBytes, err := extractTarToLocal(&stdout, localDst)
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: extract: %w", err)
		}

		return NewAttrDict(map[string]any{
			"files":     count,
			"bytes":     totalBytes,
			"direction": "from_pod",
		}), nil
	}

	if isPodDst {
		localSrc := libkite.CanonicalizePath(p.Src)
		if err := libkite.Check(thread, "fs", "read", "cp", localSrc); err != nil {
			return nil, err
		}
		if _, err := os.Stat(localSrc); err != nil {
			return nil, fmt.Errorf("k8s.cp: local source: %w", err)
		}
		if c.config != nil && c.config.DryRun {
			return NewAttrDict(map[string]any{
				"source":    p.Src,
				"files":     0,
				"bytes":     int64(0),
				"direction": "to_pod",
				"dry_run":   true,
			}), nil
		}

		parts := strings.SplitN(p.Dst, ":", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, fmt.Errorf("k8s.cp: invalid destination format, expected pod:path")
		}
		remotePath := parts[1]

		clientset, err := c.getClientset()
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}

		ctx, cancel, err := c.contextWithTimeout(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}
		defer cancel()

		cleanRemote := path.Clean(remotePath)
		cmd := []string{"tar", "xf", "-", "-C", cleanRemote}

		execOpts := &corev1.PodExecOptions{
			Command: cmd,
			Stdin:   true,
			Stdout:  true,
			Stderr:  true,
		}
		if p.Container != "" {
			execOpts.Container = p.Container
		}

		req := clientset.CoreV1().RESTClient().Post().
			Resource("pods").Name(p.Pod).Namespace(ns).
			SubResource("exec").
			VersionedParams(execOpts, scheme.ParameterCodec)

		exec, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}

		pr, pw := io.Pipe()
		var fileCount int
		var totalBytes int64
		go func() {
			fc, tb, packErr := packLocalToTar(localSrc, pw)
			fileCount = fc
			totalBytes = tb
			if packErr != nil {
				_ = pw.CloseWithError(packErr)
			} else {
				_ = pw.Close()
			}
		}()

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  pr,
			Stdout: &stdout,
			Stderr: &stderr,
		})
		_ = pr.Close()
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: exec upload: %w (%s)", err, stderr.String())
		}

		return NewAttrDict(map[string]any{
			"source":    p.Src,
			"files":     fileCount,
			"bytes":     totalBytes,
			"direction": "to_pod",
		}), nil
	}

	return nil, fmt.Errorf("k8s.cp: source or destination must reference a pod (use pod:path format)")
}

// extractTarToLocal safely extracts a tar stream into localDst, preventing tar-slip attacks.
func extractTarToLocal(r io.Reader, localDst string) (int, int64, error) {
	tr := tar.NewReader(r)
	count := 0
	var totalBytes int64

	dstClean := filepath.Clean(localDst)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, totalBytes, fmt.Errorf("reading tar stream: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if cleanName == "." || cleanName == "/" {
			continue
		}
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return count, totalBytes, fmt.Errorf("insecure path in tar archive: %q", hdr.Name)
		}

		targetPath := filepath.Join(dstClean, cleanName)
		rel, err := filepath.Rel(dstClean, targetPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return count, totalBytes, fmt.Errorf("insecure path escaping destination: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return count, totalBytes, fmt.Errorf("creating directory %q: %w", targetPath, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return count, totalBytes, fmt.Errorf("creating parent directory for %q: %w", targetPath, err)
			}
			perm := hdr.FileInfo().Mode().Perm()
			if perm == 0 {
				perm = 0644
			}
			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				return count, totalBytes, fmt.Errorf("creating destination file %q: %w", targetPath, err)
			}
			written, err := io.Copy(f, tr)
			f.Close()
			if err != nil {
				return count, totalBytes, fmt.Errorf("writing destination file %q: %w", targetPath, err)
			}
			count++
			totalBytes += written
		case tar.TypeSymlink, tar.TypeLink:
			if filepath.IsAbs(hdr.Linkname) || strings.HasPrefix(filepath.Clean(hdr.Linkname), "..") {
				return count, totalBytes, fmt.Errorf("insecure link in tar archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
		}
	}
	return count, totalBytes, nil
}

// packLocalToTar streams the local file or directory at localSrc into pw as a tar archive.
func packLocalToTar(localSrc string, pw *io.PipeWriter) (int, int64, error) {
	tw := tar.NewWriter(pw)
	srcClean := filepath.Clean(localSrc)

	fi, err := os.Stat(srcClean)
	if err != nil {
		_ = tw.Close()
		return 0, 0, fmt.Errorf("stat local source %q: %w", srcClean, err)
	}

	count := 0
	var totalBytes int64

	if !fi.IsDir() {
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			_ = tw.Close()
			return 0, 0, err
		}
		hdr.Name = filepath.Base(srcClean)
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			return 0, 0, err
		}
		f, err := os.Open(srcClean)
		if err != nil {
			_ = tw.Close()
			return 0, 0, err
		}
		written, copyErr := io.Copy(tw, f)
		f.Close()
		if copyErr != nil {
			_ = tw.Close()
			return 0, 0, copyErr
		}
		count++
		totalBytes += written
	} else {
		walkErr := filepath.Walk(srcClean, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(srcClean, p)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f, err := os.Open(p)
				if err != nil {
					return err
				}
				written, copyErr := io.Copy(tw, f)
				f.Close()
				if copyErr != nil {
					return copyErr
				}
				count++
				totalBytes += written
			}
			return nil
		})
		if walkErr != nil {
			_ = tw.Close()
			return count, totalBytes, walkErr
		}
	}

	if err := tw.Close(); err != nil {
		return count, totalBytes, err
	}
	return count, totalBytes, nil
}

// describe returns a detailed description of a resource.
// Signature: k8s.describe(kind, name, namespace="", timeout="")
func (c *K8sClient) describe(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.describe: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.describe: %w", err)
	}
	defer cancel()

	// Get the resource
	var obj *unstructuredObj
	if namespaced {
		obj, err = c.dynClient.Resource(gvr).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
	} else {
		obj, err = c.dynClient.Resource(gvr).Get(ctx, p.Name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.describe: %w", err)
	}

	// Conditions
	var condList []starlark.Value
	conditions, found, _ := unstructuredNestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, c := range conditions {
			if m, ok := c.(map[string]any); ok {
				condList = append(condList, NewAttrDict(m))
			}
		}
	}

	// Events for this resource
	var eventList []starlark.Value
	eventGVR, _, _ := c.resolver.Resolve("events")
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", p.Name, ns)
	events, err := c.dynClient.Resource(eventGVR).Namespace(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err == nil {
		for _, event := range events.Items {
			d := NewAttrDict(map[string]any{
				"type":    getNestedString(event.Object, "type"),
				"reason":  getNestedString(event.Object, "reason"),
				"message": getNestedString(event.Object, "message"),
				"count":   fmt.Sprint(getNestedField(event.Object, "count")),
			})
			eventList = append(eventList, d)
		}
	}

	resDict := unstructuredToAttrDict(obj)
	return NewAttrDict(map[string]any{
		"resource":   resDict,
		"conditions": condList,
		"events":     eventList,
	}), nil
}

func getNestedString(obj map[string]any, key string) string {
	if v, ok := obj[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func getNestedField(obj map[string]any, key string) any {
	return obj[key]
}
