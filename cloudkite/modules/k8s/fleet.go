package k8s

import (
	"fmt"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/fleet"
)

// fleet creates a Fleet of Kubernetes compute resources (Nodes, Pods, Services, etc.).
// Signature: k8s.fleet(kind="Pod", namespace="", labels={}, label_selector="", fields="", timeout="")
func (c *K8sClient) fleet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "k8s", "read", "read", ""); err != nil {
		return nil, err
	}

	var rawLabels *starlark.Dict
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "labels" {
			if d, ok := kv[1].(*starlark.Dict); ok {
				rawLabels = d
			}
		} else {
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	var p struct {
		Kind          string `name:"kind" position:"0"`
		Namespace     string `name:"namespace"`
		LabelSelector string `name:"label_selector"`
		Fields        string `name:"fields"`
		Timeout       string `name:"timeout"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Kind == "" {
		p.Kind = "Pod"
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.fleet: %w", err)
	}

	ns := p.Namespace
	if ns == "" && namespaced {
		ns = c.namespace
	}

	opts := metav1.ListOptions{}
	if p.LabelSelector != "" {
		opts.LabelSelector = p.LabelSelector
	} else if rawLabels != nil {
		var parts []string
		for _, item := range rawLabels.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					if v == "" {
						parts = append(parts, k)
					} else {
						parts = append(parts, fmt.Sprintf("%s=%s", k, v))
					}
				}
			}
		}
		opts.LabelSelector = strings.Join(parts, ",")
	}
	if p.Fields != "" {
		opts.FieldSelector = p.Fields
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.fleet: %w", err)
	}
	defer cancel()

	var list *unstructured.UnstructuredList
	if namespaced {
		list, err = c.dynClient.Resource(gvr).Namespace(ns).List(ctx, opts)
	} else {
		list, err = c.dynClient.Resource(gvr).List(ctx, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.fleet: %w", err)
	}

	resources := make([]fleet.Resource, 0, len(list.Items))
	kindLower := strings.ToLower(p.Kind)

	for _, item := range list.Items {
		r := fleet.Resource{
			ID:     string(item.GetUID()),
			Name:   item.GetName(),
			Kind:   strings.ToLower(item.GetKind()),
			Labels: item.GetLabels(),
			Data:   item.UnstructuredContent(),
		}
		if r.Labels == nil {
			r.Labels = make(map[string]string)
		}
		if r.ID == "" {
			r.ID = r.Name
		}
		if r.Kind == "" {
			r.Kind = kindLower
		}

		// Resource-specific address extraction
		switch kindLower {
		case "node", "nodes":
			// Extract internal/external IP from node status
			addresses, found, _ := unstructured.NestedSlice(item.Object, "status", "addresses")
			if found {
				var internalIP, externalIP, hostname string
				for _, a := range addresses {
					if addrMap, ok := a.(map[string]any); ok {
						addrType, _ := addrMap["type"].(string)
						addrVal, _ := addrMap["address"].(string)
						switch addrType {
						case "InternalIP":
							internalIP = addrVal
						case "ExternalIP":
							externalIP = addrVal
						case "Hostname":
							hostname = addrVal
						}
					}
				}
				if internalIP != "" {
					r.Address = internalIP
				} else if externalIP != "" {
					r.Address = externalIP
				} else if hostname != "" {
					r.Address = hostname
				}
			}
			if r.Address == "" {
				r.Address = r.Name
			}

		case "pod", "pods":
			podIP, found, _ := unstructured.NestedString(item.Object, "status", "podIP")
			if found && podIP != "" {
				r.Address = podIP
			} else {
				r.Address = r.Name
			}
			// Add namespace to labels for easy filtering
			if item.GetNamespace() != "" {
				r.Labels["namespace"] = item.GetNamespace()
			}

		default:
			r.Address = r.Name
		}

		resources = append(resources, r)
	}

	return fleet.New(resources), nil
}
