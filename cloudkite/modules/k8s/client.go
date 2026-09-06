package k8s

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/modules/ssh"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sClient wraps a Kubernetes dynamic client and exposes Tier 1 + Tier 2
// operations as Starlark HasAttrs methods.
type K8sClient struct {
	dynClient dynamic.Interface
	clientset kubernetes.Interface
	disc      discovery.DiscoveryInterface
	resolver  *Resolver
	restCfg   *rest.Config
	namespace string
	context   string
	timeout   string
	config    *libkite.ModuleConfig
	thread    *starlark.Thread
	dialer    *ssh.TunnelDialer
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
	"get":              (*K8sClient).get,
	"list":             (*K8sClient).listResources,
	"create":           (*K8sClient).create,
	"apply":            (*K8sClient).apply,
	"delete":           (*K8sClient).del,
	"patch":            (*K8sClient).patch,
	"label":            (*K8sClient).label,
	"annotate":         (*K8sClient).annotate,
	"status":           (*K8sClient).updateStatus,
	"finalizer_add":    (*K8sClient).finalizerAdd,
	"finalizer_remove": (*K8sClient).finalizerRemove,
	"condition_set":    (*K8sClient).conditionSet,
	"event":            (*K8sClient).event,
	"claims":           (*K8sClient).claims,
	"pvcs":             (*K8sClient).pvcs,
	"pvs":              (*K8sClient).pvs,
	"storage_classes":  (*K8sClient).storageClasses,

	// Tier 1: Watch
	"watch":    (*K8sClient).watch,
	"wait_for": (*K8sClient).waitFor,

	// Tier 1: I/O
	"logs":         (*K8sClient).logs,
	"logs_follow":  (*K8sClient).logsFollow,
	"exec":         (*K8sClient).execCmd,
	"debug":        (*K8sClient).debug,
	"port_forward": (*K8sClient).portForward,

	// Tier 1: Cluster info & Fleets
	"context":         (*K8sClient).contextName,
	"namespace_name":  (*K8sClient).namespaceName,
	"version":         (*K8sClient).version,
	"api_resources":   (*K8sClient).apiResources,
	"fleet":           (*K8sClient).fleet,
	"server":          (*K8sClient).serverEndpoint,
	"tls_server_name": (*K8sClient).tlsServerName,
	"close":           (*K8sClient).closeClient,

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
	"resize":        (*K8sClient).resize,
	"route":         (*K8sClient).route,
	"validate":      (*K8sClient).validate,

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
			return libkite.TryWrap("k8s.client."+name, base), nil
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

type clientOptions struct {
	contextName   string
	namespace     string
	kubeconfig    string
	timeout       string
	server        string
	tlsServerName string
	jumpDict      *starlark.Dict
}

// resolveTargetAddr parses an API server URL or host:port string and extracts a strict host:port.
func resolveTargetAddr(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty API server address")
	}

	parseURL := rawURL
	if !strings.Contains(rawURL, "://") {
		parseURL = "https://" + rawURL
	}

	u, err := url.Parse(parseURL)
	if err != nil {
		return "", fmt.Errorf("invalid API server URL %q: %w", rawURL, err)
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("missing host in API server URL %q", rawURL)
	}

	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", fmt.Errorf("cannot infer port for scheme %q in API server URL %q", u.Scheme, rawURL)
		}
	}

	return net.JoinHostPort(host, port), nil
}

