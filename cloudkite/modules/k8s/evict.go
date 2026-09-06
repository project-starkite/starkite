package k8s

import (
	"fmt"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// evict evicts a Pod by calling the /eviction subresource, honoring PodDisruptionBudgets.
// Signature: k8s.evict(name, namespace="", dry_run=False, timeout="")
func (c *K8sClient) evict(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "write", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Namespace string `name:"namespace"`
		DryRun    bool   `name:"dry_run"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Name == "" {
		return nil, fmt.Errorf("k8s.evict: name is required")
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}
	if ns == "" {
		ns = "default"
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.evict: %w", err)
	}
	defer cancel()

	podGVR, _, err := c.resolver.Resolve("pod")
	if err != nil {
		return nil, fmt.Errorf("k8s.evict: resolve pod: %w", err)
	}

	eviction := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "policy/v1",
			"kind":       "Eviction",
			"metadata": map[string]any{
				"name":      p.Name,
				"namespace": ns,
			},
		},
	}

	opts := metav1.CreateOptions{}
	if p.DryRun || (c.config != nil && c.config.DryRun) {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = c.dynClient.Resource(podGVR).Namespace(ns).Create(ctx, eviction, opts, "eviction")
	if err != nil {
		return nil, fmt.Errorf("k8s.evict %s/%s: %w", ns, p.Name, err)
	}

	return NewAttrDict(map[string]any{
		"evicted":   true,
		"name":      p.Name,
		"namespace": ns,
		"dry_run":   p.DryRun,
	}), nil
}
