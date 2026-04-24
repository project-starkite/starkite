package k8s

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// get retrieves a single Kubernetes resource.
// Signature: k8s.get(kind, name, namespace="", timeout="")
func (c *K8sClient) get(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
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
		return nil, fmt.Errorf("k8s.get: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.get: %w", err)
	}
	defer cancel()

	var result any
	if namespaced {
		obj, err := c.dynClient.Resource(gvr).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.get: %w", err)
		}
		result = obj
	} else {
		obj, err := c.dynClient.Resource(gvr).Get(ctx, p.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.get: %w", err)
		}
		result = obj
	}

	return unstructuredToDict(result.(*unstructuredObj))
}

// listResources retrieves a list of Kubernetes resources.
// Signature: k8s.list(kind, namespace="", labels="", fields="", timeout="")
func (c *K8sClient) listResources(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		Labels    string `name:"labels"`
		Fields    string `name:"fields"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.list: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	opts := metav1.ListOptions{}
	if p.Labels != "" {
		opts.LabelSelector = p.Labels
	}
	if p.Fields != "" {
		opts.FieldSelector = p.Fields
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.list: %w", err)
	}
	defer cancel()

	var list *unstructuredList
	if namespaced {
		list, err = c.dynClient.Resource(gvr).Namespace(ns).List(ctx, opts)
	} else {
		list, err = c.dynClient.Resource(gvr).List(ctx, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.list: %w", err)
	}

	items := make([]starlark.Value, 0, len(list.Items))
	for i := range list.Items {
		dict, err := unstructuredToDict(&list.Items[i])
		if err != nil {
			return nil, fmt.Errorf("k8s.list: item %d: %w", i, err)
		}
		items = append(items, dict)
	}

	return starlark.NewList(items), nil
}

// create creates a new Kubernetes resource.
// Signature: k8s.create(manifest, namespace="", dry_run=False, timeout="")
func (c *K8sClient) create(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// manifest is starlark.Value at position 0 — extract before startype
	var manifest starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "manifest", &manifest)
	remaining := args
	if manifest == nil && len(args) > 0 {
		manifest = args[0]
		remaining = args[1:]
	}

	var p struct {
		Namespace string `name:"namespace"`
		DryRun    bool   `name:"dry_run"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, fmt.Errorf("k8s.create: missing required argument: manifest")
	}

	objs, err := parseManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("k8s.create: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.create: %w", err)
	}
	defer cancel()

	opts := metav1.CreateOptions{}
	if p.DryRun || (c.config != nil && c.config.DryRun) {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	results := make([]starlark.Value, 0, len(objs))
	for _, obj := range objs {
		gvk := obj.GroupVersionKind()
		gvr, namespaced, err := c.resolver.Resolve(gvk.Kind)
		if err != nil {
			return nil, fmt.Errorf("k8s.create: %w", err)
		}

		objNs := ns
		if n := obj.GetNamespace(); n != "" {
			objNs = n
		}

		var created *unstructuredObj
		if namespaced {
			created, err = c.dynClient.Resource(gvr).Namespace(objNs).Create(ctx, obj, opts)
		} else {
			created, err = c.dynClient.Resource(gvr).Create(ctx, obj, opts)
		}
		if err != nil {
			return nil, fmt.Errorf("k8s.create: %s %q: %w", gvk.Kind, obj.GetName(), err)
		}

		dict, err := unstructuredToDict(created)
		if err != nil {
			return nil, fmt.Errorf("k8s.create: convert result: %w", err)
		}
		results = append(results, dict)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return starlark.NewList(results), nil
}

// apply performs server-side apply on Kubernetes resources.
// Signature: k8s.apply(manifest, namespace="", field_manager="starkite", dry_run=False, force=False, timeout="")
func (c *K8sClient) apply(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// manifest and owner are starlark.Value — extract before startype
	var manifest, ownerValue starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "manifest", &manifest)
	filteredKwargs = filterKwargValue(filteredKwargs, "owner", &ownerValue)
	remaining := args
	if manifest == nil && len(args) > 0 {
		manifest = args[0]
		remaining = args[1:]
	}

	var p struct {
		Namespace    string `name:"namespace"`
		FieldManager string `name:"field_manager"`
		DryRun       bool   `name:"dry_run"`
		Force        bool   `name:"force"`
		Timeout      string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, fmt.Errorf("k8s.apply: missing required argument: manifest")
	}

	fieldManager := p.FieldManager
	if fieldManager == "" {
		fieldManager = "starkite"
	}

	objs, err := parseManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("k8s.apply: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.apply: %w", err)
	}
	defer cancel()

	opts := metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &p.Force,
	}
	if p.DryRun || (c.config != nil && c.config.DryRun) {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	results := make([]starlark.Value, 0, len(objs))
	for _, obj := range objs {
		gvk := obj.GroupVersionKind()
		gvr, namespaced, err := c.resolver.Resolve(gvk.Kind)
		if err != nil {
			return nil, fmt.Errorf("k8s.apply: %w", err)
		}

		// Inject ownerReference if owner is provided
		if ownerValue != nil {
			if err := injectOwnerRef(obj, ownerValue); err != nil {
				return nil, fmt.Errorf("k8s.apply: owner: %w", err)
			}
		}

		data, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("k8s.apply: marshal: %w", err)
		}

		objNs := ns
		if n := obj.GetNamespace(); n != "" {
			objNs = n
		}

		var applied *unstructuredObj
		if namespaced {
			applied, err = c.dynClient.Resource(gvr).Namespace(objNs).Patch(ctx, obj.GetName(), types.ApplyPatchType, data, opts)
		} else {
			applied, err = c.dynClient.Resource(gvr).Patch(ctx, obj.GetName(), types.ApplyPatchType, data, opts)
		}
		if err != nil {
			return nil, fmt.Errorf("k8s.apply: %s %q: %w", gvk.Kind, obj.GetName(), err)
		}

		dict, err := unstructuredToDict(applied)
		if err != nil {
			return nil, fmt.Errorf("k8s.apply: convert result: %w", err)
		}
		results = append(results, dict)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return starlark.NewList(results), nil
}

