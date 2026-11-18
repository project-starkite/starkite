package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestK8sControl_EnforcesWritePermissionOnFakeDynamic(t *testing.T) {
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"k8s.read", "k8s.config"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}

	scheme := runtime.NewScheme()
	fakeDyn := dynamicfake.NewSimpleDynamicClient(scheme)
	mod := &Module{
		config: &libkite.ModuleConfig{},
		defaultClient: &K8sClient{
			dynClient: fakeDyn,
			resolver:  NewResolver(nil),
			namespace: "default",
		},
	}

	thread := &starlark.Thread{Name: "test-control-write-gate"}
	libkite.SetPermissions(thread, checker)

	reconcileFn := starlark.NewBuiltin("reconcile", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	_, err = mod.controlBuiltin(thread, nil, starlark.Tuple{starlark.String("Deployment")}, []starlark.Tuple{
		{starlark.String("reconcile"), reconcileFn},
	})
	if err == nil || !libkite.IsPermissionError(err) || !strings.Contains(err.Error(), "k8s.write") {
		t.Errorf("expected unconditional k8s.write PermissionError from k8s.control, got %v", err)
	}
}

func TestK8sExtractTarToLocal_RejectsSymlinkParentEscape(t *testing.T) {
	rootDir := t.TempDir()
	dstDir := filepath.Join(rootDir, "dst")
	outsideDir := filepath.Join(rootDir, "outside")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(dstDir, "escape")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	payload := []byte("owned")
	_ = tw.WriteHeader(&tar.Header{
		Name:     "escape/pwn.txt",
		Mode:     0644,
		Size:     int64(len(payload)),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(payload)
	_ = tw.Close()

	if _, _, err := extractTarToLocal(&buf, dstDir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink escape error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "pwn.txt")); !os.IsNotExist(err) {
		t.Error("expected outside file not to be created through symlinked parent dir")
	}
}

func TestK8sClient_GlobalDryRunPropagatesToMutators(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	nodeGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	fakeDyn := setupControlFakeClient(scheme, map[schema.GroupVersionResource]string{
		deployGVR: "DeploymentList",
		nodeGVR:   "NodeList",
		podGVR:    "PodList",
	})
	fakeCS := fake.NewSimpleClientset()

	seedObj := func(gvr schema.GroupVersionResource, ns string, obj map[string]any) {
		u := &unstructured.Unstructured{Object: obj}
		if ns != "" {
			if _, err := fakeDyn.Resource(gvr).Namespace(ns).Create(ctx, u, metav1.CreateOptions{}); err != nil {
				t.Fatalf("seed %v: %v", gvr, err)
			}
		} else {
			if _, err := fakeDyn.Resource(gvr).Create(ctx, u, metav1.CreateOptions{}); err != nil {
				t.Fatalf("seed %v: %v", gvr, err)
			}
		}
	}

	seedObj(deployGVR, "default", map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":       "old-deploy",
			"namespace":  "default",
			"labels":     map[string]any{"app": "stack"},
			"finalizers": []any{"example.io/keep"},
			"managedFields": []any{
				map[string]any{"manager": "starkite", "operation": "Apply"},
			},
		},
		"spec": map[string]any{
			"replicas": int64(1),
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{map[string]any{"name": "app", "image": "nginx:1"}},
				},
			},
		},
		"status": map[string]any{},
	})
	seedObj(nodeGVR, "", map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "node-1"},
		"spec":       map[string]any{},
	})
	seedObj(podGVR, "default", map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "p1", "namespace": "default"},
		"spec":       map[string]any{"nodeName": "node-1"},
	})

	fakeDyn.ClearActions()

	var recordedPatchDryRun [][]string
	var recordedDeleteDryRun [][]string
	var recordedUpdateDryRun [][]string

	fakeDyn.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if pa, ok := action.(clienttesting.PatchActionImpl); ok {
			recordedPatchDryRun = append(recordedPatchDryRun, pa.GetPatchOptions().DryRun)
		}
		return false, nil, nil
	})
	fakeDyn.PrependReactor("delete", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if da, ok := action.(clienttesting.DeleteAction); ok {
			recordedDeleteDryRun = append(recordedDeleteDryRun, da.GetDeleteOptions().DryRun)
		}
		return false, nil, nil
	})
	fakeDyn.PrependReactor("update", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if ua, ok := action.(clienttesting.UpdateActionImpl); ok {
			recordedUpdateDryRun = append(recordedUpdateDryRun, ua.GetUpdateOptions().DryRun)
		}
		return false, nil, nil
	})

	client := &K8sClient{
		dynClient: fakeDyn,
		clientset: fakeCS,
		resolver:  NewResolver(nil),
		namespace: "default",
		config:    &libkite.ModuleConfig{DryRun: true},
	}
	thread := &starlark.Thread{Name: "test-global-dryrun"}

	// 1. apply(prune=True) must not delete old-deploy under global DryRun
	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: new-deploy
  namespace: default
  labels:
    app: stack
