package k8s

import (
	"testing"
	"time"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLifecycleIsDeleting(t *testing.T) {
	m := New()
	thread := &starlark.Thread{Name: "test-thread"}

	tests := []struct {
		name     string
		obj      starlark.Value
		expected bool
	}{
		{
			name:     "plain dict without metadata",
			obj:      starlark.NewDict(0),
			expected: false,
		},
		{
			name: "dict without deletionTimestamp",
			obj: func() starlark.Value {
				d := starlark.NewDict(1)
				meta := starlark.NewDict(1)
				_ = meta.SetKey(starlark.String("name"), starlark.String("app"))
				_ = d.SetKey(starlark.String("metadata"), meta)
				return d
			}(),
			expected: false,
		},
		{
			name: "dict with deletionTimestamp",
			obj: func() starlark.Value {
				d := starlark.NewDict(1)
				meta := starlark.NewDict(1)
				_ = meta.SetKey(starlark.String("deletionTimestamp"), starlark.String("2026-09-06T12:00:00Z"))
				_ = d.SetKey(starlark.String("metadata"), meta)
				return d
			}(),
			expected: true,
		},
		{
			name: "AttrDict with deletionTimestamp",
			obj: func() starlark.Value {
				u := &unstructured.Unstructured{
					Object: map[string]any{
						"metadata": map[string]any{
							"deletionTimestamp": "2026-09-06T12:00:00Z",
						},
					},
				}
				return unstructuredToAttrDict(u)
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := m.isDeleting(thread, nil, starlark.Tuple{tt.obj}, nil)
			if err != nil {
				t.Fatalf("isDeleting failed: %v", err)
			}
			b, ok := res.(starlark.Bool)
			if !ok {
				t.Fatalf("expected bool, got %T", res)
			}
			if bool(b) != tt.expected {
				t.Fatalf("got %v, want %v", b, tt.expected)
			}
		})
	}
}

func TestLifecycleFinalizerHas(t *testing.T) {
	m := New()
	thread := &starlark.Thread{Name: "test-thread"}

	u := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"finalizers": []any{
					"example.com/custom-fin",
				},
			},
		},
	}
	attrDict := unstructuredToAttrDict(u)

	// Has existing finalizer
	res, err := m.finalizerHas(thread, nil, starlark.Tuple{attrDict, starlark.String("example.com/custom-fin")}, nil)
	if err != nil {
		t.Fatalf("finalizerHas failed: %v", err)
	}
	if b, ok := res.(starlark.Bool); !ok || !bool(b) {
		t.Fatalf("expected true, got %v", res)
	}

	// Missing finalizer
	res, err = m.finalizerHas(thread, nil, starlark.Tuple{attrDict, starlark.String("example.com/missing")}, nil)
	if err != nil {
		t.Fatalf("finalizerHas failed: %v", err)
	}
	if b, ok := res.(starlark.Bool); !ok || bool(b) {
		t.Fatalf("expected false, got %v", res)
	}
}

func TestLifecycleFinalizerAddAndRemove(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
	})
	ctx := t.Context()

	initialObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":       "test-deploy",
				"namespace":  "default",
				"uid":        "uid-lifecycle-1",
				"generation": int64(1),
			},
		},
	}
	_, err := fakeClient.Resource(deployGVR).Namespace("default").Create(ctx, initialObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create initial obj: %v", err)
	}

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	ctrl := &controller{
		echoKeys: make(map[string]time.Time),
	}
	thread := &starlark.Thread{Name: "test-thread"}
	thread.SetLocal(ActiveControllerKey, ctrl)

	attrDict := unstructuredToAttrDict(initialObj)

	// 1. Add finalizer
	addedVal, err := kClient.finalizerAdd(thread, nil, starlark.Tuple{attrDict, starlark.String("example.com/protect")}, nil)
	if err != nil {
		t.Fatalf("finalizerAdd failed: %v", err)
	}
	if addedVal == nil {
		t.Fatal("expected returned value from finalizerAdd")
	}

	// Verify object has finalizer in fake dynamic client
	got1, err := fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "test-deploy", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get obj: %v", err)
	}
	fins := got1.GetFinalizers()
	if len(fins) != 1 || fins[0] != "example.com/protect" {
		t.Fatalf("expected finalizers [example.com/protect], got %v", fins)
	}

	// 2. Remove finalizer
	updatedDict := unstructuredToAttrDict(got1)
	removedVal, err := kClient.finalizerRemove(thread, nil, starlark.Tuple{updatedDict, starlark.String("example.com/protect")}, nil)
	if err != nil {
		t.Fatalf("finalizerRemove failed: %v", err)
	}
	if removedVal == nil {
		t.Fatal("expected returned value from finalizerRemove")
	}

	// Verify object has no finalizers
	got2, err := fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "test-deploy", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get obj: %v", err)
	}
	if len(got2.GetFinalizers()) != 0 {
		t.Fatalf("expected 0 finalizers, got %v", got2.GetFinalizers())
	}
}

func TestLifecycleConditionGetAndSet(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
	})
	ctx := t.Context()

	initialObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":       "test-deploy",
				"namespace":  "default",
				"uid":        "uid-lifecycle-2",
				"generation": int64(1),
			},
		},
	}
	_, err := fakeClient.Resource(deployGVR).Namespace("default").Create(ctx, initialObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create initial obj: %v", err)
	}

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	thread := &starlark.Thread{Name: "test-thread"}
	attrDict := unstructuredToAttrDict(initialObj)
	m := New()

	// 1. Condition get on empty conditions -> None
	gotNone, err := m.conditionGet(thread, nil, starlark.Tuple{attrDict, starlark.String("Ready")}, nil)
	if err != nil {
		t.Fatalf("conditionGet failed: %v", err)
	}
	if gotNone != starlark.None {
		t.Fatalf("expected None for missing condition, got %v", gotNone)
	}

	// 2. Condition set Ready=True
	kwargs := []starlark.Tuple{
		{starlark.String("reason"), starlark.String("Configured")},
		{starlark.String("message"), starlark.String("App is healthy")},
	}
	setVal, err := kClient.conditionSet(thread, nil, starlark.Tuple{attrDict, starlark.String("Ready"), starlark.String("True")}, kwargs)
	if err != nil {
		t.Fatalf("conditionSet failed: %v", err)
	}
	if setVal == nil {
		t.Fatal("expected returned value from conditionSet")
	}

	// 3. Condition get should now return the set condition
	gotCond, err := m.conditionGet(thread, nil, starlark.Tuple{setVal, starlark.String("Ready")}, nil)
	if err != nil {
		t.Fatalf("conditionGet failed: %v", err)
	}
	condDict, ok := gotCond.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", gotCond)
	}
	stVal, err := condDict.Attr("status")
	if err != nil || stVal != starlark.String("True") {
		t.Fatalf("expected status True, got %v", stVal)
	}
	rsVal, err := condDict.Attr("reason")
	if err != nil || rsVal != starlark.String("Configured") {
		t.Fatalf("expected reason Configured, got %v", rsVal)
	}
}
