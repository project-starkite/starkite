package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.starlark.net/starlark"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
)

func TestControlSelfEchoSuppression(t *testing.T) {
	c := &controller{
		echoKeys: make(map[string]time.Time),
	}

	// Record self-echo from status update
	c.RecordSelfEcho("app-uid-123", "45678")

	// Verify echo is detected
	if !c.isSelfEcho("app-uid-123", "45678") {
		t.Fatal("expected isSelfEcho to return true for recorded echo")
	}

	// Verify echo is consumed (one-shot suppression)
	if c.isSelfEcho("app-uid-123", "45678") {
		t.Fatal("expected isSelfEcho to return false after consumption")
	}

	// Verify non-matching rv
	if c.isSelfEcho("app-uid-123", "99999") {
		t.Fatal("expected isSelfEcho to return false for different resourceVersion")
	}
}

func TestControlGenerationFilter(t *testing.T) {
	tests := []struct {
		name              string
		generationChanged bool
		oldGen            int64
		newGen            int64
		shouldEnqueue     bool
	}{
		{
			name:              "status-only update dropped when generationChanged is true",
			generationChanged: true,
			oldGen:            2,
			newGen:            2,
			shouldEnqueue:     false,
		},
		{
			name:              "spec update enqueued when generation changed",
			generationChanged: true,
			oldGen:            2,
			newGen:            3,
			shouldEnqueue:     true,
		},
		{
			name:              "update enqueued when generationChanged is false",
			generationChanged: false,
			oldGen:            2,
			newGen:            2,
			shouldEnqueue:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())
			defer queue.ShutDown()

			c := &controller{
				generationChanged: tt.generationChanged,
				queue:             queue,
				cache:             make(map[string]*unstructured.Unstructured),
			}

			key := "default/test-app"
			oldObj := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "example.io/v1",
					"kind":       "TestApp",
					"metadata": map[string]any{
						"name":       "test-app",
						"namespace":  "default",
						"generation": tt.oldGen,
					},
				},
			}
			c.cache[key] = oldObj

			newObj := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "example.io/v1",
					"kind":       "TestApp",
					"metadata": map[string]any{
						"name":       "test-app",
						"namespace":  "default",
						"generation": tt.newGen,
					},
				},
			}

			// Simulate watch event logic
			c.cacheMu.Lock()
			cachedOld := c.cache[key]
			c.cache[key] = newObj.DeepCopy()
			c.cacheMu.Unlock()

			dropped := false
			if c.generationChanged {
				if cachedOld != nil && newObj.GetGeneration() != 0 && cachedOld.GetGeneration() == newObj.GetGeneration() {
					dropped = true
				}
			}

			if !dropped {
				c.queue.Add(queueItem{key: key, eventType: "MODIFIED", old: cachedOld})
			}

			enqueued := c.queue.Len() > 0
			if enqueued != tt.shouldEnqueue {
				t.Fatalf("queue len = %d (enqueued=%v), want shouldEnqueue=%v", c.queue.Len(), enqueued, tt.shouldEnqueue)
			}
		})
	}
}

func TestControlAutoWatch(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	childGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	ctx := t.Context()

	c := &controller{
		ctx:         ctx,
		gvr:         parentGVR,
		watchedGVRs: make(map[schema.GroupVersionResource]context.CancelFunc),
	}

	// AutoWatch child GVR
	c.AutoWatch(childGVR)
	c.autoWatchMu.Lock()
	_, exists := c.watchedGVRs[childGVR]
	c.autoWatchMu.Unlock()
	if !exists {
		t.Fatal("expected childGVR to be registered in watchedGVRs")
	}

	// AutoWatch primary GVR should be ignored
	c.AutoWatch(parentGVR)
	c.autoWatchMu.Lock()
	_, parentExists := c.watchedGVRs[parentGVR]
	c.autoWatchMu.Unlock()
	if parentExists {
		t.Fatal("primary GVR should not be added to watchedGVRs")
	}
}

