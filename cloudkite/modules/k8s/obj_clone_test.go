package k8s

import (
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func evalK8sStarlark(t *testing.T, script string) starlark.StringDict {
	t.Helper()
	m := New()
	globals, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	globals["struct"] = starlark.NewBuiltin("struct", starlarkstruct.Make)
	thread := &starlark.Thread{Name: "test-k8s-obj-clone"}
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

	baseData, _, _ := baseCM.Get(starlark.String("data"))
	baseGreeting, _, _ := baseData.(*starlark.Dict).Get(starlark.String("altGreeting"))
	if string(baseGreeting.(starlark.String)) != "Good Morning!" {
		t.Fatalf("base_cm mutated: altGreeting=%v", baseGreeting)
	}

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

func TestK8sYAMLSerializesStructs(t *testing.T) {
	script := `
manifest = struct(
    apiVersion = "v1",
    kind = "ConfigMap",
    metadata = struct(name = "the-map", labels = struct(app = "hello")),
    data = struct(enableRisky = "true"),
)
out = k8s.yaml(manifest)
`
	out := evalK8sStarlark(t, script)
	yamlStr := string(out["out"].(starlark.String))
	for _, want := range []string{`kind: ConfigMap`, `name: the-map`, `app: hello`, `enableRisky: "true"`} {
		if !strings.Contains(yamlStr, want) {
			t.Fatalf("k8s.yaml(struct) missing %q:\n%s", want, yamlStr)
		}
	}
}