// newK8sClient creates a K8sClient from kubeconfig parameters and optional SSH bastion configuration.
func newK8sClient(thread *starlark.Thread, config *libkite.ModuleConfig, opts clientOptions) (*K8sClient, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if opts.kubeconfig != "" {
		rules.ExplicitPath = opts.kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if opts.contextName != "" {
		overrides.CurrentContext = opts.contextName
	}
	if opts.namespace != "" {
		overrides.Context.Namespace = opts.namespace
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

	// Apply server endpoint override if specified
	if opts.server != "" {
		restCfg.Host = opts.server
	}

	// Apply TLS SNI server name override if specified
	if opts.tlsServerName != "" {
		restCfg.TLSClientConfig.ServerName = opts.tlsServerName
	}

	// Establish SSH bastion tunnel if jump is configured
	var tunnelDialer *ssh.TunnelDialer
	if opts.jumpDict != nil {
		jumpCfg, err := ssh.ParseJumpDict(opts.jumpDict)
		if err != nil {
			return nil, fmt.Errorf("k8s.config: invalid jump configuration: %w", err)
		}

		targetAddr, err := resolveTargetAddr(restCfg.Host)
		if err != nil {
			return nil, fmt.Errorf("k8s.config: failed to resolve target API server address from %q: %w", restCfg.Host, err)
		}

		var dialTimeout time.Duration
		if opts.timeout != "" {
			if d, err := time.ParseDuration(opts.timeout); err == nil {
				dialTimeout = d
			}
		}

		tunnelDialer, err = ssh.NewTunnelDialer(jumpCfg, targetAddr, dialTimeout)
		if err != nil {
			return nil, fmt.Errorf("k8s.config: failed to initialize SSH tunnel dialer: %w", err)
		}

		restCfg.Dial = tunnelDialer.DialContext
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		if tunnelDialer != nil {
			_ = tunnelDialer.Close()
		}
		return nil, fmt.Errorf("k8s.config: failed to create dynamic client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		if tunnelDialer != nil {
			_ = tunnelDialer.Close()
		}
		return nil, fmt.Errorf("k8s.config: failed to create discovery client: %w", err)
	}

	// Resolve the actual namespace if not explicitly set
	namespace := opts.namespace
	if namespace == "" {
		ns, _, err := clientConfig.Namespace()
		if err == nil && ns != "" {
			namespace = ns
		} else {
			namespace = "default"
		}
	}

	// Resolve the actual context
	contextName := opts.contextName
	if contextName == "" {
		rawConfig, err := clientConfig.RawConfig()
		if err == nil {
			contextName = rawConfig.CurrentContext
		}
	}

	clientset, _ := kubernetes.NewForConfig(restCfg)

	return &K8sClient{
		dynClient: dynClient,
		clientset: clientset,
		disc:      disc,
		resolver:  NewResolver(disc),
		restCfg:   restCfg,
		namespace: namespace,
		context:   contextName,
		timeout:   opts.timeout,
		config:    config,
		thread:    thread,
		dialer:    tunnelDialer,
	}, nil
}

// getClientset returns the kubernetes.Interface, creating it from restCfg if needed.
func (c *K8sClient) getClientset() (kubernetes.Interface, error) {
	if c.clientset != nil {
		return c.clientset, nil
	}
	if c.restCfg != nil {
		cs, err := kubernetes.NewForConfig(c.restCfg)
		if err != nil {
			return nil, err
		}
		c.clientset = cs
		return cs, nil
	}
	return nil, fmt.Errorf("no kubernetes clientset or rest config available")
}

// filterKwarg extracts a *starlark.Dict kwarg by name (converting *AttrDict to *starlark.Dict if needed),
// sets *dest if found, and returns remaining kwargs for startype.Args.
func filterKwarg(kwargs []starlark.Tuple, name string, dest **starlark.Dict) []starlark.Tuple {
	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == name {
			if d, ok := kv[1].(*starlark.Dict); ok {
				*dest = d
			} else if a, ok := kv[1].(*AttrDict); ok {
				*dest = a.ToDict()
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

func (c *K8sClient) serverEndpoint(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.restCfg != nil {
		return starlark.String(c.restCfg.Host), nil
	}
	return starlark.None, nil
}

func (c *K8sClient) tlsServerName(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.restCfg != nil && c.restCfg.TLSClientConfig.ServerName != "" {
		return starlark.String(c.restCfg.TLSClientConfig.ServerName), nil
	}
	return starlark.None, nil
}

func (c *K8sClient) closeClient(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := c.Close(); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

// Close terminates any active tunnel dialer and associated network channels.
func (c *K8sClient) Close() error {
	if c.dialer != nil {
		err := c.dialer.Close()
		c.dialer = nil
		return err
	}
	return nil
}

// Dialer returns the configured TunnelDialer, or nil if not running over an SSH tunnel.
func (c *K8sClient) Dialer() *ssh.TunnelDialer {
	return c.dialer
}