spec:
  replicas: 1
`
	if _, err := client.apply(thread, nil, starlark.Tuple{starlark.String(manifest)}, []starlark.Tuple{
		{starlark.String("prune"), starlark.True},
	}); err != nil {
		t.Fatalf("apply dry_run failed: %v", err)
	}
	if _, err := fakeDyn.Resource(deployGVR).Namespace("default").Get(ctx, "old-deploy", metav1.GetOptions{}); err != nil {
		t.Errorf("expected old-deploy not to be pruned under global DryRun: %v", err)
	}

	// 2. Patch/Update/Delete mutators must propagate DryRunAll
	if _, err := client.patch(thread, nil, starlark.Tuple{
		starlark.String("deployment"),
		starlark.String("old-deploy"),
		starlark.String(`{"spec":{"replicas":2}}`),
	}, nil); err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	if _, err := client.scale(thread, nil, starlark.Tuple{
		starlark.String("deployment"),
		starlark.String("old-deploy"),
		starlark.MakeInt(3),
	}, nil); err != nil {
		t.Fatalf("scale failed: %v", err)
	}
	if _, err := client.cordon(thread, nil, starlark.Tuple{starlark.String("node-1")}, nil); err != nil {
		t.Fatalf("cordon failed: %v", err)
	}
	if _, err := client.drain(thread, nil, starlark.Tuple{starlark.String("node-1")}, nil); err != nil {
		t.Fatalf("drain failed: %v", err)
	}

	objDict, err := unstructuredToDict(&unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "old-deploy", "namespace": "default"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.updateStatus(thread, nil, starlark.Tuple{objDict, starlark.NewDict(0)}, nil); err != nil {
		t.Fatalf("updateStatus failed: %v", err)
	}
	if _, err := client.del(thread, nil, starlark.Tuple{starlark.String("deployment"), starlark.String("old-deploy")}, nil); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	for i, dr := range recordedPatchDryRun {
		if !slices.Contains(dr, metav1.DryRunAll) {
			t.Errorf("patch call %d missing DryRunAll: %v", i, dr)
		}
	}
	for i, dr := range recordedDeleteDryRun {
		if !slices.Contains(dr, metav1.DryRunAll) {
			t.Errorf("delete call %d missing DryRunAll: %v", i, dr)
		}
	}
	for i, dr := range recordedUpdateDryRun {
		if !slices.Contains(dr, metav1.DryRunAll) {
			t.Errorf("update call %d missing DryRunAll: %v", i, dr)
		}
	}
}

func TestK8sConfig_KubeconfigAndServerOverrideSecurity(t *testing.T) {
	kcDir := t.TempDir()
	kcPath := filepath.Join(kcDir, "config")
	kubeYAML := `apiVersion: v1
clusters:
- cluster:
    server: https://trusted.k8s.local:6443
    insecure-skip-tls-verify: true
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
users:
- name: default
  user:
    token: super-secret-bearer-token
