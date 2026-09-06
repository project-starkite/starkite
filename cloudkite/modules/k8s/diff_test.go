package k8s

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDiff_LiveDrift(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
	})
	ctx := t.Context()

	// Initial live deployment in cluster (replicas=1, image="nginx:1.26")
	liveObj := &unstructuredObj{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "web",
				"namespace": "default",
			},
			"spec": map[string]any{
				"replicas": int64(1),
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name":  "web",
								"image": "nginx:1.26",
							},
						},
					},
				},
			},
		},
	}
	if _, err := fakeClient.Resource(deployGVR).Namespace("default").Create(ctx, liveObj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed live deployment: %v", err)
	}

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	thread := &starlark.Thread{Name: "test-thread"}

	// Manifest with desired changes: replicas=3, image="nginx:1.27"
	desiredManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: default
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: web
          image: nginx:1.27
`
	args := starlark.Tuple{starlark.String(desiredManifest)}
	res, err := kClient.diff(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("diff error: %v", err)
	}

	report, ok := res.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", res)
	}

	hasDrift, _ := report.data["has_drift"].(bool)
	if !hasDrift {
		t.Errorf("expected has_drift=true")
	}

	diffText, _ := report.data["diff"].(string)
	if diffText == "" {
		t.Errorf("expected non-empty diff string")
	}
	if !strings.Contains(diffText, "--- live") || !strings.Contains(diffText, "+++ applied") {
		t.Errorf("diff string missing headers:\n%s", diffText)
	}
}

func TestDiff_NewResource(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
	})

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	thread := &starlark.Thread{Name: "test-thread"}

	desiredManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: new-service
  namespace: default
spec:
  replicas: 2
`
	args := starlark.Tuple{starlark.String(desiredManifest)}
	res, err := kClient.diff(thread, nil, args, nil)
	if err != nil {
		t.Fatalf("diff error: %v", err)
	}

	report, ok := res.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict, got %T", res)
	}

	hasDrift, _ := report.data["has_drift"].(bool)
	if !hasDrift {
		t.Errorf("expected has_drift=true for new resource")
	}

	liveVal := report.data["live"]
	if liveVal != starlark.None {
		t.Errorf("expected live=None for new resource, got %v", liveVal)
	}
}

func TestApply_Prune(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	fakeClient := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
	})
	ctx := t.Context()

	// 1. Seed old deployment that should be pruned (managed by starkite, matching label)
	oldDeploy := &unstructuredObj{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "old-service",
				"namespace": "default",
				"labels": map[string]any{
					"app": "my-stack",
				},
				"managedFields": []any{
					map[string]any{
						"manager": "starkite",
					},
				},
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}
	if _, err := fakeClient.Resource(deployGVR).Namespace("default").Create(ctx, oldDeploy, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed old deploy: %v", err)
	}

	// 2. Seed unmanaged deployment with same label (managed by someone-else, should NOT be pruned)
	otherDeploy := &unstructuredObj{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "other-service",
				"namespace": "default",
				"labels": map[string]any{
					"app": "my-stack",
				},
				"managedFields": []any{
					map[string]any{
						"manager": "other-manager",
					},
				},
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}
	if _, err := fakeClient.Resource(deployGVR).Namespace("default").Create(ctx, otherDeploy, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to seed other deploy: %v", err)
	}

	resolver := NewResolver(nil)
	kClient := &K8sClient{
		dynClient: fakeClient,
		resolver:  resolver,
		namespace: "default",
	}

	thread := &starlark.Thread{Name: "test-thread"}

	// Apply manifest with ONLY kept-service (old-service is omitted)
	newManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kept-service
  namespace: default
  labels:
    app: my-stack
spec:
  replicas: 2
`
	args := starlark.Tuple{starlark.String(newManifest)}
	labelsDict := starlark.NewDict(1)
	_ = labelsDict.SetKey(starlark.String("app"), starlark.String("my-stack"))
	kwargs := []starlark.Tuple{
		{starlark.String("prune"), starlark.Bool(true)},
		{starlark.String("prune_labels"), labelsDict},
	}

	_, err := kClient.apply(thread, nil, args, kwargs)
	if err != nil {
		t.Fatalf("apply with prune failed: %v", err)
	}

	// Verify old-service is deleted
	_, err = fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "old-service", metav1.GetOptions{})
	if err == nil {
		t.Errorf("expected old-service to be pruned/deleted, but it still exists")
	}

	// Verify other-service is NOT deleted (different field manager)
	_, err = fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "other-service", metav1.GetOptions{})
	if err != nil {
		t.Errorf("expected other-service to remain, got err: %v", err)
	}

	// Verify kept-service exists
	_, err = fakeClient.Resource(deployGVR).Namespace("default").Get(ctx, "kept-service", metav1.GetOptions{})
	if err != nil {
		t.Errorf("expected kept-service to exist, got err: %v", err)
	}
}
