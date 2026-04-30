package k8s

import (
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// Resolver maps short kind names (e.g., "pods", "deployments") to their
// full GroupVersionResource using built-in shortcuts and lazy API discovery.
type Resolver struct {
	discovery discovery.DiscoveryInterface
	cache     map[string]resolvedGVR
	mu        sync.RWMutex
}

type resolvedGVR struct {
	gvr        schema.GroupVersionResource
	namespaced bool
}

// builtinKinds maps common kind names/plurals to their GVR and namespaced flag.
var builtinKinds = map[string]resolvedGVR{
	// Core v1
	"pod":                    {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, namespaced: true},
	"pods":                   {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, namespaced: true},
	"service":                {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, namespaced: true},
	"services":               {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, namespaced: true},
	"svc":                    {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, namespaced: true},
	"configmap":              {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, namespaced: true},
	"configmaps":             {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, namespaced: true},
	"cm":                     {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, namespaced: true},
	"secret":                 {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, namespaced: true},
	"secrets":                {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, namespaced: true},
	"namespace":              {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, namespaced: false},
	"namespaces":             {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, namespaced: false},
	"ns":                     {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, namespaced: false},
	"node":                   {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, namespaced: false},
	"nodes":                  {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, namespaced: false},
	"persistentvolumeclaim":  {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, namespaced: true},
	"persistentvolumeclaims": {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, namespaced: true},
	"pvc":                    {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, namespaced: true},
	"serviceaccount":         {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, namespaced: true},
	"serviceaccounts":        {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, namespaced: true},
	"sa":                     {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, namespaced: true},
	"event":                  {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}, namespaced: true},
	"events":                 {gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}, namespaced: true},

	// apps/v1
	"deployment":  {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, namespaced: true},
	"deployments": {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, namespaced: true},
	"deploy":      {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, namespaced: true},
	"statefulset":  {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, namespaced: true},
	"statefulsets":  {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, namespaced: true},
	"sts":          {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, namespaced: true},
	"daemonset":   {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, namespaced: true},
	"daemonsets":  {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, namespaced: true},
	"ds":          {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, namespaced: true},
	"replicaset":  {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, namespaced: true},
	"replicasets": {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, namespaced: true},
	"rs":          {gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, namespaced: true},

	// batch/v1
	"job":      {gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, namespaced: true},
	"jobs":     {gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, namespaced: true},
	"cronjob":  {gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, namespaced: true},
	"cronjobs": {gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, namespaced: true},

	// networking.k8s.io/v1
	"ingress":       {gvr: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, namespaced: true},
	"ingresses":     {gvr: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, namespaced: true},
	"networkpolicy": {gvr: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, namespaced: true},
	"networkpolicies": {gvr: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, namespaced: true},

	// autoscaling/v2
	"horizontalpodautoscaler":  {gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, namespaced: true},
	"horizontalpodautoscalers": {gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, namespaced: true},
	"hpa":                      {gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, namespaced: true},

	// rbac.authorization.k8s.io/v1
	"role":               {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, namespaced: true},
	"roles":              {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, namespaced: true},
	"clusterrole":        {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, namespaced: false},
	"clusterroles":       {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, namespaced: false},
	"rolebinding":        {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, namespaced: true},
	"rolebindings":       {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, namespaced: true},
	"clusterrolebinding":  {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, namespaced: false},
	"clusterrolebindings": {gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, namespaced: false},
}

// NewResolver creates a new Resolver with the given discovery client.
func NewResolver(disc discovery.DiscoveryInterface) *Resolver {
	return &Resolver{
		discovery: disc,
		cache:     make(map[string]resolvedGVR),
	}
}

// Resolve maps a kind string to its GroupVersionResource and namespaced flag.
// It checks built-in shortcuts first, then falls back to API discovery.
// Supports qualified forms like "apps/deployments" to specify the group.
func (r *Resolver) Resolve(kind string) (schema.GroupVersionResource, bool, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))

	// Handle qualified form: "group/resource"
	if parts := strings.SplitN(kind, "/", 2); len(parts) == 2 {
		return r.resolveQualified(parts[0], parts[1])
	}

	// Check built-in shortcuts
	if resolved, ok := builtinKinds[kind]; ok {
		return resolved.gvr, resolved.namespaced, nil
	}

	// Check cache
	r.mu.RLock()
	if resolved, ok := r.cache[kind]; ok {
		r.mu.RUnlock()
		return resolved.gvr, resolved.namespaced, nil
	}
	r.mu.RUnlock()

	// Fall back to API discovery
	return r.discoverKind(kind)
}

// resolveQualified handles "group/resource" qualified form.
func (r *Resolver) resolveQualified(group, resource string) (schema.GroupVersionResource, bool, error) {
	// Check builtins first
	for _, resolved := range builtinKinds {
		if resolved.gvr.Group == group && resolved.gvr.Resource == resource {
			return resolved.gvr, resolved.namespaced, nil
		}
	}

	// Fall back to discovery for qualified names
	return r.discoverQualified(group, resource)
}

// discoverKind uses API discovery to find the GVR for an unknown kind.
func (r *Resolver) discoverKind(kind string) (schema.GroupVersionResource, bool, error) {
	if r.discovery == nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("unknown kind %q and no discovery client available", kind)
	}

	lists, err := r.discovery.ServerPreferredResources()
	if err != nil {
		// Partial results are OK
		if lists == nil {
			return schema.GroupVersionResource{}, false, fmt.Errorf("failed to discover API resources: %w", err)
		}
	}

	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		for _, res := range list.APIResources {
			if strings.EqualFold(res.Name, kind) || strings.EqualFold(res.Kind, kind) {
				resolved := resolvedGVR{
					gvr:        schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: res.Name},
					namespaced: res.Namespaced,
				}
				r.mu.Lock()
				r.cache[kind] = resolved
				r.mu.Unlock()
				return resolved.gvr, resolved.namespaced, nil
			}
		}
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("unknown kind %q", kind)
}

// discoverQualified uses API discovery for a qualified "group/resource" lookup.
func (r *Resolver) discoverQualified(group, resource string) (schema.GroupVersionResource, bool, error) {
	if r.discovery == nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("unknown kind %s/%s and no discovery client available", group, resource)
	}

	lists, err := r.discovery.ServerPreferredResources()
	if err != nil && lists == nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("failed to discover API resources: %w", err)
	}

	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		if gv.Group != group {
			continue
		}
		for _, res := range list.APIResources {
			if res.Name == resource {
				return schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: res.Name}, res.Namespaced, nil
			}
		}
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("unknown kind %s/%s", group, resource)
}