func TestControlReconcileFunctionSignatures(t *testing.T) {
	thread := &starlark.Thread{Name: "test-thread"}

	// 1-argument reconcile(cr)
	starSrc1 := `
def reconcile(cr):
    return "reconciled-" + cr.metadata.name
`
	globals1, err := starlark.ExecFile(thread, "test1.star", starSrc1, nil)
	if err != nil {
		t.Fatalf("ExecFile 1: %v", err)
	}
	reconcile1 := globals1["reconcile"].(starlark.Callable)

	ctrl1 := &controller{reconcileFn: reconcile1}
	testObj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "my-app",
			},
		},
	}
	attrDict := unstructuredToAttrDict(testObj)

	res1, err := ctrl1.callReconcile(thread, "MODIFIED", attrDict)
	if err != nil {
		t.Fatalf("callReconcile 1-arg failed: %v", err)
	}
	if str, ok := starlark.AsString(res1); !ok || str != "reconciled-my-app" {
		t.Fatalf("callReconcile 1-arg got %v, want reconciled-my-app", res1)
	}

	// 2-argument reconcile(event, cr)
	starSrc2 := `
def reconcile(event, cr):
    return event + ":" + cr.metadata.name
`
	globals2, err := starlark.ExecFile(thread, "test2.star", starSrc2, nil)
	if err != nil {
		t.Fatalf("ExecFile 2: %v", err)
	}
	reconcile2 := globals2["reconcile"].(starlark.Callable)

	ctrl2 := &controller{reconcileFn: reconcile2}
	res2, err := ctrl2.callReconcile(thread, "ADDED", attrDict)
	if err != nil {
		t.Fatalf("callReconcile 2-arg failed: %v", err)
	}
	if str, ok := starlark.AsString(res2); !ok || str != "ADDED:my-app" {
		t.Fatalf("callReconcile 2-arg got %v, want ADDED:my-app", res2)
	}
}

func TestControlReconcileDesiredChildrenAndOrphanPruning(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	svcGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

	scheme := runtime.NewScheme()
	customListKinds := map[schema.GroupVersionResource]string{
		parentGVR: "TestAppList",
		deployGVR: "DeploymentList",
		svcGVR:    "ServiceList",
	}
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, customListKinds)

	// Simulate Server-Side Apply in fake client
	fakeClient.PrependReactor("patch", "*", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		patchAction := action.(clienttesting.PatchAction)
		var obj unstructured.Unstructured
		if err := json.Unmarshal(patchAction.GetPatch(), &obj.Object); err != nil {
			return true, nil, err
		}
		tracker := fakeClient.Tracker()
		gvr := patchAction.GetResource()
		ns := patchAction.GetNamespace()
		_, getErr := tracker.Get(gvr, ns, patchAction.GetName())
		if getErr != nil {
			_ = tracker.Create(gvr, &obj, ns)
		} else {
			_ = tracker.Update(gvr, &obj, ns)
		}
		return true, &obj, nil
	})

	ctx := t.Context()

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	c := &controller{
		kind:        "TestApp",
		gvr:         parentGVR,
		namespaced:  true,
		namespace:   "default",
		client:      kClient,
		ctx:         ctx,
		watchedGVRs: make(map[schema.GroupVersionResource]context.CancelFunc),
	}

	// Pre-register deployGVR and svcGVR in watchedGVRs
	c.watchedGVRs[deployGVR] = func() {}
	c.watchedGVRs[svcGVR] = func() {}

	parentUID := "parent-uuid-999"
	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":      "test-app",
				"namespace": "default",
				"uid":       parentUID,
			},
		},
	}

	// Pre-create an orphan Service owned by parent in the fake dynamic client
	orphanSvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name":      "test-app-svc",
				"namespace": "default",
				"ownerReferences": []any{
					map[string]any{
						"apiVersion": "example.io/v1",
						"kind":       "TestApp",
						"name":       "test-app",
						"uid":        parentUID,
					},
				},
			},
		},
	}
	_, err := fakeClient.Resource(svcGVR).Namespace("default").Create(ctx, orphanSvc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create initial orphan service: %v", err)
	}

	// Reconciler returns ONLY a Deployment (Service should be pruned!)
	deployDict := starlark.NewDict(4)
	_ = deployDict.SetKey(starlark.String("apiVersion"), starlark.String("apps/v1"))
	_ = deployDict.SetKey(starlark.String("kind"), starlark.String("Deployment"))
	metaDict := starlark.NewDict(2)
	_ = metaDict.SetKey(starlark.String("name"), starlark.String("test-app-deploy"))
	_ = metaDict.SetKey(starlark.String("namespace"), starlark.String("default"))
	_ = deployDict.SetKey(starlark.String("metadata"), metaDict)

	childrenList := starlark.NewList([]starlark.Value{deployDict})

	// Execute reconcileDesiredChildren
	err = c.reconcileDesiredChildren(parentObj, childrenList)
	if err != nil {
		t.Fatalf("reconcileDesiredChildren failed: %v", err)
	}

	// 1. Verify Deployment was applied with ownerReference
	appliedDeploy, err := fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "test-app-deploy", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected test-app-deploy to exist: %v", err)
	}
	ownerRefs := appliedDeploy.GetOwnerReferences()
	if len(ownerRefs) == 0 || ownerRefs[0].UID != "parent-uuid-999" {
		t.Fatalf("expected ownerReference to parent, got: %+v", ownerRefs)
	}
	if ownerRefs[0].Controller == nil || !*ownerRefs[0].Controller {
		t.Fatalf("expected controller=true on ownerReference")
	}

	// 2. Verify orphan Service was pruned
	_, err = fakeClient.Resource(svcGVR).Namespace("default").Get(ctx, "test-app-svc", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected orphan service test-app-svc to be deleted/pruned, but it still exists")
	}
}

