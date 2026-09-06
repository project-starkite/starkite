package k8s

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
