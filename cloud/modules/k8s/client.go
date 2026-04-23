package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sClient wraps a Kubernetes dynamic client and exposes Tier 1 + Tier 2
// operations as Starlark HasAttrs methods.
type K8sClient struct {
	dynClient dynamic.Interface
	disc      discovery.DiscoveryInterface
	resolver  *Resolver
	restCfg   *rest.Config
	namespace string
	context   string
	timeout   string
	config    *starbase.ModuleConfig
	thread    *starlark.Thread
}

// Starlark value interface

func (c *K8sClient) String() string {
	return fmt.Sprintf("<k8s.client context=%q namespace=%q>", c.context, c.namespace)
}

func (c *K8sClient) Type() string          { return "k8s.client" }
func (c *K8sClient) Freeze()               {}
func (c *K8sClient) Truth() starlark.Bool  { return starlark.True }
func (c *K8sClient) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: k8s.client") }

// allMethods defines the mapping from method names to implementations.
// Tier 1 and Tier 2 methods that are not yet implemented return stubs.
var allMethods = map[string]clientMethod{
	// Tier 1: CRUD
	"get":      (*K8sClient).get,
	"list":     (*K8sClient).listResources,
	"create":   (*K8sClient).create,
	"apply":    (*K8sClient).apply,
	"delete":   (*K8sClient).del,
	"patch":    (*K8sClient).patch,
	"label":    (*K8sClient).label,
	"annotate": (*K8sClient).annotate,
	"status":   (*K8sClient).updateStatus,

	// Tier 1: Watch
	"watch":    (*K8sClient).watch,
	"wait_for": (*K8sClient).waitFor,

	// Tier 1: I/O
	"logs":         (*K8sClient).logs,
	"logs_follow":  (*K8sClient).logsFollow,
	"exec":         (*K8sClient).execCmd,
	"port_forward": (*K8sClient).portForward,

	// Tier 1: Cluster info
	"context":       (*K8sClient).contextName,
	"namespace_name": (*K8sClient).namespaceName,
	"version":       (*K8sClient).version,
	"api_resources": (*K8sClient).apiResources,

	// Tier 2: High-level
	"deploy":        (*K8sClient).deployHighLevel,
	"run":           (*K8sClient).run,
	"expose":        (*K8sClient).expose,
	"scale":         (*K8sClient).scale,
	"autoscale":     (*K8sClient).autoscale,
	"rollout":       (*K8sClient).rollout,
	"set_image":     (*K8sClient).setImage,
	"set_env":       (*K8sClient).setEnv,
	"set_resources": (*K8sClient).setResources,

	// Tier 2: Node ops
	"drain":     (*K8sClient).drain,
	"cordon":    (*K8sClient).cordon,
	"uncordon":  (*K8sClient).uncordon,
	"taint":     (*K8sClient).taint,
	"untaint":   (*K8sClient).untaint,
	"top_nodes": (*K8sClient).topNodes,
	"top_pods":  (*K8sClient).topPods,
	"cp":        (*K8sClient).cp,
	"describe":  (*K8sClient).describe,
}

// Attr returns the named attribute — a builtin method or property.
// Supports try_ prefix: k.try_get(...) returns a Result instead of error.
func (c *K8sClient) Attr(name string) (starlark.Value, error) {
	if baseName, ok := strings.CutPrefix(name, "try_"); ok {
		if method, ok := allMethods[baseName]; ok {
			base := starlark.NewBuiltin("k8s.client."+baseName, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				return method(c, thread, fn, args, kwargs)
			})
			return starbase.TryWrap("k8s.client."+name, base), nil
		}
	}
	if method, ok := allMethods[name]; ok {
		return starlark.NewBuiltin("k8s.client."+name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return method(c, thread, fn, args, kwargs)
		}), nil
	}
	return nil, nil
}

// AttrNames returns all method names for introspection, including try_ variants.
func (c *K8sClient) AttrNames() []string {
	names := make([]string, 0, len(allMethods)*2)
	for name := range allMethods {
		names = append(names, name)
		names = append(names, "try_"+name)
	}
	sort.Strings(names)
	return names
}

// contextWithTimeout returns a context with timeout using fallback logic:
// per-call timeout → client default → no timeout (context.Background).
func (c *K8sClient) contextWithTimeout(perCallTimeout string) (context.Context, context.CancelFunc, error) {
	timeout := perCallTimeout
	if timeout == "" {
		timeout = c.timeout
	}
	if timeout == "" {
		return context.Background(), func() {}, nil
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid timeout %q: %w", timeout, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	return ctx, cancel, nil
}

// newK8sClient creates a K8sClient from kubeconfig parameters.
func newK8sClient(thread *starlark.Thread, config *starbase.ModuleConfig, contextName, namespace, kubeconfig, timeout string) (*K8sClient, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	if namespace != "" {
		overrides.Context.Namespace = namespace
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		// Fallback to in-cluster config (running inside a pod)
		restCfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("k8s.config: failed to build config (tried kubeconfig and in-cluster): %w", err)
		}
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.config: failed to create dynamic client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s.config: failed to create discovery client: %w", err)
	}

	// Resolve the actual namespace if not explicitly set
	if namespace == "" {
		ns, _, err := clientConfig.Namespace()
		if err == nil && ns != "" {
			namespace = ns
		} else {
			namespace = "default"
		}
	}

	// Resolve the actual context
	if contextName == "" {
		rawConfig, err := clientConfig.RawConfig()
		if err == nil {
			contextName = rawConfig.CurrentContext
		}
	}

	return &K8sClient{
		dynClient: dynClient,
		disc:      disc,
		resolver:  NewResolver(disc),
		restCfg:   restCfg,
		namespace: namespace,
		context:   contextName,
		timeout:   timeout,
		config:    config,
		thread:    thread,
	}, nil
}

// filterKwarg extracts a *starlark.Dict kwarg by name, sets *dest if found,
// and returns remaining kwargs for startype.Args.
func filterKwarg(kwargs []starlark.Tuple, name string, dest **starlark.Dict) []starlark.Tuple {
	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == name {
			if d, ok := kv[1].(*starlark.Dict); ok {
				*dest = d
			}
		} else {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}

// filterKwargValue extracts a starlark.Value kwarg by name, sets *dest if found,
// and returns remaining kwargs for startype.Args.
func filterKwargValue(kwargs []starlark.Tuple, name string, dest *starlark.Value) []starlark.Tuple {
	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == name {
			*dest = kv[1]
		} else {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}

// Tier 1: Cluster info (simple methods that don't need separate files)
func (c *K8sClient) contextName(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(c.context), nil
}
func (c *K8sClient) namespaceName(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(c.namespace), nil
}

// Tier 2 methods are implemented in highlevel.go and nodeops.go.
// filterKwargCallable extracts a starlark.Callable kwarg by name, sets *dest if found,
// and returns remaining kwargs for startype.Args.
func filterKwargCallable(kwargs []starlark.Tuple, name string, dest *starlark.Callable) []starlark.Tuple {
	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == name {
			if c, ok := kv[1].(starlark.Callable); ok {
				*dest = c
			}
		} else {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}