func setupControlFakeClient(scheme *runtime.Scheme, customListKinds map[schema.GroupVersionResource]string) *dynamicfake.FakeDynamicClient {
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, customListKinds)

	fakeClient.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(clienttesting.PatchAction)
		gvr := patchAction.GetResource()
		ns := patchAction.GetNamespace()
		tracker := fakeClient.Tracker()

		obj, err := tracker.Get(gvr, ns, patchAction.GetName())
		if err != nil {
			return true, nil, err
		}
		u := obj.(*unstructured.Unstructured).DeepCopy()

		var patchMap map[string]any
		if err := json.Unmarshal(patchAction.GetPatch(), &patchMap); err != nil {
			return true, nil, err
		}

		if metaPatch, ok := patchMap["metadata"].(map[string]any); ok {
			if finVal, ok := metaPatch["finalizers"]; ok {
				if finSlice, ok := finVal.([]any); ok {
					var fins []string
					for _, f := range finSlice {
						fins = append(fins, f.(string))
					}
					u.SetFinalizers(fins)
				} else if finVal == nil {
					u.SetFinalizers(nil)
				}
			}
		}

		if err := tracker.Update(gvr, u, ns); err != nil {
			return true, nil, err
		}
		return true, u, nil
	})

	fakeClient.PrependReactor("update", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "status" {
			updateAction := action.(clienttesting.UpdateAction)
			u := updateAction.GetObject().(*unstructured.Unstructured)
			gvr := updateAction.GetResource()
			ns := updateAction.GetNamespace()
			if err := fakeClient.Tracker().Update(gvr, u, ns); err != nil {
				return true, nil, err
			}
			return true, u, nil
		}
		return false, nil, nil
	})

	return fakeClient
}