// del deletes a Kubernetes resource.
// Signature: k8s.delete(kind, name, namespace="", propagation="Background", timeout="")
func (c *K8sClient) del(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind        string `name:"kind" position:"0" required:"true"`
		Name        string `name:"name" position:"1" required:"true"`
		Namespace   string `name:"namespace"`
		Propagation string `name:"propagation"`
		Timeout     string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.delete: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	opts := metav1.DeleteOptions{}
	switch strings.ToLower(p.Propagation) {
	case "orphan":
		policy := metav1.DeletePropagationOrphan
		opts.PropagationPolicy = &policy
	case "foreground":
		policy := metav1.DeletePropagationForeground
		opts.PropagationPolicy = &policy
	case "", "background":
		policy := metav1.DeletePropagationBackground
		opts.PropagationPolicy = &policy
	default:
		return nil, fmt.Errorf("k8s.delete: unknown propagation policy %q", p.Propagation)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.delete: %w", err)
	}
	defer cancel()

	if namespaced {
		err = c.dynClient.Resource(gvr).Namespace(ns).Delete(ctx, p.Name, opts)
	} else {
		err = c.dynClient.Resource(gvr).Delete(ctx, p.Name, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.delete: %w", err)
	}

	return starlark.True, nil
}

// patch applies a patch to a Kubernetes resource.
// Signature: k8s.patch(kind, name, patch, namespace="", type="merge", timeout="")
func (c *K8sClient) patch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// patch is starlark.Value at position 2 — extract before startype
	var patchData starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "patch", &patchData)
	remaining := args
	if patchData == nil && len(args) > 2 {
		patchData = args[2]
		remaining = args[:2]
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Type      string `name:"type"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if patchData == nil {
		return nil, fmt.Errorf("k8s.patch: missing required argument: patch")
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.patch: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	// Convert patch to JSON
	var patchBytes []byte
	switch v := patchData.(type) {
	case *starlark.Dict:
		dict, err := dictToUnstructured(v)
		if err != nil {
			return nil, fmt.Errorf("k8s.patch: %w", err)
		}
		patchBytes, err = json.Marshal(dict.Object)
		if err != nil {
			return nil, fmt.Errorf("k8s.patch: marshal: %w", err)
		}
	case starlark.String:
		patchBytes = []byte(string(v))
	default:
		return nil, fmt.Errorf("k8s.patch: patch must be dict or JSON string, got %s", patchData.Type())
	}

	// Map patch type string to k8s type
	var pt types.PatchType
	switch strings.ToLower(p.Type) {
	case "strategic":
		pt = types.StrategicMergePatchType
	case "json":
		pt = types.JSONPatchType
	case "", "merge":
		pt = types.MergePatchType
	default:
		return nil, fmt.Errorf("k8s.patch: unknown patch type %q (use merge, strategic, or json)", p.Type)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.patch: %w", err)
	}
	defer cancel()

	var patched *unstructuredObj
	if namespaced {
		patched, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, pt, patchBytes, metav1.PatchOptions{})
	} else {
		patched, err = c.dynClient.Resource(gvr).Patch(ctx, p.Name, pt, patchBytes, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.patch: %w", err)
	}

	return unstructuredToDict(patched)
}

// label sets labels on a Kubernetes resource.
// Signature: k8s.label(kind, name, labels, namespace="", timeout="")
func (c *K8sClient) label(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// labels is *starlark.Dict at position 2 — extract before startype
	var labelsDict *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "labels", &labelsDict)
	remaining := args
	if labelsDict == nil && len(args) > 2 {
		if d, ok := args[2].(*starlark.Dict); ok {
			labelsDict = d
		}
		remaining = args[:2]
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if labelsDict == nil {
		return nil, fmt.Errorf("k8s.label: missing required argument: labels")
	}

	// Build the merge patch for metadata.labels
	labels := make(map[string]any)
	for _, item := range labelsDict.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("k8s.label: label key must be string, got %s", item[0].Type())
		}
		v, ok := starlark.AsString(item[1])
		if !ok {
			return nil, fmt.Errorf("k8s.label: label value must be string, got %s", item[1].Type())
		}
		labels[k] = v
	}

	patchObj := map[string]any{
		"metadata": map[string]any{
			"labels": labels,
		},
	}
	patchBytes, err := json.Marshal(patchObj)
	if err != nil {
		return nil, fmt.Errorf("k8s.label: marshal: %w", err)
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.label: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.label: %w", err)
	}
	defer cancel()

	var patched *unstructuredObj
	if namespaced {
		patched, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	} else {
		patched, err = c.dynClient.Resource(gvr).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.label: %w", err)
	}

	return unstructuredToDict(patched)
}

// annotate sets annotations on a Kubernetes resource.
// Signature: k8s.annotate(kind, name, annotations, namespace="", timeout="")
func (c *K8sClient) annotate(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// annotations is *starlark.Dict at position 2 — extract before startype
	var annotationsDict *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "annotations", &annotationsDict)
	remaining := args
	if annotationsDict == nil && len(args) > 2 {
		if d, ok := args[2].(*starlark.Dict); ok {
			annotationsDict = d
		}
		remaining = args[:2]
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if annotationsDict == nil {
		return nil, fmt.Errorf("k8s.annotate: missing required argument: annotations")
	}

	annotations := make(map[string]any)
	for _, item := range annotationsDict.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("k8s.annotate: annotation key must be string, got %s", item[0].Type())
		}
		v, ok := starlark.AsString(item[1])
		if !ok {
			return nil, fmt.Errorf("k8s.annotate: annotation value must be string, got %s", item[1].Type())
		}
		annotations[k] = v
	}

	patchObj := map[string]any{
		"metadata": map[string]any{
			"annotations": annotations,
		},
	}
	patchBytes, err := json.Marshal(patchObj)
	if err != nil {
		return nil, fmt.Errorf("k8s.annotate: marshal: %w", err)
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.annotate: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.annotate: %w", err)
	}
	defer cancel()

	var patched *unstructuredObj
	if namespaced {
		patched, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	} else {
		patched, err = c.dynClient.Resource(gvr).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.annotate: %w", err)
	}

	return unstructuredToDict(patched)
}

// version returns the Kubernetes server version.
// Signature: k8s.version(timeout="")
func (c *K8sClient) version(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
		return nil, err
	}

	info, err := c.disc.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("k8s.version: %w", err)
	}

	dict := starlark.NewDict(4)
	dict.SetKey(starlark.String("major"), starlark.String(info.Major))
	dict.SetKey(starlark.String("minor"), starlark.String(info.Minor))
	dict.SetKey(starlark.String("git_version"), starlark.String(info.GitVersion))
	dict.SetKey(starlark.String("platform"), starlark.String(info.Platform))

	return dict, nil
}

// apiResources returns available API resources on the server.
// Signature: k8s.api_resources(timeout="")
func (c *K8sClient) apiResources(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "read", ""); err != nil {
		return nil, err
	}

	lists, err := c.disc.ServerPreferredResources()
	if err != nil && lists == nil {
		return nil, fmt.Errorf("k8s.api_resources: %w", err)
	}

	var results []starlark.Value
	for _, list := range lists {
		for _, res := range list.APIResources {
			d := starlark.NewDict(5)
			d.SetKey(starlark.String("name"), starlark.String(res.Name))
			d.SetKey(starlark.String("kind"), starlark.String(res.Kind))
			d.SetKey(starlark.String("group_version"), starlark.String(list.GroupVersion))
			d.SetKey(starlark.String("namespaced"), starlark.Bool(res.Namespaced))

			verbs := make([]starlark.Value, len(res.Verbs))
			for j, v := range res.Verbs {
				verbs[j] = starlark.String(v)
			}
			d.SetKey(starlark.String("verbs"), starlark.NewList(verbs))

			results = append(results, d)
		}
	}

	return starlark.NewList(results), nil
}

// ============================================================================
// Owner reference injection
// ============================================================================

// injectOwnerRef adds an ownerReference to the object from the owner value.
func injectOwnerRef(obj *unstructuredObj, owner starlark.Value) error {
	var apiVersion, kind, name, uid string

	switch o := owner.(type) {
	case *AttrDict:
		apiVersion, _ = o.data["apiVersion"].(string)
		kind, _ = o.data["kind"].(string)
		if meta, ok := o.data["metadata"].(map[string]interface{}); ok {
			name, _ = meta["name"].(string)
			uid, _ = meta["uid"].(string)
		}
	case *starlark.Dict:
		if v, found, _ := o.Get(starlark.String("apiVersion")); found {
			apiVersion, _ = starlark.AsString(v)
		}
		if v, found, _ := o.Get(starlark.String("kind")); found {
			kind, _ = starlark.AsString(v)
		}
		if v, found, _ := o.Get(starlark.String("metadata")); found {
			if meta, ok := v.(*starlark.Dict); ok {
				if n, f, _ := meta.Get(starlark.String("name")); f {
					name, _ = starlark.AsString(n)
				}
				if u, f, _ := meta.Get(starlark.String("uid")); f {
					uid, _ = starlark.AsString(u)
				}
			}
		}
	default:
		return fmt.Errorf("owner must be a dict or AttrDict, got %s", owner.Type())
	}

	if apiVersion == "" || kind == "" || name == "" || uid == "" {
		return fmt.Errorf("owner missing required fields (apiVersion=%q, kind=%q, name=%q, uid=%q)", apiVersion, kind, name, uid)
	}

	ref := map[string]interface{}{
		"apiVersion":         apiVersion,
		"kind":               kind,
		"name":               name,
		"uid":                uid,
		"controller":         true,
		"blockOwnerDeletion": true,
	}

	metadata, ok := obj.Object["metadata"].(map[string]interface{})
	if !ok {
		metadata = make(map[string]interface{})
		obj.Object["metadata"] = metadata
	}
	refs, _ := metadata["ownerReferences"].([]interface{})
	metadata["ownerReferences"] = append(refs, ref)
	return nil
}

// ============================================================================
// Status subresource update
// ============================================================================

// updateStatus updates the status subresource of a Kubernetes resource.
// Signature: k8s.status(obj, status, namespace="", timeout="")
func (c *K8sClient) updateStatus(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var objValue, statusValue starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "obj", &objValue)
	filteredKwargs = filterKwargValue(filteredKwargs, "status", &statusValue)
	remaining := args
	if objValue == nil && len(remaining) > 0 {
		objValue = remaining[0]
		remaining = remaining[1:]
	}
	if statusValue == nil && len(remaining) > 0 {
		statusValue = remaining[0]
		remaining = remaining[1:]
	}

	var p struct {
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if objValue == nil || statusValue == nil {
		return nil, fmt.Errorf("k8s.status: requires obj and status arguments")
	}

	var objKind, objName, objNs string
	switch o := objValue.(type) {
	case *AttrDict:
		objKind, _ = o.data["kind"].(string)
		if meta, ok := o.data["metadata"].(map[string]interface{}); ok {
			objName, _ = meta["name"].(string)
			objNs, _ = meta["namespace"].(string)
		}
	case *starlark.Dict:
		if v, found, _ := o.Get(starlark.String("kind")); found {
			objKind, _ = starlark.AsString(v)
		}
		if v, found, _ := o.Get(starlark.String("metadata")); found {
			if meta, ok := v.(*starlark.Dict); ok {
				if n, f, _ := meta.Get(starlark.String("name")); f {
					objName, _ = starlark.AsString(n)
				}
				if n, f, _ := meta.Get(starlark.String("namespace")); f {
					objNs, _ = starlark.AsString(n)
				}
			}
		}
	default:
		return nil, fmt.Errorf("k8s.status: obj must be a dict or AttrDict, got %s", objValue.Type())
	}

	if objKind == "" || objName == "" {
		return nil, fmt.Errorf("k8s.status: obj missing kind or metadata.name")
	}
	if p.Namespace != "" {
		objNs = p.Namespace
	}
	if objNs == "" {
		objNs = c.namespace
	}

	statusDict, ok := statusValue.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("k8s.status: status must be a dict, got %s", statusValue.Type())
	}
	statusMap, err := startype.Dict(statusDict).ToMap()
	if err != nil {
		return nil, fmt.Errorf("k8s.status: %w", err)
	}

	gvr, _, err := c.resolver.Resolve(objKind)
	if err != nil {
		return nil, fmt.Errorf("k8s.status: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.status: %w", err)
	}
	defer cancel()

	current, err := c.dynClient.Resource(gvr).Namespace(objNs).Get(ctx, objName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.status: get %s %q: %w", objKind, objName, err)
	}

	currentStatus, _ := current.Object["status"].(map[string]interface{})
	if currentStatus == nil {
		currentStatus = make(map[string]interface{})
	}
	for k, v := range statusMap {
		currentStatus[k] = v
	}
	current.Object["status"] = currentStatus

	updated, err := c.dynClient.Resource(gvr).Namespace(objNs).UpdateStatus(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.status: update %s %q: %w", objKind, objName, err)
	}

	return unstructuredToDict(updated)
}
