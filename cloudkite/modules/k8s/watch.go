package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// watch watches Kubernetes resources for changes.
// Signature: k8s.watch(kind, namespace="", labels="", handler=None, timeout="")
// With handler: calls handler(event_type, resource_dict) for each event, stops on False return.
// Without handler: collects events into a list, returns on timeout.
func (c *K8sClient) watch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	// handler is starlark.Callable — extract from kwargs before startype
	var handler starlark.Callable
	filteredKwargs := filterKwargCallable(kwargs, "handler", &handler)

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Labels    string `name:"labels"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.watch: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	opts := metav1.ListOptions{}
	if p.Labels != "" {
		opts.LabelSelector = p.Labels
	}

	// Set up context with timeout
	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.watch: %w", err)
	}
	defer cancel()

	var watcher watch.Interface
	if namespaced {
		watcher, err = c.dynClient.Resource(gvr).Namespace(ns).Watch(ctx, opts)
	} else {
		watcher, err = c.dynClient.Resource(gvr).Watch(ctx, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.watch: %w", err)
	}
	defer watcher.Stop()

	if handler != nil {
		return c.watchWithHandler(thread, watcher, handler, ctx)
	}
	return c.watchCollect(watcher, ctx)
}

func (c *K8sClient) watchWithHandler(thread *starlark.Thread, watcher watch.Interface, handler starlark.Callable, ctx context.Context) (starlark.Value, error) {
	count := 0
	for {
		select {
		case <-ctx.Done():
			return starlark.MakeInt(count), nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return starlark.MakeInt(count), nil
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			dict, err := unstructuredToDict(obj)
			if err != nil {
				continue
			}

			result, err := starlark.Call(thread, handler, starlark.Tuple{
				starlark.String(string(event.Type)),
				dict,
			}, nil)
			if err != nil {
				return nil, fmt.Errorf("k8s.watch: handler: %w", err)
			}

			count++

			// Stop if handler returns False
			if b, ok := result.(starlark.Bool); ok && !bool(b) {
				return starlark.MakeInt(count), nil
			}
		}
	}
}

func (c *K8sClient) watchCollect(watcher watch.Interface, ctx context.Context) (starlark.Value, error) {
	var events []starlark.Value
	for {
		select {
		case <-ctx.Done():
			return starlark.NewList(events), nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return starlark.NewList(events), nil
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			dict, err := unstructuredToDict(obj)
			if err != nil {
				continue
			}

			eventDict := starlark.NewDict(2)
			eventDict.SetKey(starlark.String("type"), starlark.String(string(event.Type)))
			eventDict.SetKey(starlark.String("object"), dict)
			events = append(events, eventDict)
		}
	}
}

// waitFor waits for a Kubernetes resource to reach a specified condition.
// Signature: k8s.wait_for(kind, name, condition="ready", namespace="", timeout="3m")
// Returns: {"ready": bool, "resource": dict, "message": string}
func (c *K8sClient) waitFor(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Condition string `name:"condition"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	p.Timeout = "3m" // default
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Condition == "" {
		p.Condition = "ready"
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.wait_for: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.wait_for: %w", err)
	}
	defer cancel()

	// First check current state
	var obj *unstructuredObj
	if namespaced {
		obj, err = c.dynClient.Resource(gvr).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
	} else {
		obj, err = c.dynClient.Resource(gvr).Get(ctx, p.Name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.wait_for: %w", err)
	}

	if met, msg := checkCondition(obj, p.Condition); met {
		return waitResult(true, obj, msg)
	}

	// Watch for changes
	opts := metav1.ListOptions{
		FieldSelector:   fmt.Sprintf("metadata.name=%s", p.Name),
		ResourceVersion: obj.GetResourceVersion(),
	}

	var watcher watch.Interface
	if namespaced {
		watcher, err = c.dynClient.Resource(gvr).Namespace(ns).Watch(ctx, opts)
	} else {
		watcher, err = c.dynClient.Resource(gvr).Watch(ctx, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.wait_for: watch: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			dict, _ := unstructuredToDict(obj)
			result := starlark.NewDict(3)
			result.SetKey(starlark.String("ready"), starlark.False)
			result.SetKey(starlark.String("resource"), dict)
			result.SetKey(starlark.String("message"), starlark.String("timeout waiting for condition"))
			return result, nil

		case event, ok := <-watcher.ResultChan():
			if !ok {
				return waitResult(false, obj, "watch channel closed")
			}

			if event.Type == watch.Deleted {
				if p.Condition == "deleted" || p.Condition == "delete" {
					return waitResult(true, obj, "resource deleted")
				}
				return waitResult(false, obj, "resource was deleted")
			}

			updated, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}
			obj = updated

			if met, msg := checkCondition(obj, p.Condition); met {
				return waitResult(true, obj, msg)
			}
		}
	}
}

// checkCondition checks whether the given condition is met on the object.
func checkCondition(obj *unstructuredObj, condition string) (bool, string) {
	conditions, found, err := unstructuredNestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false, "no conditions found"
	}

	conditionType := mapConditionName(condition)

	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := cond["type"].(string); strings.EqualFold(t, conditionType) {
			status, _ := cond["status"].(string)
			message, _ := cond["message"].(string)
			if status == "True" {
				return true, message
			}
			return false, message
		}
	}

	// Check phase for pods
	phase, found, _ := unstructuredNestedString(obj.Object, "status", "phase")
	if found {
		switch strings.ToLower(condition) {
		case "ready", "running":
			if phase == "Running" || phase == "Succeeded" {
				return true, fmt.Sprintf("phase: %s", phase)
			}
		case "complete", "completed", "succeeded":
			if phase == "Succeeded" {
				return true, fmt.Sprintf("phase: %s", phase)
			}
		}
	}

	return false, "condition not met"
}

func mapConditionName(condition string) string {
	switch strings.ToLower(condition) {
	case "ready":
		return "Ready"
	case "available":
		return "Available"
	case "complete", "completed":
		return "Complete"
	case "progressing":
		return "Progressing"
	case "initialized":
		return "Initialized"
	case "podscheduled":
		return "PodScheduled"
	default:
		return condition
	}
}

func waitResult(ready bool, obj *unstructuredObj, message string) (starlark.Value, error) {
	dict, _ := unstructuredToDict(obj)
	result := starlark.NewDict(3)
	result.SetKey(starlark.String("ready"), starlark.Bool(ready))
	if dict != nil {
		result.SetKey(starlark.String("resource"), dict)
	} else {
		result.SetKey(starlark.String("resource"), starlark.None)
	}
	result.SetKey(starlark.String("message"), starlark.String(message))
	return result, nil
}

// unstructuredNestedSlice extracts a nested slice from an unstructured object.
func unstructuredNestedSlice(obj map[string]any, fields ...string) ([]any, bool, error) {
	val, found, err := nestedField(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	s, ok := val.([]any)
	if !ok {
		return nil, false, fmt.Errorf("expected slice, got %T", val)
	}
	return s, true, nil
}

// unstructuredNestedString extracts a nested string from an unstructured object.
func unstructuredNestedString(obj map[string]any, fields ...string) (string, bool, error) {
	val, found, err := nestedField(obj, fields...)
	if !found || err != nil {
		return "", found, err
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("expected string, got %T", val)
	}
	return s, true, nil
}

func nestedField(obj map[string]any, fields ...string) (any, bool, error) {
	var val any = obj
	for _, f := range fields {
		m, ok := val.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		val, ok = m[f]
		if !ok {
			return nil, false, nil
		}
	}
	return val, true, nil
}