func TestControlAutomaticFinalizerInjection(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		parentGVR: "TestAppList",
	})
	ctx := t.Context()

	// Active parent object with NO finalizers
	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":       "test-app",
				"namespace":  "default",
				"uid":        "parent-uuid-1",
				"generation": int64(1),
			},
		},
	}
	_, err := fakeClient.Resource(parentGVR).Namespace("default").Create(ctx, parentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test parent: %v", err)
	}

	var reconciledNames []string
	var finalizedNames []string
	predeclared := starlark.StringDict{
		"record_reconcile": starlark.NewBuiltin("record_reconcile", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			reconciledNames = append(reconciledNames, name)
			return starlark.None, nil
		}),
		"record_finalize": starlark.NewBuiltin("record_finalize", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			finalizedNames = append(finalizedNames, name)
			return starlark.None, nil
		}),
	}
	thread := &starlark.Thread{Name: "test-thread"}
	starSrc := `
def reconcile(cr):
    record_reconcile(cr.metadata.name)
def finalize(cr):
    record_finalize(cr.metadata.name)
`
	globals, err := starlark.ExecFile(thread, "test_finalize.star", starSrc, predeclared)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	c := &controller{
		kind:          "TestApp",
		gvr:           parentGVR,
		namespaced:    true,
		namespace:     "default",
		finalizerName: "testapps.example.io/finalizer",
		finalizeFn:    globals["finalize"].(starlark.Callable),
		reconcileFn:   globals["reconcile"].(starlark.Callable),
		client: &K8sClient{
			dynClient: fakeClient,
			namespace: "default",
		},
		ctx:      ctx,
		cache:    map[string]*unstructured.Unstructured{"default/test-app": parentObj.DeepCopy()},
		echoKeys: make(map[string]time.Time),
	}

	// Dispatch ADDED item
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "ADDED"})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	// 1. Verify finalizer was injected into the stored object
	gotObj, err := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get parent after dispatch: %v", err)
	}
	fins := gotObj.GetFinalizers()
	if len(fins) != 1 || fins[0] != "testapps.example.io/finalizer" {
		t.Fatalf("expected finalizers [testapps.example.io/finalizer], got: %v", fins)
	}

	// 2. Verify reconcile ran and finalize did NOT run
	if len(reconciledNames) != 1 || reconciledNames[0] != "test-app" {
		t.Fatalf("expected reconcile to be called once with test-app, got %v", reconciledNames)
	}
	if len(finalizedNames) != 0 {
		t.Fatalf("expected finalize to NOT be called for active resource, got %d calls", len(finalizedNames))
	}

	// 3. Verify Ready condition was automatically set to True
	conds, _, _ := unstructured.NestedSlice(gotObj.Object, "status", "conditions")
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %v", conds)
	}
	cond := conds[0].(map[string]any)
	if cond["type"] != "Ready" || cond["status"] != "True" || cond["reason"] != "Reconciled" {
		t.Fatalf("unexpected ready condition: %+v", cond)
	}
}

func TestControlFinalizeExecutionAndStripping(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		parentGVR: "TestAppList",
	})
	ctx := t.Context()

	now := metav1.Now()
	// Deleting parent object with finalizer present
	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":              "test-app",
				"namespace":         "default",
				"uid":               "parent-uuid-2",
				"generation":        int64(2),
				"deletionTimestamp": now.Format(time.RFC3339),
				"finalizers": []any{
					"testapps.example.io/finalizer",
				},
			},
		},
	}
	_, err := fakeClient.Resource(parentGVR).Namespace("default").Create(ctx, parentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test parent: %v", err)
	}

	var reconciledNames []string
	var finalizedNames []string
	predeclared := starlark.StringDict{
		"record_reconcile": starlark.NewBuiltin("record_reconcile", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			reconciledNames = append(reconciledNames, name)
			return starlark.None, nil
		}),
		"record_finalize": starlark.NewBuiltin("record_finalize", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			finalizedNames = append(finalizedNames, name)
			return starlark.None, nil
		}),
	}
	thread := &starlark.Thread{Name: "test-thread"}
	starSrc := `
def reconcile(cr):
    record_reconcile(cr.metadata.name)
def finalize(cr):
    record_finalize(cr.metadata.name)
`
	globals, err := starlark.ExecFile(thread, "test_finalize2.star", starSrc, predeclared)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	c := &controller{
		kind:          "TestApp",
		gvr:           parentGVR,
		namespaced:    true,
		namespace:     "default",
		finalizerName: "testapps.example.io/finalizer",
		finalizeFn:    globals["finalize"].(starlark.Callable),
		reconcileFn:   globals["reconcile"].(starlark.Callable),
		client: &K8sClient{
			dynClient: fakeClient,
			namespace: "default",
		},
		ctx:      ctx,
		cache:    map[string]*unstructured.Unstructured{"default/test-app": parentObj.DeepCopy()},
		echoKeys: make(map[string]time.Time),
	}

	// Dispatch MODIFIED item on deleting object
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	// 1. Verify finalize was called and reconcile was NOT called
	if len(finalizedNames) != 1 || finalizedNames[0] != "test-app" {
		t.Fatalf("expected finalize to be called once with test-app, got %v", finalizedNames)
	}
	if len(reconciledNames) != 0 {
		t.Fatalf("expected reconcile to NOT be called on deleting resource, got %d", len(reconciledNames))
	}

	// 2. Verify finalizer was stripped cleanly
	gotObj, err := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get parent after dispatch: %v", err)
	}
	if len(gotObj.GetFinalizers()) != 0 {
		t.Fatalf("expected finalizers to be empty after finalize, got: %v", gotObj.GetFinalizers())
	}
}

