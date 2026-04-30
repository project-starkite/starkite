package k8s

import (
	"go.starlark.net/starlark"
)

// KubeObject is the interface for all Kubernetes object types returned by k8s.obj constructors.
// It implements starlark.HasAttrs for attribute access and provides conversion to dict form.
type KubeObject interface {
	starlark.HasAttrs
	// ToDict converts the object to a *starlark.Dict suitable for k8s API operations.
	ToDict() *starlark.Dict
	// Kind returns the Kubernetes kind string (e.g., "Deployment", "Service").
	Kind() string
}

// clientMethod is the signature for methods on K8sClient that are exposed as Starlark builtins.
type clientMethod func(c *K8sClient, thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

// ExecResult holds the result of executing a command in a pod.
type ExecResult struct {
	Stdout string
	Stderr string
	Code   int
}

// PortForwardHandle is a Starlark value representing an active port-forward session.
type PortForwardHandle struct {
	localPort int
	stopCh    chan struct{}
}

func (h *PortForwardHandle) String() string        { return "<port_forward>" }
func (h *PortForwardHandle) Type() string           { return "port_forward" }
func (h *PortForwardHandle) Freeze()                {}
func (h *PortForwardHandle) Truth() starlark.Bool   { return starlark.True }
func (h *PortForwardHandle) Hash() (uint32, error)  { return 0, nil }

func (h *PortForwardHandle) Attr(name string) (starlark.Value, error) {
	switch name {
	case "local_port":
		return starlark.MakeInt(h.localPort), nil
	case "stop":
		return starlark.NewBuiltin("port_forward.stop", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			close(h.stopCh)
			return starlark.None, nil
		}), nil
	}
	return nil, nil
}

func (h *PortForwardHandle) AttrNames() []string {
	return []string{"local_port", "stop"}
}

// DrainResult holds the result of a node drain operation.
type DrainResult struct {
	Evicted []string
	Errors  []string
	Node    string
}

// RolloutStatus holds the result of a rollout status check.
type RolloutStatus struct {
	Complete  bool
	Replicas  int
	Ready     int
	Updated   int
	Available int
}
