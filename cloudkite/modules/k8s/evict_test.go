package k8s

import (
	"testing"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestEvict_SuccessAndDryRun(t *testing.T) {
	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	// Prepend reactor for eviction subresource
	evictionCalled := false
	var recordedDryRun []string
	fakeClient.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			evictionCalled = true
			createAct := action.(clienttesting.CreateActionImpl)
			recordedDryRun = createAct.GetCreateOptions().DryRun
			return true, createAct.GetObject(), nil
		}
		return false, nil, nil
	})

	ctx := t.Context()
	podObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "worker-pod",
				"namespace": "production",
			},
		},
	}
	if _, err := fakeClient.Resource(podGVR).Namespace("production").Create(ctx, podObj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "production",
	}

	thread := &starlark.Thread{Name: "test-thread"}

	// 1. Dry run eviction
	args := starlark.Tuple{starlark.String("worker-pod")}
	kwargs := []starlark.Tuple{
		{starlark.String("dry_run"), starlark.Bool(true)},
	}

	res, err := kClient.evict(thread, nil, args, kwargs)
	if err != nil {
		t.Fatalf("evict dry_run error: %v", err)
	}

	dict, ok := res.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", res)
	}
	if dict.data["evicted"] != true {
		t.Errorf("expected evicted=true, got %v", dict.data["evicted"])
	}
	if dict.data["name"] != "worker-pod" {
		t.Errorf("expected name=worker-pod, got %v", dict.data["name"])
	}
	if dict.data["dry_run"] != true {
		t.Errorf("expected dry_run=true, got %v", dict.data["dry_run"])
	}
	if !evictionCalled {
		t.Errorf("eviction subresource reactor was not called")
	}
	if len(recordedDryRun) == 0 || recordedDryRun[0] != metav1.DryRunAll {
		t.Errorf("expected DryRunAll, got %v", recordedDryRun)
	}

	// 2. Real eviction
	evictionCalled = false
	recordedDryRun = nil
	res2, err := kClient.evict(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("evict error: %v", err)
	}
	dict2 := res2.(*AttrDict)
	if dict2.data["dry_run"] != false {
		t.Errorf("expected dry_run=false, got %v", dict2.data["dry_run"])
	}
	if len(recordedDryRun) != 0 {
		t.Errorf("expected empty DryRun options, got %v", recordedDryRun)
	}
}

func TestEvict_MissingName(t *testing.T) {
	kClient := &K8sClient{
		resolver:  NewResolver(nil),
		namespace: "default",
	}
	thread := &starlark.Thread{Name: "test-thread"}
	_, err := kClient.evict(thread, nil, starlark.Tuple{}, nil)
	if err == nil {
		t.Fatalf("expected error for missing name, got nil")
	}
}