func TestControlFinalizeErrorRetention(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		parentGVR: "TestAppList",
	})
	ctx := t.Context()

	now := metav1.Now()
	// Deleting parent object with finalizer present
	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":              "test-app",
				"namespace":         "default",
				"uid":               "parent-uuid-3",
				"generation":        int64(2),
				"deletionTimestamp": now.Format(time.RFC3339),
				"finalizers": []any{
					"testapps.example.io/finalizer",
				},
			},
		},
	}
	_, err := fakeClient.Resource(parentGVR).Namespace("default").Create(ctx, parentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test parent: %v", err)
	}

	thread := &starlark.Thread{Name: "test-thread"}
	starSrc := `
def finalize(cr):
    fail("external dependency cleanup failed")
`
	globals, err := starlark.ExecFile(thread, "test_finalize_err.star", starSrc, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	c := &controller{
		kind:          "TestApp",
		gvr:           parentGVR,
		namespaced:    true,
		namespace:     "default",
		finalizerName: "testapps.example.io/finalizer",
		finalizeFn:    globals["finalize"].(starlark.Callable),
		client: &K8sClient{
			dynClient: fakeClient,
			namespace: "default",
		},
		ctx:      ctx,
		cache:    map[string]*unstructured.Unstructured{"default/test-app": parentObj.DeepCopy()},
		echoKeys: make(map[string]time.Time),
	}

	// Dispatch item on deleting object — finalize will fail
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err == nil {
		t.Fatal("expected dispatch error when finalize fails, got nil")
	}

	// 1. Verify finalizer was RETAINED (not stripped)
	gotObj, err := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get parent after dispatch: %v", err)
	}
	fins := gotObj.GetFinalizers()
	if len(fins) != 1 || fins[0] != "testapps.example.io/finalizer" {
		t.Fatalf("expected finalizer to be retained, got: %v", fins)
	}

	// 2. Verify Ready condition set to False with FinalizeError
	conds, _, _ := unstructured.NestedSlice(gotObj.Object, "status", "conditions")
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %v", conds)
	}
	cond := conds[0].(map[string]any)
	if cond["type"] != "Ready" || cond["status"] != "False" || cond["reason"] != "FinalizeError" {
		t.Fatalf("unexpected ready condition: %+v", cond)
	}
}

