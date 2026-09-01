package k8s

import (
	"testing"

	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/project-starkite/starkite/libkite/fleet"
)

func TestK8sClientFleetPods(t *testing.T) {
	scheme := runtime.NewScheme()

	pod1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "api-pod-1",
				"namespace": "production",
				"uid":       "pod-uid-1",
				"labels": map[string]any{
					"app": "api",
					"env": "prod",
				},
			},
			"status": map[string]any{
				"podIP": "10.244.1.5",
			},
		},
	}
	pod2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "web-pod-1",
				"namespace": "production",
				"uid":       "pod-uid-2",
				"labels": map[string]any{
					"app": "web",
					"env": "prod",
				},
			},
			"status": map[string]any{
				"podIP": "10.244.1.6",
			},
		},
	}

	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, pod1, pod2)
	resolver := NewResolver(nil)

	client := &K8sClient{
		dynClient: dynClient,
		resolver:  resolver,
		namespace: "production",
	}

	thread := &starlark.Thread{Name: "test-k8s-fleet"}
	val, err := client.fleet(thread, starlark.NewBuiltin("k8s.fleet", nil), nil, []starlark.Tuple{
		{starlark.String("kind"), starlark.String("Pod")},
		{starlark.String("namespace"), starlark.String("production")},
	})
	if err != nil {
		t.Fatalf("k8s.fleet error: %v", err)
	}

	fl, ok := val.(*fleet.Fleet)
	if !ok {
		t.Fatalf("expected *fleet.Fleet, got %T", val)
	}

	if len(fl.Resources()) != 2 {
		t.Fatalf("expected 2 resources in fleet, got %d", len(fl.Resources()))
	}

	// Verify filtering on the resulting fleet
	webFleetVal, err := fl.Attr("filter")
	if err != nil {
		t.Fatal(err)
	}
	webBuiltin := webFleetVal.(*starlark.Builtin)
	res, err := starlark.Call(thread, webBuiltin, nil, []starlark.Tuple{
		{starlark.String("app"), starlark.String("web")},
	})
	if err != nil {
		t.Fatal(err)
	}
	webFleet := res.(*fleet.Fleet)
	if len(webFleet.Resources()) != 1 || webFleet.Resources()[0].Name != "web-pod-1" {
		t.Fatalf("expected 1 web pod, got %v", webFleet.Resources())
	}
	if webFleet.Resources()[0].Address != "10.244.1.6" {
		t.Errorf("expected podIP 10.244.1.6, got %s", webFleet.Resources()[0].Address)
	}
}

func TestK8sClientFleetNodes(t *testing.T) {
	scheme := runtime.NewScheme()

	node1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"name": "kind-worker-1",
				"uid":  "node-uid-1",
				"labels": map[string]any{
					"topology.kubernetes.io/zone": "us-east-1a",
				},
			},
			"status": map[string]any{
				"addresses": []any{
					map[string]any{
						"type":    "InternalIP",
						"address": "172.18.0.3",
					},
					map[string]any{
						"type":    "Hostname",
						"address": "kind-worker-1",
					},
				},
			},
		},
	}

	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, node1)
	resolver := NewResolver(nil)

	client := &K8sClient{
		dynClient: dynClient,
		resolver:  resolver,
	}

	thread := &starlark.Thread{Name: "test-k8s-fleet-nodes"}
	val, err := client.fleet(thread, starlark.NewBuiltin("k8s.fleet", nil), nil, []starlark.Tuple{
		{starlark.String("kind"), starlark.String("Node")},
	})
	if err != nil {
		t.Fatalf("k8s.fleet error: %v", err)
	}

	fl, ok := val.(*fleet.Fleet)
	if !ok {
		t.Fatalf("expected *fleet.Fleet, got %T", val)
	}

	if len(fl.Resources()) != 1 {
		t.Fatalf("expected 1 node in fleet, got %d", len(fl.Resources()))
	}

	res := fl.Resources()[0]
	if res.Name != "kind-worker-1" {
		t.Errorf("expected node name kind-worker-1, got %s", res.Name)
	}
	if res.Address != "172.18.0.3" {
		t.Errorf("expected InternalIP 172.18.0.3, got %s", res.Address)
	}
	if res.Kind != "node" {
		t.Errorf("expected kind 'node', got %s", res.Kind)
	}
}
