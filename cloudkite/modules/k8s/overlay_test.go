package k8s

import (
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

func evalK8sStarlark(t *testing.T, script string) starlark.StringDict {
	t.Helper()
	m := New()
	globals, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	thread := &starlark.Thread{Name: "test-k8s-overlay"}
	out, err := starlark.ExecFile(thread, "test.star", script, globals)
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	return out
}

func TestObjCloneKubeResourceAndMapMerge(t *testing.T) {
	script := `
base_cm = k8s.obj.config_map(
    name = "the-map",
    labels = {"app": "hello", "tier": "web"},
    data = {"altGreeting": "Good Morning!", "enableRisky": "false"},
)
staging_cm = k8s.obj.config_map(
    base_cm,
    labels = {"environment": "staging", "tier": None},
    data = {"altGreeting": "Have a pineapple!"},
)

base_dep = k8s.obj.deployment(
    name = "the-deployment",
    replicas = 1,
    labels = {"app": "hello"},
    containers = [{"name": "app", "image": "monopole/hello:1"}],
)
prod_dep = k8s.obj.deployment(
    base_dep,
    replicas = 3,
    labels = {"environment": "production"},
)
`
	out := evalK8sStarlark(t, script)

	baseCM := out["base_cm"].(*KubeResource).ToDict()
	stagingCM := out["staging_cm"].(*KubeResource).ToDict()

	// Base ConfigMap must not be mutated.
	baseData, _, _ := baseCM.Get(starlark.String("data"))
	baseGreeting, _, _ := baseData.(*starlark.Dict).Get(starlark.String("altGreeting"))
	if string(baseGreeting.(starlark.String)) != "Good Morning!" {
		t.Fatalf("base_cm mutated: altGreeting=%v", baseGreeting)
	}

	// Staging ConfigMap merges data and labels, and deletes tier via None.
	stDataVal, _, _ := stagingCM.Get(starlark.String("data"))
	stData := stDataVal.(*starlark.Dict)
	stGreeting, _, _ := stData.Get(starlark.String("altGreeting"))
	stRisky, _, _ := stData.Get(starlark.String("enableRisky"))
	if string(stGreeting.(starlark.String)) != "Have a pineapple!" {
		t.Fatalf("staging_cm.data.altGreeting = %v", stGreeting)
	}
	if string(stRisky.(starlark.String)) != "false" {
		t.Fatalf("staging_cm.data.enableRisky = %v, want false (preserved from base)", stRisky)
	}

	stMetaVal, _, _ := stagingCM.Get(starlark.String("metadata"))
	stLabelsVal, _, _ := stMetaVal.(*starlark.Dict).Get(starlark.String("labels"))
	stLabels := stLabelsVal.(*starlark.Dict)
	if _, found, _ := stLabels.Get(starlark.String("app")); !found {
		t.Fatal("staging_cm lost base label 'app'")
	}
	if _, found, _ := stLabels.Get(starlark.String("tier")); found {
		t.Fatal("staging_cm expected label 'tier' deleted by None")
	}
	envVal, _, _ := stLabels.Get(starlark.String("environment"))
	if string(envVal.(starlark.String)) != "staging" {
		t.Fatalf("staging_cm label environment = %v", envVal)
	}

	// Cloned Deployment preserves pod template while overriding replicas and merging labels.
	prodDep := out["prod_dep"].(*KubeResource).ToDict()
	specVal, _, _ := prodDep.Get(starlark.String("spec"))
	spec := specVal.(*starlark.Dict)
	replicas, _, _ := spec.Get(starlark.String("replicas"))
	if replicas.(starlark.Int).String() != "3" {
		t.Fatalf("prod_dep replicas = %v, want 3", replicas)
	}
	if _, found, _ := spec.Get(starlark.String("template")); !found {
		t.Fatal("prod_dep lost pod template on clone")
	}
}

func TestObjCloneKindMismatchError(t *testing.T) {
	m := New()
	globals, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	script := `
svc = k8s.obj.service(name = "svc", ports = [{"port": 80}])
bad = k8s.obj.deployment(svc)
`
	thread := &starlark.Thread{Name: "test-mismatch"}
	_, err = starlark.ExecFile(thread, "test.star", script, globals)
	if err == nil || !strings.Contains(err.Error(), "cannot clone Service into Deployment") {
		t.Fatalf("expected kind mismatch error, got: %v", err)
	}
}

func TestOverlayEndToEndTransformsAndRefRewriting(t *testing.T) {
	script := `
base_sa = k8s.obj.service_account(name = "hello-sa")
base_cm = k8s.obj.config_map(
    name = "the-map",
    data = {"altGreeting": "Good Morning!", "enableRisky": "false"},
)
base_dep = k8s.obj.deployment(
    name = "the-deployment",
    replicas = 1,
    labels = {"app": "hello"},
    service_account = base_sa,
    containers = [{
        "name": "the-container",
        "image": "monopole/hello:1",
        "env": [
            {"name": "ALT_GREETING", "valueFrom": {"configMapKeyRef": {"name": "the-map", "key": "altGreeting"}}},
        ],
    }],
)
base_svc = k8s.obj.service(
    name = "the-service",
    selector = {"app": "hello"},
    ports = [{"port": 8666, "targetPort": 8080}],
)

def harden_containers(obj):
    if obj.get("kind") != "Deployment":
        return obj
    for c in obj["spec"]["template"]["spec"]["containers"]:
        c["securityContext"] = {"runAsNonRoot": True, "readOnlyRootFilesystem": True}
    return obj

out = k8s.overlay(
    [base_sa, base_cm, base_dep, base_svc],
    namespace = "hello-prod",
    name_prefix = "prod-",
    labels = {"environment": "production"},
    annotations = {"managed-by": "starkite"},
    images = {"monopole/hello": "registry.example.internal/monopole/hello:2"},
    patches = [
        {"kind": "Deployment", "name": "the-deployment", "patch": {"spec": {"replicas": 3}}},
        {"kind": "ConfigMap", "name": "the-map", "patch": {"data": {"altGreeting": "Hello Prod!"}}},
        harden_containers,
    ],
    extra = [
        k8s.obj.namespace(name = "hello-prod"),
        k8s.obj.horizontal_pod_autoscaler(
            name = "the-hpa",
            scale_target_ref = {"apiVersion": "apps/v1", "kind": "Deployment", "name": "the-deployment"},
            min_replicas = 3,
            max_replicas = 6,
        ),
    ],
)
rendered = k8s.yaml(out)
`
	out := evalK8sStarlark(t, script)
	list := out["out"].(*starlark.List)
	if list.Len() != 6 {
		t.Fatalf("expected 6 resources, got %d", list.Len())
	}

	byKind := make(map[string]*starlark.Dict, list.Len())
	for i := 0; i < list.Len(); i++ {
		d := list.Index(i).(*starlark.Dict)
		k, _, _ := d.Get(starlark.String("kind"))
		byKind[string(k.(starlark.String))] = d
	}

	// Namespace is cluster-scoped and never prefixed or namespaced.
	nsMetaVal, _, _ := byKind["Namespace"].Get(starlark.String("metadata"))
	nsMeta := nsMetaVal.(*starlark.Dict)
	nsName, _, _ := nsMeta.Get(starlark.String("name"))
	if string(nsName.(starlark.String)) != "hello-prod" {
		t.Fatalf("Namespace name = %v, want hello-prod", nsName)
	}
	if _, hasNS, _ := nsMeta.Get(starlark.String("namespace")); hasNS {
		t.Fatal("Namespace should not receive metadata.namespace")
	}

	// Deployment renamed, namespaced, replicas patched, image rewritten, securityContext injected, refs rewritten.
	dep := byKind["Deployment"]
	depMetaVal, _, _ := dep.Get(starlark.String("metadata"))
	depMeta := depMetaVal.(*starlark.Dict)
	depName, _, _ := depMeta.Get(starlark.String("name"))
	depNS, _, _ := depMeta.Get(starlark.String("namespace"))
	if string(depName.(starlark.String)) != "prod-the-deployment" || string(depNS.(starlark.String)) != "hello-prod" {
		t.Fatalf("Deployment name/ns = %v/%v", depName, depNS)
	}

	depSpecVal, _, _ := dep.Get(starlark.String("spec"))
	depSpec := depSpecVal.(*starlark.Dict)
	replicas, _, _ := depSpec.Get(starlark.String("replicas"))
	if replicas.(starlark.Int).String() != "3" {
		t.Fatalf("Deployment replicas = %v, want 3", replicas)
	}

	tmplVal, _, _ := depSpec.Get(starlark.String("template"))
	podSpecVal, _, _ := tmplVal.(*starlark.Dict).Get(starlark.String("spec"))
	podSpec := podSpecVal.(*starlark.Dict)
	saName, _, _ := podSpec.Get(starlark.String("serviceAccountName"))
	if string(saName.(starlark.String)) != "prod-hello-sa" {
		t.Fatalf("serviceAccountName = %v, want prod-hello-sa", saName)
	}

	containersVal, _, _ := podSpec.Get(starlark.String("containers"))
	c0 := containersVal.(*starlark.List).Index(0).(*starlark.Dict)
	img, _, _ := c0.Get(starlark.String("image"))
	if string(img.(starlark.String)) != "registry.example.internal/monopole/hello:2" {
		t.Fatalf("container image = %v", img)
	}
	scVal, foundSC, _ := c0.Get(starlark.String("securityContext"))
	if !foundSC {
		t.Fatal("expected securityContext injected by patch function")
	}
	runAsNonRoot, _, _ := scVal.(*starlark.Dict).Get(starlark.String("runAsNonRoot"))
	if !bool(runAsNonRoot.(starlark.Bool)) {
		t.Fatalf("runAsNonRoot = %v, want true", runAsNonRoot)
	}

	envVal, _, _ := c0.Get(starlark.String("env"))
	env0 := envVal.(*starlark.List).Index(0).(*starlark.Dict)
	vfVal, _, _ := env0.Get(starlark.String("valueFrom"))
	cmRefVal, _, _ := vfVal.(*starlark.Dict).Get(starlark.String("configMapKeyRef"))
	cmRefName, _, _ := cmRefVal.(*starlark.Dict).Get(starlark.String("name"))
	if string(cmRefName.(starlark.String)) != "prod-the-map" {
		t.Fatalf("configMapKeyRef.name = %v, want prod-the-map", cmRefName)
	}

	// HPA in extra gets prefixed and its scaleTargetRef.name rewritten.
	hpaSpecVal, _, _ := byKind["HorizontalPodAutoscaler"].Get(starlark.String("spec"))
	targetRefVal, _, _ := hpaSpecVal.(*starlark.Dict).Get(starlark.String("scaleTargetRef"))
	targetName, _, _ := targetRefVal.(*starlark.Dict).Get(starlark.String("name"))
	if string(targetName.(starlark.String)) != "prod-the-deployment" {
		t.Fatalf("HPA scaleTargetRef.name = %v, want prod-the-deployment", targetName)
	}

	// Strict KYAML output renders cleanly with quoted boolean string.
	rendered := string(out["rendered"].(starlark.String))
	if !strings.Contains(rendered, `enableRisky: "false"`) {
		t.Fatalf("expected strict KYAML quoted boolean string in:\n%s", rendered)
	}
}

func TestOverlayDropResourceAndImageDictSpecAndTryVariant(t *testing.T) {
	script := `
raw_yaml = """
apiVersion: v1
kind: ConfigMap
metadata:
  name: drop-me
data:
  a: "1"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: web
          image: localhost:5000/team/web:1.0
"""

def drop_cm(obj):
    if obj.get("kind") == "ConfigMap":
        return None
    return obj

out = k8s.overlay(
    raw_yaml,
    images = {"localhost:5000/team/web": {"new_tag": "2.0"}},
    patches = [drop_cm],
)
bad = k8s.try_overlay("not-a-map")
`
	out := evalK8sStarlark(t, script)
	list := out["out"].(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 resource after drop_cm, got %d", list.Len())
	}
	dep := list.Index(0).(*starlark.Dict)
	specVal, _, _ := dep.Get(starlark.String("spec"))
	tmplVal, _, _ := specVal.(*starlark.Dict).Get(starlark.String("template"))
	podSpecVal, _, _ := tmplVal.(*starlark.Dict).Get(starlark.String("spec"))
	c0 := podSpecVal.(*starlark.Dict)
	cList, _, _ := c0.Get(starlark.String("containers"))
	img, _, _ := cList.(*starlark.List).Index(0).(*starlark.Dict).Get(starlark.String("image"))
	if string(img.(starlark.String)) != "localhost:5000/team/web:2.0" {
		t.Fatalf("image = %v, want localhost:5000/team/web:2.0", img)
	}

	bad := out["bad"].(starlark.HasAttrs)
	okVal, err := bad.Attr("ok")
	if err != nil {
		t.Fatal(err)
	}
	if bool(okVal.(starlark.Bool)) {
		t.Fatal("expected k8s.try_overlay on invalid YAML to return ok=False")
	}
}