func TestControlReadyConditionInference(t *testing.T) {
	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		parentGVR: "TestAppList",
	})
	ctx := t.Context()

	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":       "test-app",
				"namespace":  "default",
				"uid":        "parent-uuid-4",
				"generation": int64(1),
			},
		},
	}
	_, err := fakeClient.Resource(parentGVR).Namespace("default").Create(ctx, parentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test parent: %v", err)
	}

	shouldFail := false
	predeclared := starlark.StringDict{
		"should_fail": starlark.NewBuiltin("should_fail", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return starlark.Bool(shouldFail), nil
		}),
	}

	thread := &starlark.Thread{Name: "test-thread"}
	starSrc := `
def reconcile(cr):
    if should_fail():
        fail("reconciliation exploded")
    return None
`
	globals, err := starlark.ExecFile(thread, "test_ready.star", starSrc, predeclared)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	c := &controller{
		kind:        "TestApp",
		gvr:         parentGVR,
		namespaced:  true,
		namespace:   "default",
		reconcileFn: globals["reconcile"].(starlark.Callable),
		client: &K8sClient{
			dynClient: fakeClient,
			namespace: "default",
		},
		ctx:      ctx,
		cache:    map[string]*unstructured.Unstructured{"default/test-app": parentObj.DeepCopy()},
		echoKeys: make(map[string]time.Time),
	}

	// 1. First reconciliation: should set Ready=True
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "ADDED"})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	got1, _ := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	conds1, _, _ := unstructured.NestedSlice(got1.Object, "status", "conditions")
	if len(conds1) != 1 {
		t.Fatalf("expected 1 condition, got %v", conds1)
	}
	cond1 := conds1[0].(map[string]any)
	if cond1["status"] != "True" || cond1["reason"] != "Reconciled" {
		t.Fatalf("expected Ready=True/Reconciled, got: %+v", cond1)
	}
	t1 := cond1["lastTransitionTime"].(string)
	if t1 == "" {
		t.Fatal("expected non-empty lastTransitionTime")
	}

	// 2. Second reconciliation (success again): lastTransitionTime must be PRESERVED
	time.Sleep(10 * time.Millisecond)
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err != nil {
		t.Fatalf("dispatch 2 error: %v", err)
	}
	got2, _ := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	conds2, _, _ := unstructured.NestedSlice(got2.Object, "status", "conditions")
	cond2 := conds2[0].(map[string]any)
	t2 := cond2["lastTransitionTime"].(string)
	if t2 != t1 {
		t.Fatalf("lastTransitionTime changed when status was unchanged: got %s, want %s", t2, t1)
	}

	// 3. Third reconciliation: trigger error -> Ready=False, reason="ReconcileError"
	shouldFail = true
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err == nil {
		t.Fatal("expected error on failed reconcile")
	}
	got3, _ := fakeClient.Resource(parentGVR).Namespace("default").Get(ctx, "test-app", metav1.GetOptions{})
	conds3, _, _ := unstructured.NestedSlice(got3.Object, "status", "conditions")
	cond3 := conds3[0].(map[string]any)
	if cond3["status"] != "False" || cond3["reason"] != "ReconcileError" {
		t.Fatalf("expected Ready=False/ReconcileError, got: %+v", cond3)
	}
}

func TestControlLifecycleEventEmission(t *testing.T) {
	ctx := t.Context()

	parentGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "testapps"}
	scheme := runtime.NewScheme()

	parentObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.io/v1",
			"kind":       "TestApp",
			"metadata": map[string]any{
				"name":            "test-app",
				"namespace":       "default",
				"uid":             "app-12345",
				"resourceVersion": "1",
				"generation":      int64(1),
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}

	fakeDynClient := dynamicfake.NewSimpleDynamicClient(scheme, parentObj)
	fakeClientset := fake.NewSimpleClientset()

	var shouldFail bool
	thread := &starlark.Thread{Name: "test-thread"}
	predeclared := starlark.StringDict{
		"should_fail": starlark.NewBuiltin("should_fail", func(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return starlark.Bool(shouldFail), nil
		}),
	}
	starSrc := `
def reconcile(cr):
    if should_fail():
        fail("simulated reconcile failure")
    return None

def finalize(cr):
    if should_fail():
        fail("simulated finalize failure")
    return None
`
	globals, err := starlark.ExecFile(thread, "test_events.star", starSrc, predeclared)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	c := &controller{
		kind:          "TestApp",
		gvr:           parentGVR,
		namespaced:    true,
		namespace:     "default",
		finalizerName: "example.io/finalizer",
		finalizeFn:    globals["finalize"].(starlark.Callable),
		reconcileFn:   globals["reconcile"].(starlark.Callable),
		client: &K8sClient{
			dynClient: fakeDynClient,
			clientset: fakeClientset,
			namespace: "default",
		},
		ctx:      ctx,
		cache:    map[string]*unstructured.Unstructured{"default/test-app": parentObj.DeepCopy()},
		echoKeys: make(map[string]time.Time),
	}

	// 1. Normal reconcile success -> Event: Normal Reconciled
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "ADDED"})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	events, err := fakeClientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List events error: %v", err)
	}
	if len(events.Items) == 0 {
		t.Fatal("expected at least 1 event to be emitted")
	}
	lastEvt := events.Items[len(events.Items)-1]
	if lastEvt.Reason != "Reconciled" || lastEvt.Type != corev1.EventTypeNormal {
		t.Fatalf("expected Normal/Reconciled, got %s/%s", lastEvt.Type, lastEvt.Reason)
	}
	if lastEvt.InvolvedObject.Name != "test-app" || lastEvt.InvolvedObject.Kind != "TestApp" {
		t.Fatalf("unexpected involvedObject: %+v", lastEvt.InvolvedObject)
	}

	// 2. Reconcile failure -> Event: Warning ReconcileError
	shouldFail = true
	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err == nil {
		t.Fatal("expected error on failed reconcile")
	}
	events, _ = fakeClientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	lastEvt = events.Items[len(events.Items)-1]
	if lastEvt.Reason != "ReconcileError" || lastEvt.Type != corev1.EventTypeWarning {
		t.Fatalf("expected Warning/ReconcileError, got %s/%s", lastEvt.Type, lastEvt.Reason)
	}

	// 3. Teardown with finalizer: trigger teardown
	shouldFail = false
	deletingObj := parentObj.DeepCopy()
	now := metav1.Now()
	deletingObj.SetDeletionTimestamp(&now)
	deletingObj.SetFinalizers([]string{"example.io/finalizer"})
	c.cache["default/test-app"] = deletingObj
	_, _ = fakeDynClient.Resource(parentGVR).Namespace("default").Update(ctx, deletingObj, metav1.UpdateOptions{})

	err = c.dispatch(queueItem{key: "default/test-app", eventType: "MODIFIED"})
	if err != nil {
		t.Fatalf("dispatch teardown error: %v", err)
	}
	events, _ = fakeClientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{})
	var foundFinalizing, foundFinalized bool
	for _, e := range events.Items {
		if e.Reason == "Finalizing" && e.Type == corev1.EventTypeNormal {
			foundFinalizing = true
		}
		if e.Reason == "Finalized" && e.Type == corev1.EventTypeNormal {
			foundFinalized = true
		}
	}
	if !foundFinalizing {
		t.Fatal("expected Finalizing event")
	}
	if !foundFinalized {
		t.Fatal("expected Finalized event")
	}
}

