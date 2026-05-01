package k8s

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "k8s"

// Module provides 3-tier Kubernetes operations for the cloud edition.
// Tier 1: CRUD, watch, I/O, cluster info
// Tier 2: High-level kubectl abstractions (deploy, scale, drain, etc.)
// Tier 3: k8s.obj.* constructors (pod, deployment, service, etc.)
type Module struct {
	once          sync.Once
	module        starlark.Value
	config        *libkite.ModuleConfig
	defaultClient *K8sClient
	mu            sync.Mutex
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "k8s provides 3-tier Kubernetes operations: CRUD (get, list, apply, delete, patch, label, annotate), " +
		"watch (watch, wait_for), I/O (logs, exec, port_forward), " +
		"high-level (deploy, scale, rollout, drain), and k8s.obj constructors"
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "config" }

// ensureDefaultClient lazily creates a K8sClient using the current kubeconfig defaults.
func (m *Module) ensureDefaultClient(thread *starlark.Thread) (*K8sClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.defaultClient != nil {
		return m.defaultClient, nil
	}

	client, err := newK8sClient(thread, m.config, "", "", "", "")
	if err != nil {
		return nil, fmt.Errorf("k8s: failed to create default client: %w", err)
	}
	m.defaultClient = client
	return m.defaultClient, nil
}

// withDefault wraps a clientMethod so it can be called at module level.
// It lazily creates a default client and delegates to the client method.
func (m *Module) withDefault(name string, method clientMethod) *starlark.Builtin {
	return starlark.NewBuiltin("k8s."+name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		client, err := m.ensureDefaultClient(thread)
		if err != nil {
			return nil, err
		}
		return method(client, thread, fn, args, kwargs)
	})
}

// configFactory creates a K8sClient with explicit configuration.
// Signature: k8s.config(context="", namespace="", kubeconfig="", timeout="")
func (m *Module) configFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "config", ""); err != nil {
		return nil, err
	}

	var p struct {
		Context    string `name:"context"`
		Namespace  string `name:"namespace"`
		Kubeconfig string `name:"kubeconfig"`
		Timeout    string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	return newK8sClient(thread, m.config, p.Context, p.Namespace, p.Kubeconfig, p.Timeout)
}

// yamlHelper converts a Starlark value (KubeObject, dict, or list) to a YAML string.
// For a single value, encodes as one YAML document. For a list, produces multi-doc
// YAML with "---" separators. Delegates to startype for conversion, so any type
// implementing startype.DictConvertible is supported automatically.
func (m *Module) yamlHelper(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var obj starlark.Value
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &obj); err != nil {
		return nil, err
	}

	// For lists, produce multi-doc YAML
	if list, ok := obj.(*starlark.List); ok {
		var buf bytes.Buffer
		for i := 0; i < list.Len(); i++ {
			if i > 0 {
				buf.WriteString("---\n")
			}
			data, err := encodeOne(list.Index(i))
			if err != nil {
				return nil, fmt.Errorf("k8s.yaml[%d]: %w", i, err)
			}
			buf.Write(data)
		}
		return starlark.String(buf.String()), nil
	}

	// Single value
	data, err := encodeOne(obj)
	if err != nil {
		return nil, fmt.Errorf("k8s.yaml: %w", err)
	}
	return starlark.String(data), nil
}

// encodeOne converts a single Starlark value to YAML bytes via startype.
func encodeOne(val starlark.Value) ([]byte, error) {
	var goVal any
	if err := startype.Starlark(val).Go(&goVal); err != nil {
		return nil, err
	}
	return yaml.Marshal(goVal)
}

// Load builds the k8s module with all three tiers.
func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		// Build k8s.obj sub-namespace with real constructors
		objModule := &starlarkstruct.Module{
			Name:    "k8s.obj",
			Members: ObjConstructors(),
		}

		// Build top-level members
		members := starlark.StringDict{
			// Factory
			"config": starlark.NewBuiltin("k8s.config", m.configFactory),

			// Controller runtime
			"control": starlark.NewBuiltin("k8s.control", m.controlBuiltin),

			// Admission webhooks
			"webhook": starlark.NewBuiltin("k8s.webhook", m.webhookBuiltin),

			// Utility
			"yaml": starlark.NewBuiltin("k8s.yaml", m.yamlHelper),

			// k8s.obj sub-namespace
			"obj": objModule,

			// Tier 1: CRUD
			"get":      m.withDefault("get", (*K8sClient).get),
			"list":     m.withDefault("list", (*K8sClient).listResources),
			"create":   m.withDefault("create", (*K8sClient).create),
			"apply":    m.withDefault("apply", (*K8sClient).apply),
			"delete":   m.withDefault("delete", (*K8sClient).del),
			"patch":    m.withDefault("patch", (*K8sClient).patch),
			"label":    m.withDefault("label", (*K8sClient).label),
			"annotate": m.withDefault("annotate", (*K8sClient).annotate),
			"status":   m.withDefault("status", (*K8sClient).updateStatus),

			// Tier 1: Watch
			"watch":    m.withDefault("watch", (*K8sClient).watch),
			"wait_for": m.withDefault("wait_for", (*K8sClient).waitFor),

			// Tier 1: I/O
			"logs":         m.withDefault("logs", (*K8sClient).logs),
			"logs_follow":  m.withDefault("logs_follow", (*K8sClient).logsFollow),
			"exec":         m.withDefault("exec", (*K8sClient).execCmd),
			"port_forward": m.withDefault("port_forward", (*K8sClient).portForward),

			// Tier 1: Cluster info
			"context":        m.withDefault("context", (*K8sClient).contextName),
			"namespace_name": m.withDefault("namespace_name", (*K8sClient).namespaceName),
			"version":        m.withDefault("version", (*K8sClient).version),
			"api_resources":  m.withDefault("api_resources", (*K8sClient).apiResources),

			// Tier 2: High-level
			"deploy":        m.withDefault("deploy", (*K8sClient).deployHighLevel),
			"run":           m.withDefault("run", (*K8sClient).run),
			"expose":        m.withDefault("expose", (*K8sClient).expose),
			"scale":         m.withDefault("scale", (*K8sClient).scale),
			"autoscale":     m.withDefault("autoscale", (*K8sClient).autoscale),
			"rollout":       m.withDefault("rollout", (*K8sClient).rollout),
			"set_image":     m.withDefault("set_image", (*K8sClient).setImage),
			"set_env":       m.withDefault("set_env", (*K8sClient).setEnv),
			"set_resources": m.withDefault("set_resources", (*K8sClient).setResources),

			// Tier 2: Node ops
			"drain":     m.withDefault("drain", (*K8sClient).drain),
			"cordon":    m.withDefault("cordon", (*K8sClient).cordon),
			"uncordon":  m.withDefault("uncordon", (*K8sClient).uncordon),
			"taint":     m.withDefault("taint", (*K8sClient).taint),
			"untaint":   m.withDefault("untaint", (*K8sClient).untaint),
			"top_nodes": m.withDefault("top_nodes", (*K8sClient).topNodes),
			"top_pods":  m.withDefault("top_pods", (*K8sClient).topPods),
			"cp":        m.withDefault("cp", (*K8sClient).cp),
			"describe":  m.withDefault("describe", (*K8sClient).describe),
		}

		m.module = libkite.NewTryModule(string(ModuleName), members)
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}
