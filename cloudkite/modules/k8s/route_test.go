package k8s

import (
	"testing"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestHighLevel_Route(t *testing.T) {
	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		httpRouteGVR: "HTTPRouteList",
	})
	ctx := t.Context()

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	initialObj := &unstructuredObj{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      "web-route",
				"namespace": "default",
			},
		},
	}
	if _, err := fakeClient.Resource(httpRouteGVR).Namespace("default").Create(ctx, initialObj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed HTTPRoute in fakeClient: %v", err)
	}

	thread := &starlark.Thread{Name: "test-thread"}

	args := starlark.Tuple{starlark.String("web-route")}
	kwargs := []starlark.Tuple{
		{starlark.String("gateway"), starlark.String("prod-gw")},
		{starlark.String("service"), starlark.String("web-svc")},
		{starlark.String("port"), starlark.MakeInt(8080)},
		{starlark.String("prefix"), starlark.String("/api")},
		{starlark.String("host"), starlark.String("api.example.com")},
	}

	res, err := kClient.route(thread, nil, args, kwargs)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}

	attrDict, ok := res.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict result, got %T", res)
	}
	if name := attrDict.data["route"]; name != "web-route" {
		t.Errorf("expected route 'web-route', got %v", name)
	}

	// Verify the HTTPRoute was created in fakeClient
	obj, err := fakeClient.Resource(httpRouteGVR).Namespace("default").Get(ctx, "web-route", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created HTTPRoute: %v", err)
	}
	if obj.GetName() != "web-route" {
		t.Errorf("HTTPRoute name = %q, want 'web-route'", obj.GetName())
	}
}