func TestControlHealthServer(t *testing.T) {
	c := &controller{
		healthPort: 8081,
	}
	handler := c.healthHandler()

	// 1. /healthz -> 200 OK
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /healthz code = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "ok\n" && rr.Body.String() != "ok" {
		t.Fatalf("GET /healthz body = %q", rr.Body.String())
	}

	// 2. /readyz without leader election -> 200 OK
	c.enableLeaderElection = false
	req = httptest.NewRequest("GET", "/readyz", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /readyz (no leader election) code = %d, want 200", rr.Code)
	}

	// 3. /readyz with leader election enabled and not leading -> 503
	c.enableLeaderElection = true
	c.leading.Store(false)
	req = httptest.NewRequest("GET", "/readyz", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz (standby) code = %d, want 503", rr.Code)
	}

	// 4. /readyz with leader election enabled and leading -> 200
	c.leading.Store(true)
	req = httptest.NewRequest("GET", "/readyz", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /readyz (leading) code = %d, want 200", rr.Code)
	}

	// 5. unknown route -> 404
	req = httptest.NewRequest("GET", "/unknown", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /unknown code = %d, want 404", rr.Code)
	}
}

func TestControlEnqueueMappedKeys(t *testing.T) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())
	defer queue.ShutDown()

	c := &controller{
		namespaced: true,
		namespace:  "prod",
		queue:      queue,
	}

	// 1. String key with namespace
	c.enqueueMappedKeys(starlark.String("default/app-a"))
	// 2. String key bare name (should prepend controller namespace)
	c.enqueueMappedKeys(starlark.String("app-b"))
	// 3. List of strings
	c.enqueueMappedKeys(starlark.NewList([]starlark.Value{
		starlark.String("staging/app-c"),
		starlark.String("app-d"),
	}))
	// 4. Dict with name & namespace
	d1 := starlark.NewDict(2)
	_ = d1.SetKey(starlark.String("name"), starlark.String("app-e"))
	_ = d1.SetKey(starlark.String("namespace"), starlark.String("infra"))
	c.enqueueMappedKeys(d1)
	// 5. Dict with name only
	d2 := starlark.NewDict(1)
	_ = d2.SetKey(starlark.String("name"), starlark.String("app-f"))
	c.enqueueMappedKeys(d2)
	// 6. AttrDict
	ad := NewAttrDict(map[string]any{"name": "app-g", "namespace": "custom"})
	c.enqueueMappedKeys(ad)
	// 7. None -> should not enqueue
	c.enqueueMappedKeys(starlark.None)

	expectedKeys := []string{
		"default/app-a",
		"prod/app-b",
		"staging/app-c",
		"prod/app-d",
		"infra/app-e",
		"prod/app-f",
		"custom/app-g",
	}

	for _, want := range expectedKeys {
		if queue.Len() == 0 {
			t.Fatalf("expected queue to contain %s, but queue is empty", want)
		}
		item, _ := queue.Get()
		queue.Done(item)
		qItem := item.(queueItem)
		if qItem.key != want {
			t.Fatalf("queue item key = %s, want %s", qItem.key, want)
		}
	}

	if queue.Len() != 0 {
		t.Fatalf("expected queue to be empty, got len %d", queue.Len())
	}
}