`
	if err := os.WriteFile(kcPath, []byte(kubeYAML), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kcPath)

	// 1. Auto-discovered kubeconfig without fs.read must fail with PermissionError.
	denyFS, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"k8s.config"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	th1 := &starlark.Thread{Name: "test-kc-fs"}
	libkite.SetPermissions(th1, denyFS)
	mod := New()
	if _, err := mod.configFactory(th1, nil, nil, nil); err == nil || !libkite.IsPermissionError(err) || !strings.Contains(err.Error(), "fs.read") {
		t.Errorf("expected fs.read PermissionError on auto-discovered KUBECONFIG, got %v", err)
	}

	// 2. Overriding server= to a different cluster must strip ambient bearer token.
	th2 := &starlark.Thread{Name: "test-server-override"}
	kcClient, err := newK8sClient(th2, &libkite.ModuleConfig{}, clientOptions{
		kubeconfig: kcPath,
		server:     "https://untrusted.k8s.local:8443",
	})
	if err != nil {
		t.Fatalf("newK8sClient failed: %v", err)
	}
	if kcClient.restCfg.BearerToken != "" {
		t.Errorf("expected ambient BearerToken to be stripped when server= overrides cluster host, got %q", kcClient.restCfg.BearerToken)
	}

	// 3. ExecProvider in kubeconfig must check os.exec.
	execKC := filepath.Join(kcDir, "exec-config")
	execYAML := `apiVersion: v1
clusters:
- cluster:
    server: https://trusted.k8s.local:6443
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
users:
- name: default
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: /usr/local/bin/evil-cred-helper
`
	if err := os.WriteFile(execKC, []byte(execYAML), 0600); err != nil {
		t.Fatal(err)
	}
	noExecPerms, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"k8s.config", "fs.read(" + filepath.ToSlash(libkite.CanonicalizePath(kcDir)) + "/**)"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	th3 := &starlark.Thread{Name: "test-exec-provider"}
	libkite.SetPermissions(th3, noExecPerms)
	if _, err := newK8sClient(th3, &libkite.ModuleConfig{}, clientOptions{kubeconfig: execKC}); err == nil || !libkite.IsPermissionError(err) || !strings.Contains(err.Error(), "os.exec") {
		t.Errorf("expected os.exec PermissionError for ExecProvider, got %v", err)
	}

	// 4. Scoped --sandbox-net-allow gates k8s.config / k8s.read / k8s.write / k8s.exec per cluster URL.
	scopedChecker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   libkite.SynthesizeNetworkRules([]string{"trusted.k8s.local:6443"}),
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	th4 := &starlark.Thread{Name: "test-scoped-k8s"}
	libkite.SetPermissions(th4, scopedChecker)

	trustedClient := &K8sClient{restCfg: &rest.Config{Host: "https://trusted.k8s.local:6443"}}
	if err := libkite.Check(th4, "k8s", "read", "read", trustedClient.permResource()); err != nil {
		t.Errorf("expected trusted cluster read to succeed under --sandbox-net-allow: %v", err)
	}
	otherClient := &K8sClient{restCfg: &rest.Config{Host: "https://evil.k8s.local:6443"}}
	if err := libkite.Check(th4, "k8s", "read", "read", otherClient.permResource()); err == nil || !libkite.IsPermissionError(err) {
		t.Errorf("expected other cluster read to fail with PermissionError, got %v", err)
	}
}

func TestK8sExecCapabilitySynthesizedByNetworkAllow(t *testing.T) {
	checker, err := libkite.NewPermissionChecker(libkite.SandboxNetPermissions())
	if err != nil {
		t.Fatal(err)
	}
	if err := checker.Check("k8s", "exec", "exec", ""); err != nil {
		t.Errorf("expected SandboxNetPermissions() to allow k8s.exec, got %v", err)
	}

	hint := libkite.SuggestCapabilityHint("k8s", "exec", "exec", "")
	if !strings.Contains(hint, "k8s.exec") {
		t.Errorf("expected SuggestCapabilityHint for k8s.exec to mention k8s.exec, got %q", hint)
	}
}
