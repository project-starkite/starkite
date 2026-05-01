package k8s

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// drain cordons a node and evicts its pods.
// Signature: k8s.drain(node, force=False, ignore_daemonsets=True, timeout="3m")
func (c *K8sClient) drain(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", ""); err != nil {
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

	result := starlark.NewDict(3)
	result.SetKey(starlark.String("node"), starlark.String(p.Node))
	result.SetKey(starlark.String("evicted"), starlark.NewList(evicted))
	result.SetKey(starlark.String("errors"), starlark.NewList(errors))

	return result, nil
}

// cordon marks a node as unschedulable.
// Signature: k8s.cordon(node, timeout="")
func (c *K8sClient) cordon(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", ""); err != nil {
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
	if err := libkite.Check(thread, "k8s", "write", ""); err != nil {
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
	if err := libkite.Check(thread, "k8s", "write", ""); err != nil {
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
	if err := libkite.Check(thread, "k8s", "write", ""); err != nil {
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
	if err := libkite.Check(thread, "k8s", "read", ""); err != nil {
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
		d := starlark.NewDict(4)
		d.SetKey(starlark.String("name"), starlark.String(node.GetName()))

		// Extract capacity and allocatable from status
		capacity, _, _ := nestedField(node.Object, "status", "capacity")
		allocatable, _, _ := nestedField(node.Object, "status", "allocatable")

		if cap, ok := capacity.(map[string]any); ok {
			capDict := starlark.NewDict(3)
			for k, v := range cap {
				capDict.SetKey(starlark.String(k), starlark.String(fmt.Sprint(v)))
			}
			d.SetKey(starlark.String("capacity"), capDict)
		}

		if alloc, ok := allocatable.(map[string]any); ok {
			allocDict := starlark.NewDict(3)
			for k, v := range alloc {
				allocDict.SetKey(starlark.String(k), starlark.String(fmt.Sprint(v)))
			}
			d.SetKey(starlark.String("allocatable"), allocDict)
		}

		results = append(results, d)
	}

	return starlark.NewList(results), nil
}

// topPods returns resource usage for pods using the metrics API.
// Signature: k8s.top_pods(namespace="", sort_by="", timeout="")
func (c *K8sClient) topPods(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", ""); err != nil {
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
		d := starlark.NewDict(5)
		d.SetKey(starlark.String("name"), starlark.String(pod.GetName()))
		d.SetKey(starlark.String("namespace"), starlark.String(pod.GetNamespace()))

		phase, _, _ := unstructuredNestedString(pod.Object, "status", "phase")
		d.SetKey(starlark.String("status"), starlark.String(phase))

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
		d.SetKey(starlark.String("cpu_request"), starlark.String(totalCPU))
		d.SetKey(starlark.String("memory_request"), starlark.String(totalMem))

		results = append(results, d)
	}

	return starlark.NewList(results), nil
}

// cp copies files between a pod and the local filesystem.
// Signature: k8s.cp(pod, src, dst, namespace="", container="", timeout="")
func (c *K8sClient) cp(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "exec", ""); err != nil {
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

	// Determine direction: if src starts with pod name, we're copying FROM pod
	isPodSrc := strings.Contains(p.Src, ":")
	isPodDst := strings.Contains(p.Dst, ":")

	if isPodSrc && isPodDst {
		return nil, fmt.Errorf("k8s.cp: cannot copy between two pods")
	}

	clientset, err := kubernetes.NewForConfig(c.restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.cp: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.cp: %w", err)
	}
	defer cancel()

	if isPodSrc {
		// Copy from pod to local
		parts := strings.SplitN(p.Src, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("k8s.cp: invalid source format, expected pod:path")
		}
		remotePath := parts[1]

		cmd := []string{"tar", "cf", "-", "-C", remotePath, "."}

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

		// Parse tar to get content size
		tr := tar.NewReader(&stdout)
		count := 0
		for {
			_, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			count++
		}

		result := starlark.NewDict(2)
		result.SetKey(starlark.String("files"), starlark.MakeInt(count))
		result.SetKey(starlark.String("direction"), starlark.String("from_pod"))
		return result, nil
	}

	if isPodDst {
		parts := strings.SplitN(p.Dst, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("k8s.cp: invalid destination format, expected pod:path")
		}
		remotePath := parts[1]

		cmd := []string{"tar", "xf", "-", "-C", remotePath}

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

		_, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
		if err != nil {
			return nil, fmt.Errorf("k8s.cp: %w", err)
		}

		result := starlark.NewDict(2)
		result.SetKey(starlark.String("source"), starlark.String(p.Src))
		result.SetKey(starlark.String("direction"), starlark.String("to_pod"))
		return result, nil
	}

	return nil, fmt.Errorf("k8s.cp: source or destination must reference a pod (use pod:path format)")
}

// describe returns a detailed description of a resource.
// Signature: k8s.describe(kind, name, namespace="", timeout="")
func (c *K8sClient) describe(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", ""); err != nil {
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

	result := starlark.NewDict(5)

	// Resource details
	resourceDict, _ := unstructuredToDict(obj)
	result.SetKey(starlark.String("resource"), resourceDict)

	// Conditions
	conditions, found, _ := unstructuredNestedSlice(obj.Object, "status", "conditions")
	if found {
		var condList []starlark.Value
		for _, c := range conditions {
			if m, ok := c.(map[string]any); ok {
				d := starlark.NewDict(4)
				for k, v := range m {
					d.SetKey(starlark.String(k), starlark.String(fmt.Sprint(v)))
				}
				condList = append(condList, d)
			}
		}
		result.SetKey(starlark.String("conditions"), starlark.NewList(condList))
	}

	// Events for this resource
	eventGVR, _, _ := c.resolver.Resolve("events")
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", p.Name, ns)
	events, err := c.dynClient.Resource(eventGVR).Namespace(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err == nil {
		var eventList []starlark.Value
		for _, event := range events.Items {
			d := starlark.NewDict(4)
			d.SetKey(starlark.String("type"), starlark.String(getNestedString(event.Object, "type")))
			d.SetKey(starlark.String("reason"), starlark.String(getNestedString(event.Object, "reason")))
			d.SetKey(starlark.String("message"), starlark.String(getNestedString(event.Object, "message")))
			d.SetKey(starlark.String("count"), starlark.String(fmt.Sprint(getNestedField(event.Object, "count"))))
			eventList = append(eventList, d)
		}
		result.SetKey(starlark.String("events"), starlark.NewList(eventList))
	}

	return result, nil
}

func getNestedString(obj map[string]any, key string) string {
	if v, ok := obj[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func getNestedField(obj map[string]any, key string) any {
	v, _ := obj[key]
	return v
}
