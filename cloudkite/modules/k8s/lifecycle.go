package k8s

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// extractObjectMeta extracts kind, name, namespace, and generation from a Starlark object.
func extractObjectMeta(val starlark.Value) (kind, name, namespace string, gen int64, err error) {
	var u *unstructured.Unstructured
	switch v := val.(type) {
	case KubeObject:
		u, err = dictToUnstructured(v.ToDict())
	case *AttrDict:
		u, err = dictToUnstructured(v)
	case *starlark.Dict:
		u, err = dictToUnstructured(v)
	default:
		return "", "", "", 0, fmt.Errorf("expected k8s object or dict, got %s", val.Type())
	}
	if err != nil {
		return "", "", "", 0, err
	}
	return u.GetKind(), u.GetName(), u.GetNamespace(), u.GetGeneration(), nil
}

// isDeleting checks whether metadata.deletionTimestamp is set on the object.
// Signature: k8s.is_deleting(obj) -> bool
func (m *Module) isDeleting(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var objVal starlark.Value
	_ = filterKwargValue(kwargs, "obj", &objVal)
	if objVal == nil && len(args) > 0 {
		objVal = args[0]
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.is_deleting: missing required argument: obj")
	}

	var dt any
	switch v := objVal.(type) {
	case *AttrDict:
		if meta, ok := v.data["metadata"].(map[string]any); ok {
			dt = meta["deletionTimestamp"]
		}
	case *starlark.Dict:
		if metaVal, found, _ := v.Get(starlark.String("metadata")); found {
			if metaDict, ok := metaVal.(*starlark.Dict); ok {
				if dtVal, found, _ := metaDict.Get(starlark.String("deletionTimestamp")); found {
					dt = dtVal
				}
			}
		}
	default:
		return starlark.False, nil
	}

	if dt == nil || dt == starlark.None {
		return starlark.False, nil
	}
	if s, ok := dt.(string); ok && s != "" {
		return starlark.True, nil
	}
	if s, ok := dt.(starlark.String); ok && string(s) != "" {
		return starlark.True, nil
	}
	return starlark.False, nil
}

// finalizerHas checks whether a finalizer is present in metadata.finalizers.
// Signature: k8s.finalizer.has(obj, name) -> bool
func (m *Module) finalizerHas(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var objVal starlark.Value
	filtered := filterKwargValue(kwargs, "obj", &objVal)
	remaining := args
	if objVal == nil && len(remaining) > 0 {
		objVal = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Name string `name:"name" position:"0" required:"true"`
	}
	if err := startype.Args(remaining, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.finalizer.has: %w", err)
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.finalizer.has: missing required argument: obj")
	}

	finalizers := extractFinalizers(objVal)
	if slices.Contains(finalizers, p.Name) {
		return starlark.True, nil
	}
	return starlark.False, nil
}

// finalizerAdd adds a finalizer to the object in Kubernetes.
// Signature: k8s.finalizer.add(obj, name, namespace="")
func (c *K8sClient) finalizerAdd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var objVal starlark.Value
	filtered := filterKwargValue(kwargs, "obj", &objVal)
	remaining := args
	if objVal == nil && len(remaining) > 0 {
		objVal = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.finalizer.add: missing required argument: obj")
	}

	kind, name, ns, _, err := extractObjectMeta(objVal)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}
	if p.Namespace != "" {
		ns = p.Namespace
	}
	if ns == "" {
		ns = c.namespace
	}

	finalizers := extractFinalizers(objVal)
	if slices.Contains(finalizers, p.Name) {
		return objVal, nil // already present
	}
	finalizers = append(finalizers, p.Name)

	gvr, namespaced, err := c.resolver.Resolve(kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}
	defer cancel()

	patchData, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"finalizers": finalizers,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}

	var updated *unstructured.Unstructured
	if namespaced {
		updated, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	} else {
		updated, err = c.dynClient.Resource(gvr).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.add: %w", err)
	}

	if act, ok := thread.Local(ActiveControllerKey).(ActiveController); ok && act != nil {
		act.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
	}

	return unstructuredToDict(updated)
}

// finalizerRemove removes a finalizer from the object in Kubernetes.
// Signature: k8s.finalizer.remove(obj, name, namespace="")
func (c *K8sClient) finalizerRemove(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var objVal starlark.Value
	filtered := filterKwargValue(kwargs, "obj", &objVal)
	remaining := args
	if objVal == nil && len(remaining) > 0 {
		objVal = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: missing required argument: obj")
	}

	kind, name, ns, _, err := extractObjectMeta(objVal)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}
	if p.Namespace != "" {
		ns = p.Namespace
	}
	if ns == "" {
		ns = c.namespace
	}

	finalizers := extractFinalizers(objVal)
	var newFinalizers []string
	found := false
	for _, f := range finalizers {
		if f == p.Name {
			found = true
			continue
		}
		newFinalizers = append(newFinalizers, f)
	}
	if !found {
		return objVal, nil
	}

	gvr, namespaced, err := c.resolver.Resolve(kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}
	defer cancel()

	patchData, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"finalizers": newFinalizers,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}

	var updated *unstructured.Unstructured
	if namespaced {
		updated, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	} else {
		updated, err = c.dynClient.Resource(gvr).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.finalizer.remove: %w", err)
	}

	if act, ok := thread.Local(ActiveControllerKey).(ActiveController); ok && act != nil {
		act.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
	}

	return unstructuredToDict(updated)
}