func TestControlWatchRelatedMapping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	secretGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	fakeWatcher := watch.NewFake()
	scheme := runtime.NewScheme()
	fakeDynClient := dynamicfake.NewSimpleDynamicClient(scheme)
	fakeDynClient.PrependWatchReactor("secrets", func(action clienttesting.Action) (bool, watch.Interface, error) {
		return true, fakeWatcher, nil
	})

	thread := &starlark.Thread{Name: "test-related-thread"}
	starSrc := `
def map_secret(secret):
    name = secret.metadata.name
    if name.startswith("app-config-"):
        app_name = name[len("app-config-"):]
        return "default/" + app_name
    return None
`
	globals, err := starlark.ExecFile(thread, "test_related.star", starSrc, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())
	defer queue.ShutDown()

	c := &controller{
		kind:       "TestApp",
		namespaced: true,
		namespace:  "default",
		queue:      queue,
		client: &K8sClient{
			dynClient: fakeDynClient,
			namespace: "default",
		},
		ctx:    ctx,
		thread: thread,
	}

	relWatcher := relatedWatcher{
		kind:       "secrets",
		gvr:        secretGVR,
		namespaced: true,
		mapFn:      globals["map_secret"].(starlark.Callable),
	}

	// Run doWatchRelated in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.doWatchRelated(relWatcher)
	}()

	// Inject a secret event
	secretObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "app-config-my-service",
				"namespace": "default",
			},
		},
	}
	fakeWatcher.Modify(secretObj)

	// Verify primary resource key "default/my-service" is enqueued
	deadline := time.Now().Add(2 * time.Second)
	for queue.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if queue.Len() == 0 {
		t.Fatal("timed out waiting for mapped key in queue")
	}

	item, _ := queue.Get()
	queue.Done(item)
	qItem := item.(queueItem)
	if qItem.key != "default/my-service" {
		t.Fatalf("enqueued key = %s, want default/my-service", qItem.key)
	}

	cancel()
	fakeWatcher.Stop()
	<-errCh
}

func TestK8sClientEvent(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	client := &K8sClient{
		clientset: fakeClientset,
		namespace: "default",
	}

	thread := &starlark.Thread{Name: "test-event"}
	targetObj := NewAttrDict(map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "TestApp",
		"metadata": map[string]any{
			"name":      "my-app",
			"namespace": "default",
			"uid":       "12345",
		},
	})

	args := starlark.Tuple{
		targetObj,
		starlark.String("Synced"),
		starlark.String("Resource is in sync"),
	}
	kwargs := []starlark.Tuple{
		{starlark.String("type"), starlark.String("Normal")},
	}

	val, err := client.event(thread, nil, args, kwargs)
	if err != nil {
		t.Fatalf("client.event failed: %v", err)
	}
	evtDict, ok := val.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", val)
	}
	if evtDict.ToMap()["kind"] != "Event" {
		t.Fatalf("expected Event kind, got %v", evtDict.ToMap()["kind"])
	}

	events, err := fakeClientset.CoreV1().Events("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List events error: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.Items))
	}
	e := events.Items[0]
	if e.Reason != "Synced" || e.Message != "Resource is in sync" || e.Type != corev1.EventTypeNormal {
		t.Fatalf("unexpected event: %+v", e)
	}
	if e.InvolvedObject.Name != "my-app" || e.InvolvedObject.Kind != "TestApp" {
		t.Fatalf("unexpected involvedObject: %+v", e.InvolvedObject)
	}
}