func extractFinalizers(val starlark.Value) []string {
	var list []string
	switch v := val.(type) {
	case *AttrDict:
		if meta, ok := v.data["metadata"].(map[string]any); ok {
			if finList, ok := meta["finalizers"].([]any); ok {
				for _, f := range finList {
					if s, ok := f.(string); ok {
						list = append(list, s)
					}
				}
			}
		}
	case *starlark.Dict:
		if metaVal, found, _ := v.Get(starlark.String("metadata")); found {
			if metaDict, ok := metaVal.(*starlark.Dict); ok {
				if finVal, found, _ := metaDict.Get(starlark.String("finalizers")); found {
					if finList, ok := finVal.(*starlark.List); ok {
						for i := 0; i < finList.Len(); i++ {
							if s, ok := starlark.AsString(finList.Index(i)); ok {
								list = append(list, s)
							}
						}
					}
				}
			}
		}
	}
	return list
}

// conditionGet retrieves a specific condition by type from status.conditions.
// Signature: k8s.condition.get(obj, type) -> dict | None
func (m *Module) conditionGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var objVal starlark.Value
	filtered := filterKwargValue(kwargs, "obj", &objVal)
	remaining := args
	if objVal == nil && len(remaining) > 0 {
		objVal = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Type string `name:"type" position:"0" required:"true"`
	}
	if err := startype.Args(remaining, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.condition.get: %w", err)
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.condition.get: missing required argument: obj")
	}

	u, err := toUnstructuredObj(objVal)
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.get: %w", err)
	}

	conds, found, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found {
		return starlark.None, nil
	}

	for _, c := range conds {
		if cm, ok := c.(map[string]any); ok {
			if t, ok := cm["type"].(string); ok && strings.EqualFold(t, p.Type) {
				return NewAttrDict(cm), nil
			}
		}
	}
	return starlark.None, nil
}

// conditionSet updates or appends a condition in status.conditions.
// Signature: k8s.condition.set(obj, type, status="True", reason="", message="", observed_generation=True, namespace="")
func (c *K8sClient) conditionSet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var objVal starlark.Value
	filtered := filterKwargValue(kwargs, "obj", &objVal)
	remaining := args
	if objVal == nil && len(remaining) > 0 {
		objVal = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Type               string `name:"type" position:"0" required:"true"`
		Status             string `name:"status" position:"1"`
		Reason             string `name:"reason"`
		Message            string `name:"message"`
		ObservedGeneration *bool  `name:"observed_generation"`
		Namespace          string `name:"namespace"`
		Timeout            string `name:"timeout"`
	}
	p.Status = "True"
	if err := startype.Args(remaining, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.condition.set: %w", err)
	}
	if objVal == nil {
		return nil, fmt.Errorf("k8s.condition.set: missing required argument: obj")
	}

	observedGen := true
	if p.ObservedGeneration != nil {
		observedGen = *p.ObservedGeneration
	}

	kind, name, ns, gen, err := extractObjectMeta(objVal)
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.set: %w", err)
	}
	if p.Namespace != "" {
		ns = p.Namespace
	}
	if ns == "" {
		ns = c.namespace
	}

	gvr, namespaced, err := c.resolver.Resolve(kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.set: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.set: %w", err)
	}
	defer cancel()

	var current *unstructured.Unstructured
	if namespaced {
		current, err = c.dynClient.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	} else {
		current, err = c.dynClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.set: get %s %q: %w", kind, name, err)
	}

	rawConds, _, _ := unstructured.NestedSlice(current.Object, "status", "conditions")
	lastTransition := time.Now().UTC().Format(time.RFC3339)

	// Preserve lastTransitionTime if status unchanged
	for _, raw := range rawConds {
		if cm, ok := raw.(map[string]any); ok {
			if t, ok := cm["type"].(string); ok && strings.EqualFold(t, p.Type) {
				if s, ok := cm["status"].(string); ok && s == p.Status {
					if prevTime, ok := cm["lastTransitionTime"].(string); ok && prevTime != "" {
						lastTransition = prevTime
					}
				}
				break
			}
		}
	}

	newCond := map[string]any{
		"type":               p.Type,
		"status":             p.Status,
		"lastTransitionTime": lastTransition,
		"reason":             p.Reason,
		"message":            p.Message,
	}
	if observedGen {
		newCond["observedGeneration"] = gen
	}

	found := false
	for i, raw := range rawConds {
		if cm, ok := raw.(map[string]any); ok {
			if t, ok := cm["type"].(string); ok && strings.EqualFold(t, p.Type) {
				rawConds[i] = newCond
				found = true
				break
			}
		}
	}
	if !found {
		rawConds = append(rawConds, newCond)
	}

	_ = unstructured.SetNestedSlice(current.Object, rawConds, "status", "conditions")

	var updated *unstructured.Unstructured
	if namespaced {
		updated, err = c.dynClient.Resource(gvr).Namespace(ns).UpdateStatus(ctx, current, metav1.UpdateOptions{})
	} else {
		updated, err = c.dynClient.Resource(gvr).UpdateStatus(ctx, current, metav1.UpdateOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.condition.set: update %s %q: %w", kind, name, err)
	}

	if act, ok := thread.Local(ActiveControllerKey).(ActiveController); ok && act != nil {
		act.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
	}

	return unstructuredToDict(updated)
}

func toUnstructuredObj(val starlark.Value) (*unstructured.Unstructured, error) {
	switch v := val.(type) {
	case KubeObject:
		return dictToUnstructured(v.ToDict())
	case *AttrDict:
		return dictToUnstructured(v)
	case *starlark.Dict:
		return dictToUnstructured(v)
	default:
		return nil, fmt.Errorf("expected k8s object or dict, got %s", val.Type())
	}
}
