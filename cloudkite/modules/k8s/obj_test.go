package k8s

import (
	"slices"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

func TestNewKubeResource_BasicDeployment(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("replicas"), starlark.MakeInt(3)},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	if kr.Kind() != "Deployment" {
		t.Errorf("Kind() = %q, want %q", kr.Kind(), "Deployment")
	}

	// Check name attribute
	nameVal, err := kr.Attr("name")
	if err != nil {
		t.Fatalf("Attr(name) error: %v", err)
	}
	if s, ok := nameVal.(starlark.String); !ok || string(s) != "web" {
		t.Errorf("name = %v, want %q", nameVal, "web")
	}

	// Check replicas attribute
	repVal, err := kr.Attr("replicas")
	if err != nil {
		t.Fatalf("Attr(replicas) error: %v", err)
	}
	if repVal.String() != "3" {
		t.Errorf("replicas = %v, want 3", repVal)
	}
}

func TestNewKubeResource_RequiredField(t *testing.T) {
	// Container requires name and image
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
	}

	_, err := newKubeResource(containerSchema, nil, kwargs)
	if err == nil {
		t.Error("expected error for missing required field 'image'")
	}
}

func TestNewKubeResource_UnknownField(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("test")},
		{starlark.String("nonexistent"), starlark.String("value")},
	}

	_, err := newKubeResource(namespaceSchema, nil, kwargs)
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestNewKubeResource_DefaultValues(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	// replicas should default to 1
	repVal, _ := kr.Attr("replicas")
	if repVal.String() != "1" {
		t.Errorf("replicas = %v, want 1 (default)", repVal)
	}
}

func TestNewKubeResource_Container(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("nginx:1.27")},
	}

	kr, err := newKubeResource(containerSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	if kr.Kind() != "Container" {
		t.Errorf("Kind() = %q, want %q", kr.Kind(), "Container")
	}
}

func TestKubeResource_ToDict_TopLevel(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("replicas"), starlark.MakeInt(3)},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	dict := kr.ToDict()

	// Check apiVersion
	av, _, _ := dict.Get(starlark.String("apiVersion"))
	if s, ok := av.(starlark.String); !ok || string(s) != "apps/v1" {
		t.Errorf("apiVersion = %v, want %q", av, "apps/v1")
	}

	// Check kind
	kind, _, _ := dict.Get(starlark.String("kind"))
	if s, ok := kind.(starlark.String); !ok || string(s) != "Deployment" {
		t.Errorf("kind = %v, want %q", kind, "Deployment")
	}

	// Check metadata.name
	metadata, _, _ := dict.Get(starlark.String("metadata"))
	if md, ok := metadata.(*starlark.Dict); ok {
		name, _, _ := md.Get(starlark.String("name"))
		if s, ok := name.(starlark.String); !ok || string(s) != "web" {
			t.Errorf("metadata.name = %v, want %q", name, "web")
		}
	} else {
		t.Error("metadata is not a dict")
	}

	// Check spec.replicas
	spec, _, _ := dict.Get(starlark.String("spec"))
	if sp, ok := spec.(*starlark.Dict); ok {
		rep, _, _ := sp.Get(starlark.String("replicas"))
		if rep == nil {
			t.Error("spec.replicas is nil")
		}
	} else {
		t.Error("spec is not a dict")
	}
}

func TestKubeResource_ToDict_SubObject(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("nginx:1.27")},
	}

	kr, err := newKubeResource(containerSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	dict := kr.ToDict()

	// Sub-objects should NOT have apiVersion/kind/metadata
	_, found, _ := dict.Get(starlark.String("apiVersion"))
	if found {
		t.Error("sub-object should not have apiVersion")
	}

	// Should have name and image at top level
	name, _, _ := dict.Get(starlark.String("name"))
	if s, ok := name.(starlark.String); !ok || string(s) != "app" {
		t.Errorf("name = %v, want %q", name, "app")
	}

	image, _, _ := dict.Get(starlark.String("image"))
	if s, ok := image.(starlark.String); !ok || string(s) != "nginx:1.27" {
		t.Errorf("image = %v, want %q", image, "nginx:1.27")
	}
}

func TestKubeResource_String(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("replicas"), starlark.MakeInt(3)},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("newKubeResource error: %v", err)
	}

	s := kr.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestKubeResource_Type(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}
	kr, _ := newKubeResource(deploymentSchema, nil, kwargs)

	if kr.Type() != "k8s.obj.deployment" {
		t.Errorf("Type() = %q, want %q", kr.Type(), "k8s.obj.deployment")
	}
}

func TestKubeResource_AttrNames(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}
	kr, _ := newKubeResource(deploymentSchema, nil, kwargs)

	names := kr.AttrNames()
	if len(names) == 0 {
		t.Error("AttrNames() returned empty")
	}

	// Should include to_dict
	found := slices.Contains(names, "to_dict")
	if !found {
		t.Error("AttrNames() missing 'to_dict'")
	}
}

func TestKubeResource_ToDictMethod(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}
	kr, _ := newKubeResource(deploymentSchema, nil, kwargs)

	todictVal, err := kr.Attr("to_dict")
	if err != nil {
		t.Fatalf("Attr(to_dict) error: %v", err)
	}
	if todictVal == nil {
		t.Fatal("to_dict is nil")
	}
}

func TestKubeResource_UnknownAttr(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}
	kr, _ := newKubeResource(deploymentSchema, nil, kwargs)

	val, err := kr.Attr("nonexistent")
	if err != nil {
		t.Fatalf("Attr(nonexistent) should not error, got: %v", err)
	}
	if val != nil {
		t.Errorf("Attr(nonexistent) = %v, want nil", val)
	}
}

func TestKubeResource_KubeObjectInterface(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
	}
	kr, _ := newKubeResource(deploymentSchema, nil, kwargs)

	// Verify it implements KubeObject
	var ko KubeObject = kr
	if ko.Kind() != "Deployment" {
		t.Errorf("Kind() = %q, want %q", ko.Kind(), "Deployment")
	}
	dict := ko.ToDict()
	if dict == nil {
		t.Error("ToDict() returned nil")
	}
}

func TestObjConstructors(t *testing.T) {
	constructors := ObjConstructors()
	if len(constructors) == 0 {
		t.Fatal("ObjConstructors() returned empty")
	}

	expected := []string{"pod", "deployment", "service", "container", "config_map", "secret", "persistent_volume", "storage_class", "persistent_volume_claim", "volume", "volume_mount"}
	for _, name := range expected {
		if _, ok := constructors[name]; !ok {
			t.Errorf("missing constructor %q", name)
		}
	}
}

func TestNewKubeResource_FromYAML(t *testing.T) {
	yaml := starlark.String(`name: test
image: nginx:1.27`)

	kr, err := newKubeResource(containerSchema, starlark.Tuple{yaml}, nil)
	if err != nil {
		t.Fatalf("newKubeResource from YAML error: %v", err)
	}

	name, _ := kr.Attr("name")
	if s, ok := name.(starlark.String); !ok || string(s) != "test" {
		t.Errorf("name = %v, want %q", name, "test")
	}
}

func TestNewKubeResource_KwargsOverrideYAML(t *testing.T) {
	yaml := starlark.String(`name: original
image: nginx:1.27`)

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("overridden")},
	}

	kr, err := newKubeResource(containerSchema, starlark.Tuple{yaml}, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	name, _ := kr.Attr("name")
	if s, ok := name.(starlark.String); !ok || string(s) != "overridden" {
		t.Errorf("name = %v, want %q", name, "overridden")
	}
}

func TestNewKubeResource_TooManyArgs(t *testing.T) {
	_, err := newKubeResource(containerSchema, starlark.Tuple{starlark.String("a"), starlark.String("b")}, nil)
	if err == nil {
		t.Error("expected error for too many positional args")
	}
}

func TestNewKubeResource_ConfigMap(t *testing.T) {
	data := starlark.NewDict(1)
	data.SetKey(starlark.String("key"), starlark.String("value"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("my-cm")},
		{starlark.String("data"), data},
	}

	kr, err := newKubeResource(configMapSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict := kr.ToDict()
	dataVal, _, _ := dict.Get(starlark.String("data"))
	if dataVal == nil {
		t.Error("data is nil in output dict")
	}
}

// --- P0: autoTemplate / flattened workload tests ---

func TestAutoTemplate_FlattenedDeployment(t *testing.T) {
	// Build a container as a KubeResource
	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("nginx:1.27")},
	}
	container, err := newKubeResource(containerSchema, nil, containerKwargs)
	if err != nil {
		t.Fatalf("container error: %v", err)
	}

	// Build containers list
	containers := starlark.NewList([]starlark.Value{container})

	// Build labels dict
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("web"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("labels"), labels},
		{starlark.String("replicas"), starlark.MakeInt(3)},
		{starlark.String("containers"), containers},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("deployment error: %v", err)
	}

	dict := kr.ToDict()

	// Check spec.template exists
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	tmpl, found, _ := sp.Get(starlark.String("template"))
	if !found {
		t.Fatal("spec.template not found")
	}
	tmplDict := tmpl.(*starlark.Dict)

	// Check template.metadata.labels
	tmplMeta, _, _ := tmplDict.Get(starlark.String("metadata"))
	if md, ok := tmplMeta.(*starlark.Dict); ok {
		tmplLabels, _, _ := md.Get(starlark.String("labels"))
		if tmplLabels == nil {
			t.Error("template.metadata.labels is nil")
		}
	} else {
		t.Error("template.metadata is not a dict")
	}

	// Check template.spec.containers
	tmplSpec, _, _ := tmplDict.Get(starlark.String("spec"))
	if ts, ok := tmplSpec.(*starlark.Dict); ok {
		c, _, _ := ts.Get(starlark.String("containers"))
		if c == nil {
			t.Error("template.spec.containers is nil")
		}
	} else {
		t.Error("template.spec is not a dict")
	}

	// Check spec.selector auto-derived
	sel, found, _ := sp.Get(starlark.String("selector"))
	if !found {
		t.Fatal("spec.selector not found (should be auto-derived)")
	}
	selDict := sel.(*starlark.Dict)
	ml, _, _ := selDict.Get(starlark.String("matchLabels"))
	if ml == nil {
		t.Error("selector.matchLabels is nil")
	}
}

func TestAutoTemplate_ContainersAndTemplateMutualExclusion(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("dummy")})
	tmpl := starlark.NewDict(1)
	tmpl.SetKey(starlark.String("spec"), starlark.NewDict(0))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("test")},
		{starlark.String("containers"), containers},
		{starlark.String("template"), tmpl},
	}

	_, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err == nil {
		t.Error("expected error when both containers and template are provided")
	}
}

func TestAutoTemplate_ExplicitTemplateStillWorks(t *testing.T) {
	// Explicit template= should work as before (backward compat)
	tmpl := starlark.NewDict(2)
	tmplMeta := starlark.NewDict(1)
	tmplLabels := starlark.NewDict(1)
	tmplLabels.SetKey(starlark.String("app"), starlark.String("test"))
	tmplMeta.SetKey(starlark.String("labels"), tmplLabels)
	tmplSpec := starlark.NewDict(1)
	containerList := starlark.NewList([]starlark.Value{starlark.NewDict(0)})
	tmplSpec.SetKey(starlark.String("containers"), containerList)
	tmpl.SetKey(starlark.String("metadata"), tmplMeta)
	tmpl.SetKey(starlark.String("spec"), tmplSpec)

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("test")},
		{starlark.String("template"), tmpl},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("deployment error: %v", err)
	}

	if kr.Kind() != "Deployment" {
		t.Errorf("Kind() = %q, want Deployment", kr.Kind())
	}
}

func TestAutoTemplate_SelectorAutoDerive(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(2)
	labels.SetKey(starlark.String("app"), starlark.String("web"))
	labels.SetKey(starlark.String("env"), starlark.String("prod"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	sel, found, _ := sp.Get(starlark.String("selector"))
	if !found {
		t.Fatal("selector not auto-derived")
	}
	selDict := sel.(*starlark.Dict)
	ml, _, _ := selDict.Get(starlark.String("matchLabels"))
	mlDict := ml.(*starlark.Dict)

	appVal, _, _ := mlDict.Get(starlark.String("app"))
	if s, ok := appVal.(starlark.String); !ok || string(s) != "web" {
		t.Errorf("matchLabels.app = %v, want %q", appVal, "web")
	}
}

func TestAutoTemplate_ExplicitSelectorNotOverridden(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("web"))

	customSel := starlark.NewDict(1)
	customML := starlark.NewDict(1)
	customML.SetKey(starlark.String("custom"), starlark.String("selector"))
	customSel.SetKey(starlark.String("matchLabels"), customML)

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
		{starlark.String("selector"), customSel},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	sel, _, _ := sp.Get(starlark.String("selector"))
	selDict := sel.(*starlark.Dict)
	ml, _, _ := selDict.Get(starlark.String("matchLabels"))
	mlDict := ml.(*starlark.Dict)

	customVal, _, _ := mlDict.Get(starlark.String("custom"))
	if s, ok := customVal.(starlark.String); !ok || string(s) != "selector" {
		t.Errorf("matchLabels.custom = %v, want %q", customVal, "selector")
	}
}

func TestAutoTemplate_TemplateLabelOverride(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("web"))

	tmplLabels := starlark.NewDict(1)
	tmplLabels.SetKey(starlark.String("pod-label"), starlark.String("special"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
		{starlark.String("template_labels"), tmplLabels},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	tmpl, _, _ := sp.Get(starlark.String("template"))
	tmplDict := tmpl.(*starlark.Dict)
	tmplMeta, _, _ := tmplDict.Get(starlark.String("metadata"))
	md := tmplMeta.(*starlark.Dict)
	tl, _, _ := md.Get(starlark.String("labels"))
	tlDict := tl.(*starlark.Dict)

	podLabel, _, _ := tlDict.Get(starlark.String("pod-label"))
	if s, ok := podLabel.(starlark.String); !ok || string(s) != "special" {
		t.Errorf("template labels should use template_labels override, got %v", podLabel)
	}

	// Verify original labels are NOT in template metadata
	appLabel, found, _ := tlDict.Get(starlark.String("app"))
	if found && appLabel != nil {
		t.Error("template labels should NOT contain resource labels when template_labels is provided")
	}
}

func TestAutoTemplate_WithVolumes(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})

	volDict := starlark.NewDict(2)
	volDict.SetKey(starlark.String("name"), starlark.String("data"))
	volDict.SetKey(starlark.String("emptyDir"), starlark.NewDict(0))
	volumes := starlark.NewList([]starlark.Value{volDict})

	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("web"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("web")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
		{starlark.String("volumes"), volumes},
	}

	kr, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	tmpl, _, _ := sp.Get(starlark.String("template"))
	tmplDict := tmpl.(*starlark.Dict)
	tmplSpec, _, _ := tmplDict.Get(starlark.String("spec"))
	ts := tmplSpec.(*starlark.Dict)

	vols, found, _ := ts.Get(starlark.String("volumes"))
	if !found || vols == nil {
		t.Error("template.spec.volumes is missing")
	}
}

func TestAutoTemplate_CronJob(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("cleanup"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("cleanup")},
		{starlark.String("schedule"), starlark.String("0 */6 * * *")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
		{starlark.String("restart_policy"), starlark.String("Never")},
	}

	kr, err := newKubeResource(cronJobSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("cron_job error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	// CronJob should have jobTemplate, not template
	jt, found, _ := sp.Get(starlark.String("jobTemplate"))
	if !found {
		t.Fatal("spec.jobTemplate not found")
	}
	jtDict := jt.(*starlark.Dict)

	jtSpec, _, _ := jtDict.Get(starlark.String("spec"))
	jtsDict := jtSpec.(*starlark.Dict)

	tmpl, found, _ := jtsDict.Get(starlark.String("template"))
	if !found {
		t.Fatal("jobTemplate.spec.template not found")
	}
	tmplDict := tmpl.(*starlark.Dict)

	tmplSpec, _, _ := tmplDict.Get(starlark.String("spec"))
	ts := tmplSpec.(*starlark.Dict)

	rp, _, _ := ts.Get(starlark.String("restartPolicy"))
	if s, ok := rp.(starlark.String); !ok || string(s) != "Never" {
		t.Errorf("template.spec.restartPolicy = %v, want Never", rp)
	}
}

func TestAutoTemplate_StatefulSet(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("redis"))

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("redis")},
		{starlark.String("labels"), labels},
		{starlark.String("containers"), containers},
		{starlark.String("service_name"), starlark.String("redis-headless")},
	}

	kr, err := newKubeResource(statefulSetSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("stateful_set error: %v", err)
	}

	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	tmpl, found, _ := sp.Get(starlark.String("template"))
	if !found {
		t.Fatal("spec.template not found")
	}
	tmplDict := tmpl.(*starlark.Dict)
	tmplMeta, _, _ := tmplDict.Get(starlark.String("metadata"))
	if tmplMeta == nil {
		t.Error("template.metadata is nil")
	}

	svcName, _, _ := sp.Get(starlark.String("serviceName"))
	if s, ok := svcName.(starlark.String); !ok || string(s) != "redis-headless" {
		t.Errorf("serviceName = %v, want redis-headless", svcName)
	}
}

func TestAutoTemplate_NonWorkloadIgnored(t *testing.T) {
	// Pod is not a workload — containers= should be treated normally
	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("nginx:1.27")},
	}
	container, _ := newKubeResource(containerSchema, nil, containerKwargs)
	containers := starlark.NewList([]starlark.Value{container})

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("test")},
		{starlark.String("containers"), containers},
	}

	kr, err := newKubeResource(podSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("pod error: %v", err)
	}

	// Pod should have containers in spec directly, not in template
	dict := kr.ToDict()
	spec, _, _ := dict.Get(starlark.String("spec"))
	sp := spec.(*starlark.Dict)

	c, found, _ := sp.Get(starlark.String("containers"))
	if !found || c == nil {
		t.Error("pod spec.containers missing")
	}

	_, tmplFound, _ := sp.Get(starlark.String("template"))
	if tmplFound {
		t.Error("pod should NOT have template")
	}
}

func TestPodTemplate_SubObject(t *testing.T) {
	containers := starlark.NewList([]starlark.Value{starlark.String("c")})
	labels := starlark.NewDict(1)
	labels.SetKey(starlark.String("app"), starlark.String("web"))

	kwargs := []starlark.Tuple{
		{starlark.String("containers"), containers},
		{starlark.String("labels"), labels},
	}

	kr, err := newKubeResource(podTemplateSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("pod_template error: %v", err)
	}

	dict := kr.ToDict()

	// Should have metadata.labels
	meta, found, _ := dict.Get(starlark.String("metadata"))
	if !found {
		t.Fatal("metadata not found")
	}
	md := meta.(*starlark.Dict)
	lbl, _, _ := md.Get(starlark.String("labels"))
	if lbl == nil {
		t.Error("metadata.labels is nil")
	}

	// Should have spec.containers
	spec, found, _ := dict.Get(starlark.String("spec"))
	if !found {
		t.Fatal("spec not found")
	}
	sp := spec.(*starlark.Dict)
	c, _, _ := sp.Get(starlark.String("containers"))
	if c == nil {
		t.Error("spec.containers is nil")
	}
}

func TestDRA_DeviceClass(t *testing.T) {
	selectors := starlark.NewList([]starlark.Value{
		starlark.NewDict(1),
	})
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("gpu.nvidia.com")},
		{starlark.String("selectors"), selectors},
	}

	kr, err := newKubeResource(deviceClassSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("device_class error: %v", err)
	}

	if kr.Kind() != "DeviceClass" {
		t.Errorf("Kind() = %q, want DeviceClass", kr.Kind())
	}

	dict := kr.ToDict()
	av, _, _ := dict.Get(starlark.String("apiVersion"))
	if s, ok := av.(starlark.String); !ok || string(s) != "resource.k8s.io/v1" {
		t.Errorf("apiVersion = %v, want resource.k8s.io/v1", av)
	}

	spec, found, _ := dict.Get(starlark.String("spec"))
	if !found {
		t.Fatal("spec not found")
	}
	sp := spec.(*starlark.Dict)
	sel, found, _ := sp.Get(starlark.String("selectors"))
	if !found || sel == nil {
		t.Error("selectors not found in spec")
	}
}

func TestDRA_ResourceClaim_Shortcuts(t *testing.T) {
	toleration := starlark.NewDict(2)
	toleration.SetKey(starlark.String("key"), starlark.String("gpu.nvidia.com/mig"))
	toleration.SetKey(starlark.String("operator"), starlark.String("Exists"))
	tolerations := starlark.NewList([]starlark.Value{toleration})

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("ml-gpu-claim")},
		{starlark.String("device_class"), starlark.String("gpu.nvidia.com")},
		{starlark.String("count"), starlark.MakeInt(2)},
		{starlark.String("device_tolerations"), tolerations},
	}

	kr, err := newKubeResource(resourceClaimSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("resource_claim error: %v", err)
	}

	dict := kr.ToDict()
	specVal, found, _ := dict.Get(starlark.String("spec"))
	if !found {
		t.Fatal("spec not found")
	}
	spec := specVal.(*starlark.Dict)

	devicesVal, found, _ := spec.Get(starlark.String("devices"))
	if !found {
		t.Fatal("spec.devices not found")
	}
	devices := devicesVal.(*starlark.Dict)

	reqsVal, found, _ := devices.Get(starlark.String("requests"))
	if !found {
		t.Fatal("devices.requests not found")
	}
	reqs := reqsVal.(*starlark.List)
	if reqs.Len() != 1 {
		t.Fatalf("requests len = %d, want 1", reqs.Len())
	}

	firstReq := reqs.Index(0).(*starlark.Dict)
	dcName, _, _ := firstReq.Get(starlark.String("deviceClassName"))
	if s, ok := dcName.(starlark.String); !ok || string(s) != "gpu.nvidia.com" {
		t.Errorf("deviceClassName = %v, want gpu.nvidia.com", dcName)
	}

	countVal, _, _ := firstReq.Get(starlark.String("count"))
	if countVal == nil || countVal.String() != "2" {
		t.Errorf("count = %v, want 2", countVal)
	}

	reqName, _, _ := firstReq.Get(starlark.String("name"))
	if s, ok := reqName.(starlark.String); !ok || string(s) != "req-1" {
		t.Errorf("request name = %v, want req-1", reqName)
	}

	// Ensure shortcut fields are NOT leaked at root
	if _, leaked, _ := dict.Get(starlark.String("device_class")); leaked {
		t.Error("device_class leaked at root")
	}
	if _, leaked, _ := dict.Get(starlark.String("deviceClassName")); leaked {
		t.Error("deviceClassName leaked at root")
	}
	if _, leaked, _ := dict.Get(starlark.String("count")); leaked {
		t.Error("count leaked at root")
	}
}

func TestDRA_ResourceClaimTemplate_UnwrapClaim(t *testing.T) {
	// Create a claim
	claimKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("my-claim")},
		{starlark.String("device_class"), starlark.String("gpu.nvidia.com")},
		{starlark.String("count"), starlark.MakeInt(1)},
	}
	claim, err := newKubeResource(resourceClaimSchema, nil, claimKwargs)
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}

	// Create claim template with spec = claim
	tmplKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("worker-gpu-template")},
		{starlark.String("spec"), claim},
	}
	tmpl, err := newKubeResource(resourceClaimTemplateSchema, nil, tmplKwargs)
	if err != nil {
		t.Fatalf("resource_claim_template error: %v", err)
	}

	dict := tmpl.ToDict()
	specVal, found, _ := dict.Get(starlark.String("spec"))
	if !found {
		t.Fatal("template spec not found")
	}
	spec := specVal.(*starlark.Dict)

	// In ResourceClaimTemplate, spec.spec must be the ResourceClaimSpec
	innerSpecVal, found, _ := spec.Get(starlark.String("spec"))
	if !found {
		t.Fatal("spec.spec not found in ResourceClaimTemplate")
	}
	innerSpec := innerSpecVal.(*starlark.Dict)

	devicesVal, found, _ := innerSpec.Get(starlark.String("devices"))
	if !found {
		t.Fatal("devices not found in spec.spec")
	}
	devices := devicesVal.(*starlark.Dict)
	_, foundReqs, _ := devices.Get(starlark.String("requests"))
	if !foundReqs {
		t.Error("requests not found in inner spec.devices")
	}

	// Inner spec should NOT have apiVersion or kind
	if _, hasKind, _ := innerSpec.Get(starlark.String("kind")); hasKind {
		t.Error("inner spec.spec should not have 'kind'")
	}
}

func TestDRA_ContainerClaims(t *testing.T) {
	claimRef := starlark.NewDict(1)
	claimRef.SetKey(starlark.String("name"), starlark.String("gpu"))
	claimsList := starlark.NewList([]starlark.Value{claimRef})

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("engine")},
		{starlark.String("image"), starlark.String("vllm/vllm:latest")},
		{starlark.String("claims"), claimsList},
	}

	c, err := newKubeResource(containerSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("container error: %v", err)
	}

	dict := c.ToDict()

	// Container should NOT have top-level claims
	if _, hasClaims, _ := dict.Get(starlark.String("claims")); hasClaims {
		t.Error("container should not have top-level 'claims'")
	}

	// Container should have resources.claims
	resVal, found, _ := dict.Get(starlark.String("resources"))
	if !found {
		t.Fatal("resources not found on container")
	}
	res := resVal.(*starlark.Dict)

	resClaimsVal, found, _ := res.Get(starlark.String("claims"))
	if !found {
		t.Fatal("claims not found in container.resources")
	}
	resClaims := resClaimsVal.(*starlark.List)
	if resClaims.Len() != 1 {
		t.Fatalf("resClaims len = %d, want 1", resClaims.Len())
	}
}

func TestDRA_WorkloadFlatteningDeployment(t *testing.T) {
	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("engine")},
		{starlark.String("image"), starlark.String("vllm:latest")},
	}
	c, _ := newKubeResource(containerSchema, nil, containerKwargs)
	containers := starlark.NewList([]starlark.Value{c})

	claimRef := starlark.NewDict(2)
	claimRef.SetKey(starlark.String("name"), starlark.String("gpu"))
	claimRef.SetKey(starlark.String("claim_name"), starlark.String("ml-gpu-claim"))
	tmplClaimRef := starlark.NewDict(2)
	tmplClaimRef.SetKey(starlark.String("name"), starlark.String("worker-gpu"))
	tmplClaimRef.SetKey(starlark.String("template_name"), starlark.String("worker-gpu-tmpl"))
	resourceClaims := starlark.NewList([]starlark.Value{claimRef, tmplClaimRef})

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("llm-service")},
		{starlark.String("containers"), containers},
		{starlark.String("resource_claims"), resourceClaims},
	}

	deploy, err := newKubeResource(deploymentSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("deployment error: %v", err)
	}

	dict := deploy.ToDict()
	specVal, _, _ := dict.Get(starlark.String("spec"))
	spec := specVal.(*starlark.Dict)

	tmplVal, _, _ := spec.Get(starlark.String("template"))
	tmpl := tmplVal.(*starlark.Dict)

	podSpecVal, _, _ := tmpl.Get(starlark.String("spec"))
	podSpec := podSpecVal.(*starlark.Dict)

	rcVal, found, _ := podSpec.Get(starlark.String("resourceClaims"))
	if !found {
		t.Fatal("resourceClaims not found in pod spec")
	}
	rcList := rcVal.(*starlark.List)
	if rcList.Len() != 2 {
		t.Fatalf("resourceClaims len = %d, want 2", rcList.Len())
	}

	first := rcList.Index(0).(*starlark.Dict)
	claimNameVal, found, _ := first.Get(starlark.String("resourceClaimName"))
	if !found {
		t.Errorf("expected resourceClaimName key, got keys: %v", first.Keys())
	}
	if s, ok := claimNameVal.(starlark.String); !ok || string(s) != "ml-gpu-claim" {
		t.Errorf("resourceClaimName = %v, want ml-gpu-claim", claimNameVal)
	}

	second := rcList.Index(1).(*starlark.Dict)
	tmplNameVal, found, _ := second.Get(starlark.String("resourceClaimTemplateName"))
	if !found {
		t.Errorf("expected resourceClaimTemplateName key, got keys: %v", second.Keys())
	}
	if s, ok := tmplNameVal.(starlark.String); !ok || string(s) != "worker-gpu-tmpl" {
		t.Errorf("resourceClaimTemplateName = %v, want worker-gpu-tmpl", tmplNameVal)
	}
}

func TestDRA_PodResourceClaims(t *testing.T) {
	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("c")},
		{starlark.String("image"), starlark.String("app:1.0")},
	}
	c, _ := newKubeResource(containerSchema, nil, containerKwargs)
	containers := starlark.NewList([]starlark.Value{c})

	claimRef := starlark.NewDict(2)
	claimRef.SetKey(starlark.String("name"), starlark.String("gpu"))
	claimRef.SetKey(starlark.String("claim_name"), starlark.String("my-claim"))
	resourceClaims := starlark.NewList([]starlark.Value{claimRef})

	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("test-pod")},
		{starlark.String("containers"), containers},
		{starlark.String("resource_claims"), resourceClaims},
	}

	pod, err := newKubeResource(podSchema, nil, kwargs)
	if err != nil {
		t.Fatalf("pod error: %v", err)
	}

	dict := pod.ToDict()
	specVal, _, _ := dict.Get(starlark.String("spec"))
	spec := specVal.(*starlark.Dict)

	rcVal, found, _ := spec.Get(starlark.String("resourceClaims"))
	if !found {
		t.Fatal("resourceClaims not found in pod spec")
	}
	rcList := rcVal.(*starlark.List)
	if rcList.Len() != 1 {
		t.Fatalf("resourceClaims len = %d, want 1", rcList.Len())
	}
	first := rcList.Index(0).(*starlark.Dict)
	claimName, found, _ := first.Get(starlark.String("resourceClaimName"))
	if !found || claimName.String() != `"my-claim"` {
		t.Errorf("resourceClaimName = %v, want my-claim", claimName)
	}
}

func TestDRA_EndToEndYAML(t *testing.T) {
	m := New()
	loaded, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := loaded["k8s"].(starlark.HasAttrs)
	yamlFnVal, err := k8sMod.Attr("yaml")
	if err != nil || yamlFnVal == nil {
		t.Fatalf("k8s.yaml not found: %v", err)
	}
	yamlFn := yamlFnVal.(*starlark.Builtin)

	thread := &starlark.Thread{Name: "test"}

	// 1. ResourceClaim
	claimKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("llm-accelerator")},
		{starlark.String("device_class"), starlark.String("gpu.nvidia.com")},
		{starlark.String("count"), starlark.MakeInt(1)},
	}
	claim, err := newKubeResource(resourceClaimSchema, nil, claimKwargs)
	if err != nil {
		t.Fatalf("resource_claim error: %v", err)
	}

	claimYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{claim}, nil)
	if err != nil {
		t.Fatalf("yaml(claim) error: %v", err)
	}
	claimYAML := string(claimYAMLVal.(starlark.String))
	if !strings.Contains(claimYAML, "apiVersion: resource.k8s.io/v1") {
		t.Errorf("claim YAML missing apiVersion: %s", claimYAML)
	}
	if !strings.Contains(claimYAML, "deviceClassName: gpu.nvidia.com") {
		t.Errorf("claim YAML missing deviceClassName: %s", claimYAML)
	}

	// 2. Deployment with claim attachment
	claimRefContainer := starlark.NewDict(1)
	claimRefContainer.SetKey(starlark.String("name"), starlark.String("gpu"))
	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("vllm")},
		{starlark.String("image"), starlark.String("vllm/vllm-openai:latest")},
		{starlark.String("claims"), starlark.NewList([]starlark.Value{claimRefContainer})},
	}

	container, err := newKubeResource(containerSchema, nil, containerKwargs)
	if err != nil {
		t.Fatalf("container error: %v", err)
	}

	claimRef := starlark.NewDict(2)
	claimRef.SetKey(starlark.String("name"), starlark.String("gpu"))
	claimRef.SetKey(starlark.String("claim_name"), starlark.String("llm-accelerator"))

	deployKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("llm-inference")},
		{starlark.String("replicas"), starlark.MakeInt(2)},
		{starlark.String("resource_claims"), starlark.NewList([]starlark.Value{claimRef})},
		{starlark.String("containers"), starlark.NewList([]starlark.Value{container})},
	}

	deploy, err := newKubeResource(deploymentSchema, nil, deployKwargs)
	if err != nil {
		t.Fatalf("deployment error: %v", err)
	}

	deployYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{deploy}, nil)
	if err != nil {
		t.Fatalf("yaml(deploy) error: %v", err)
	}
	deployYAML := string(deployYAMLVal.(starlark.String))

	if !strings.Contains(deployYAML, "resourceClaims:") {
		t.Errorf("deploy YAML missing resourceClaims: %s", deployYAML)
	}
	if !strings.Contains(deployYAML, "resourceClaimName: llm-accelerator") {
		t.Errorf("deploy YAML missing resourceClaimName: %s", deployYAML)
	}
	if !strings.Contains(deployYAML, "claims:") {
		t.Errorf("deploy YAML missing container claims: %s", deployYAML)
	}
}

func TestVolume_Constructors(t *testing.T) {
	m := New()
	loaded, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := loaded["k8s"].(starlark.HasAttrs)
	yamlFnVal, err := k8sMod.Attr("yaml")
	if err != nil || yamlFnVal == nil {
		t.Fatalf("k8s.yaml not found: %v", err)
	}
	yamlFn := yamlFnVal.(*starlark.Builtin)
	thread := &starlark.Thread{Name: "test"}

	// 1. PersistentVolume
	pvKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("pv-nfs")},
		{starlark.String("storage"), starlark.String("100Gi")},
		{starlark.String("access_modes"), starlark.NewList([]starlark.Value{starlark.String("ReadWriteMany")})},
		{starlark.String("storage_class_name"), starlark.String("nfs-storage")},
		{starlark.String("reclaim_policy"), starlark.String("Retain")},
	}
	pv, err := newKubeResource(persistentVolumeSchema, nil, pvKwargs)
	if err != nil {
		t.Fatalf("persistent_volume error: %v", err)
	}
	pvYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{pv}, nil)
	if err != nil {
		t.Fatalf("yaml(pv) error: %v", err)
	}
	pvYAML := string(pvYAMLVal.(starlark.String))
	if !strings.Contains(pvYAML, "kind: PersistentVolume") {
		t.Errorf("pv YAML missing kind: %s", pvYAML)
	}
	if !strings.Contains(pvYAML, "storage: 100Gi") {
		t.Errorf("pv YAML missing capacity.storage: %s", pvYAML)
	}
	if !strings.Contains(pvYAML, "storageClassName: nfs-storage") {
		t.Errorf("pv YAML missing storageClassName: %s", pvYAML)
	}
	if !strings.Contains(pvYAML, "persistentVolumeReclaimPolicy: Retain") {
		t.Errorf("pv YAML missing persistentVolumeReclaimPolicy: %s", pvYAML)
	}

	// 2. StorageClass
	scKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("fast-ssd")},
		{starlark.String("provisioner"), starlark.String("kubernetes.io/no-provisioner")},
		{starlark.String("volume_binding_mode"), starlark.String("WaitForFirstConsumer")},
		{starlark.String("reclaim_policy"), starlark.String("Delete")},
		{starlark.String("allow_volume_expansion"), starlark.Bool(true)},
	}
	sc, err := newKubeResource(storageClassSchema, nil, scKwargs)
	if err != nil {
		t.Fatalf("storage_class error: %v", err)
	}
	scYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{sc}, nil)
	if err != nil {
		t.Fatalf("yaml(sc) error: %v", err)
	}
	scYAML := string(scYAMLVal.(starlark.String))
	if !strings.Contains(scYAML, "kind: StorageClass") {
		t.Errorf("sc YAML missing kind: %s", scYAML)
	}
	if !strings.Contains(scYAML, "provisioner: kubernetes.io/no-provisioner") {
		t.Errorf("sc YAML missing provisioner: %s", scYAML)
	}
	if !strings.Contains(scYAML, "volumeBindingMode: WaitForFirstConsumer") {
		t.Errorf("sc YAML missing volumeBindingMode: %s", scYAML)
	}
	if !strings.Contains(scYAML, "allowVolumeExpansion: true") {
		t.Errorf("sc YAML missing allowVolumeExpansion: %s", scYAML)
	}
	if strings.Contains(scYAML, "spec:") {
		t.Errorf("sc YAML should not have spec: %s", scYAML)
	}

	// 3. PersistentVolumeClaim
	pvcKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("data-pvc")},
		{starlark.String("storage"), starlark.String("20Gi")},
		{starlark.String("storage_class_name"), starlark.String("fast-ssd")},
		{starlark.String("access_modes"), starlark.NewList([]starlark.Value{starlark.String("ReadWriteOnce")})},
		{starlark.String("volume_name"), starlark.String("pv-local")},
	}
	pvc, err := newKubeResource(pvcSchema, nil, pvcKwargs)
	if err != nil {
		t.Fatalf("pvc error: %v", err)
	}
	pvcYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{pvc}, nil)
	if err != nil {
		t.Fatalf("yaml(pvc) error: %v", err)
	}
	pvcYAML := string(pvcYAMLVal.(starlark.String))
	if !strings.Contains(pvcYAML, "kind: PersistentVolumeClaim") {
		t.Errorf("pvc YAML missing kind: %s", pvcYAML)
	}
	if !strings.Contains(pvcYAML, "storage: 20Gi") {
		t.Errorf("pvc YAML missing storage: %s", pvcYAML)
	}
	if !strings.Contains(pvcYAML, "storageClassName: fast-ssd") {
		t.Errorf("pvc YAML missing storageClassName: %s", pvcYAML)
	}
	if !strings.Contains(pvcYAML, "volumeName: pv-local") {
		t.Errorf("pvc YAML missing volumeName: %s", pvcYAML)
	}
}

func TestVolume_SubObjectShortcuts(t *testing.T) {
	// 1. PVC as string
	v1, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-1")},
		{starlark.String("pvc"), starlark.String("my-pvc")},
	})
	if err != nil {
		t.Fatalf("v1 error: %v", err)
	}
	d1 := v1.ToDict()
	pvcVal1, found, _ := d1.Get(starlark.String("persistentVolumeClaim"))
	if !found {
		t.Fatal("v1 missing persistentVolumeClaim")
	}
	cn1, _, _ := pvcVal1.(*starlark.Dict).Get(starlark.String("claimName"))
	if cn1 == nil || cn1.String() != `"my-pvc"` {
		t.Errorf("v1 claimName = %v, want my-pvc", cn1)
	}

	// 2. PVC as KubeResource
	pvcRes, _ := newKubeResource(pvcSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("obj-pvc")},
		{starlark.String("storage"), starlark.String("5Gi")},
	})
	v2, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-2")},
		{starlark.String("pvc"), pvcRes},
	})
	if err != nil {
		t.Fatalf("v2 error: %v", err)
	}
	d2 := v2.ToDict()
	pvcVal2, found, _ := d2.Get(starlark.String("persistentVolumeClaim"))
	if !found {
		t.Fatal("v2 missing persistentVolumeClaim")
	}
	cn2, _, _ := pvcVal2.(*starlark.Dict).Get(starlark.String("claimName"))
	if cn2 == nil || cn2.String() != `"obj-pvc"` {
		t.Errorf("v2 claimName = %v, want obj-pvc", cn2)
	}

	// 3. claim_name shortcut
	v3, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-3")},
		{starlark.String("claim_name"), starlark.String("my-direct-claim")},
	})
	if err != nil {
		t.Fatalf("v3 error: %v", err)
	}
	d3 := v3.ToDict()
	pvcVal3, found, _ := d3.Get(starlark.String("persistentVolumeClaim"))
	if !found {
		t.Fatal("v3 missing persistentVolumeClaim")
	}
	cn3, _, _ := pvcVal3.(*starlark.Dict).Get(starlark.String("claimName"))
	if cn3 == nil || cn3.String() != `"my-direct-claim"` {
		t.Errorf("v3 claimName = %v, want my-direct-claim", cn3)
	}

	// 4. ConfigMap as string & KubeResource
	cmRes, _ := newKubeResource(configMapSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("cm-obj")},
	})
	v4, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-4")},
		{starlark.String("config_map"), cmRes},
	})
	if err != nil {
		t.Fatalf("v4 error: %v", err)
	}
	d4 := v4.ToDict()
	cmVal, found, _ := d4.Get(starlark.String("configMap"))
	if !found {
		t.Fatal("v4 missing configMap")
	}
	cmName, _, _ := cmVal.(*starlark.Dict).Get(starlark.String("name"))
	if cmName == nil || cmName.String() != `"cm-obj"` {
		t.Errorf("v4 cm name = %v, want cm-obj", cmName)
	}

	// 5. Secret as string & KubeResource
	secRes, _ := newKubeResource(secretSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("sec-obj")},
	})
	v5, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-5")},
		{starlark.String("secret"), secRes},
	})
	if err != nil {
		t.Fatalf("v5 error: %v", err)
	}
	d5 := v5.ToDict()
	secVal, found, _ := d5.Get(starlark.String("secret"))
	if !found {
		t.Fatal("v5 missing secret")
	}
	secName, _, _ := secVal.(*starlark.Dict).Get(starlark.String("secretName"))
	if secName == nil || secName.String() != `"sec-obj"` {
		t.Errorf("v5 secret name = %v, want sec-obj", secName)
	}

	// 6. empty_dir=True
	v6, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-6")},
		{starlark.String("empty_dir"), starlark.Bool(true)},
	})
	if err != nil {
		t.Fatalf("v6 error: %v", err)
	}
	d6 := v6.ToDict()
	edVal, found, _ := d6.Get(starlark.String("emptyDir"))
	if !found {
		t.Fatal("v6 missing emptyDir")
	}
	if edVal.(*starlark.Dict).Len() != 0 {
		t.Errorf("v6 emptyDir should be empty dict, got: %v", edVal)
	}

	// 7. ephemeral with KubeResource
	v7, err := newKubeResource(volumeSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("vol-7")},
		{starlark.String("ephemeral"), pvcRes},
	})
	if err != nil {
		t.Fatalf("v7 error: %v", err)
	}
	d7 := v7.ToDict()
	ephVal, found, _ := d7.Get(starlark.String("ephemeral"))
	if !found {
		t.Fatal("v7 missing ephemeral")
	}
	vctVal, found, _ := ephVal.(*starlark.Dict).Get(starlark.String("volumeClaimTemplate"))
	if !found {
		t.Fatal("v7 missing volumeClaimTemplate")
	}
	vctSpec, found, _ := vctVal.(*starlark.Dict).Get(starlark.String("spec"))
	if !found {
		t.Fatal("v7 missing volumeClaimTemplate.spec")
	}
	if !strings.Contains(vctSpec.String(), "resources") {
		t.Errorf("v7 vctSpec missing resources: %v", vctSpec)
	}
}

func TestVolume_WorkloadIntegration(t *testing.T) {
	m := New()
	loaded, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := loaded["k8s"].(starlark.HasAttrs)
	yamlFnVal, _ := k8sMod.Attr("yaml")
	yamlFn := yamlFnVal.(*starlark.Builtin)
	thread := &starlark.Thread{Name: "test"}

	// VolumeMount
	vmKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("data-vol")},
		{starlark.String("mount_path"), starlark.String("/data")},
		{starlark.String("read_only"), starlark.Bool(true)},
		{starlark.String("sub_path_expr"), starlark.String("$(POD_NAME)")},
		{starlark.String("mount_propagation"), starlark.String("HostToContainer")},
	}
	vm, err := newKubeResource(volumeMountSchema, nil, vmKwargs)
	if err != nil {
		t.Fatalf("volume_mount error: %v", err)
	}

	containerKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("app:v1")},
		{starlark.String("volume_mounts"), starlark.NewList([]starlark.Value{vm})},
	}
	c, err := newKubeResource(containerSchema, nil, containerKwargs)
	if err != nil {
		t.Fatalf("container error: %v", err)
	}

	volKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("data-vol")},
		{starlark.String("pvc"), starlark.String("shared-data")},
	}
	vol, err := newKubeResource(volumeSchema, nil, volKwargs)
	if err != nil {
		t.Fatalf("volume error: %v", err)
	}

	podKwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("storage-pod")},
		{starlark.String("containers"), starlark.NewList([]starlark.Value{c})},
		{starlark.String("volumes"), starlark.NewList([]starlark.Value{vol})},
	}
	pod, err := newKubeResource(podSchema, nil, podKwargs)
	if err != nil {
		t.Fatalf("pod error: %v", err)
	}

	podYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{pod}, nil)
	if err != nil {
		t.Fatalf("yaml(pod) error: %v", err)
	}
	podYAML := string(podYAMLVal.(starlark.String))

	if !strings.Contains(podYAML, "persistentVolumeClaim:") {
		t.Errorf("pod YAML missing persistentVolumeClaim: %s", podYAML)
	}
	if !strings.Contains(podYAML, "claimName: shared-data") {
		t.Errorf("pod YAML missing claimName: %s", podYAML)
	}
	if !strings.Contains(podYAML, "subPathExpr: $(POD_NAME)") {
		t.Errorf("pod YAML missing subPathExpr: %s", podYAML)
	}
	if !strings.Contains(podYAML, "mountPropagation: HostToContainer") {
		t.Errorf("pod YAML missing mountPropagation: %s", podYAML)
	}
	if !strings.Contains(podYAML, "readOnly: true") {
		t.Errorf("pod YAML missing readOnly: %s", podYAML)
	}
}

func TestPhase2_HostUsersAndSecurityContext(t *testing.T) {
	m := New()
	loaded, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := loaded["k8s"].(starlark.HasAttrs)
	yamlFnVal, _ := k8sMod.Attr("yaml")
	yamlFn := yamlFnVal.(*starlark.Builtin)
	thread := &starlark.Thread{Name: "test"}

	// 1. Pod with host_users = False
	c, _ := newKubeResource(containerSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("app")},
		{starlark.String("image"), starlark.String("alpine")},
	})
	secDict := starlark.NewDict(3)
	secDict.SetKey(starlark.String("run_as_non_root"), starlark.Bool(true))
	apparmorDict := starlark.NewDict(1)
	apparmorDict.SetKey(starlark.String("type"), starlark.String("RuntimeDefault"))
	secDict.SetKey(starlark.String("apparmor_profile"), apparmorDict)

	pod, err := newKubeResource(podSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("rootless-pod")},
		{starlark.String("host_users"), starlark.Bool(false)},
		{starlark.String("security_context"), secDict},
		{starlark.String("containers"), starlark.NewList([]starlark.Value{c})},
	})
	if err != nil {
		t.Fatalf("pod error: %v", err)
	}

	podYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{pod}, nil)
	if err != nil {
		t.Fatalf("yaml(pod) error: %v", err)
	}
	podYAML := string(podYAMLVal.(starlark.String))

	if !strings.Contains(podYAML, "hostUsers: false") {
		t.Errorf("pod YAML missing hostUsers: false:\n%s", podYAML)
	}
	if !strings.Contains(podYAML, "runAsNonRoot: true") {
		t.Errorf("pod YAML missing runAsNonRoot: true:\n%s", podYAML)
	}
	if !strings.Contains(podYAML, "appArmorProfile:") {
		t.Errorf("pod YAML missing appArmorProfile:\n%s", podYAML)
	}

	// 2. Deployment with host_users = False autoTemplate propagation
	deploy, err := newKubeResource(deploymentSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("rootless-deploy")},
		{starlark.String("host_users"), starlark.Bool(false)},
		{starlark.String("containers"), starlark.NewList([]starlark.Value{c})},
	})
	if err != nil {
		t.Fatalf("deploy error: %v", err)
	}
	deployYAMLVal, err := yamlFn.CallInternal(thread, starlark.Tuple{deploy}, nil)
	if err != nil {
		t.Fatalf("yaml(deploy) error: %v", err)
	}
	deployYAML := string(deployYAMLVal.(starlark.String))

	if !strings.Contains(deployYAML, "hostUsers: false") {
		t.Errorf("deploy YAML missing hostUsers: false:\n%s", deployYAML)
	}
}

func TestPhase2_ContainerResizePolicy(t *testing.T) {
	rp1 := starlark.NewDict(2)
	rp1.SetKey(starlark.String("resource_name"), starlark.String("cpu"))
	rp1.SetKey(starlark.String("restart_policy"), starlark.String("NotRequired"))

	rp2 := starlark.NewDict(2)
	rp2.SetKey(starlark.String("resource_name"), starlark.String("memory"))
	rp2.SetKey(starlark.String("restart_policy"), starlark.String("RestartContainer"))

	c, err := newKubeResource(containerSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("worker")},
		{starlark.String("image"), starlark.String("worker:latest")},
		{starlark.String("resize_policy"), starlark.NewList([]starlark.Value{rp1, rp2})},
	})
	if err != nil {
		t.Fatalf("container error: %v", err)
	}

	d := c.ToDict()
	rpVal, found, _ := d.Get(starlark.String("resizePolicy"))
	if !found {
		t.Fatal("resizePolicy not found on container")
	}
	rpList := rpVal.(*starlark.List)
	if rpList.Len() != 2 {
		t.Fatalf("resizePolicy len = %d, want 2", rpList.Len())
	}

	item0 := rpList.Index(0).(*starlark.Dict)
	resName, _, _ := item0.Get(starlark.String("resourceName"))
	restartPolicy, _, _ := item0.Get(starlark.String("restartPolicy"))
	if resName.String() != `"cpu"` || restartPolicy.String() != `"NotRequired"` {
		t.Errorf("item0 = %v, want cpu/NotRequired", item0)
	}
}

func TestPhase3_GatewayAPI(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	m := New()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := dict["k8s"].(*libkite.TryModule)
	yamlFnVal, _ := k8sMod.Attr("yaml")
	yamlFn := yamlFnVal.(*starlark.Builtin)

	// 1. GatewayClass
	gc, err := newKubeResource(gatewayClassSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("cilium")},
		{starlark.String("controller_name"), starlark.String("io.cilium/gateway-controller")},
		{starlark.String("description"), starlark.String("Cilium GatewayClass")},
	})
	if err != nil {
		t.Fatalf("gateway_class error: %v", err)
	}
	gcYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{gc}, nil)
	gcYAML := string(gcYAMLVal.(starlark.String))
	if !strings.Contains(gcYAML, "controllerName: io.cilium/gateway-controller") {
		t.Errorf("GatewayClass YAML missing controllerName:\n%s", gcYAML)
	}

	// 2. Gateway with reference to GatewayClass KubeResource and snake_case listeners
	listener := starlark.NewDict(4)
	listener.SetKey(starlark.String("name"), starlark.String("http"))
	listener.SetKey(starlark.String("port"), starlark.MakeInt(80))
	listener.SetKey(starlark.String("protocol"), starlark.String("HTTP"))
	allowedRoutes := starlark.NewDict(1)
	nsMap := starlark.NewDict(1)
	nsMap.SetKey(starlark.String("from"), starlark.String("Same"))
	allowedRoutes.SetKey(starlark.String("namespaces"), nsMap)
	listener.SetKey(starlark.String("allowed_routes"), allowedRoutes)

	gw, err := newKubeResource(gatewaySchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("prod-gw")},
		{starlark.String("gateway_class"), gc}, // KubeResource reference unwraps to "cilium"
		{starlark.String("listeners"), starlark.NewList([]starlark.Value{listener})},
	})
	if err != nil {
		t.Fatalf("gateway error: %v", err)
	}
	gwYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{gw}, nil)
	gwYAML := string(gwYAMLVal.(starlark.String))
	if !strings.Contains(gwYAML, "gatewayClassName: cilium") {
		t.Errorf("Gateway YAML missing gatewayClassName:\n%s", gwYAML)
	}
	if !strings.Contains(gwYAML, "allowedRoutes:") {
		t.Errorf("Gateway YAML missing normalized allowedRoutes:\n%s", gwYAML)
	}

	// 3. HTTPRoute with parentRefs=[gw] and snake_case rules
	rule := starlark.NewDict(2)
	match := starlark.NewDict(1)
	pathMap := starlark.NewDict(2)
	pathMap.SetKey(starlark.String("type"), starlark.String("PathPrefix"))
	pathMap.SetKey(starlark.String("value"), starlark.String("/api"))
	match.SetKey(starlark.String("path"), pathMap)
	rule.SetKey(starlark.String("matches"), starlark.NewList([]starlark.Value{match}))

	backendRef := starlark.NewDict(2)
	backendRef.SetKey(starlark.String("name"), starlark.String("api-svc"))
	backendRef.SetKey(starlark.String("port"), starlark.MakeInt(8080))
	rule.SetKey(starlark.String("backend_refs"), starlark.NewList([]starlark.Value{backendRef}))

	route, err := newKubeResource(httpRouteSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("api-route")},
		{starlark.String("parent_refs"), starlark.NewList([]starlark.Value{gw})}, // KubeResource reference
		{starlark.String("rules"), starlark.NewList([]starlark.Value{rule})},
	})
	if err != nil {
		t.Fatalf("http_route error: %v", err)
	}
	routeYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{route}, nil)
	routeYAML := string(routeYAMLVal.(starlark.String))
	if !strings.Contains(routeYAML, "parentRefs:") || !strings.Contains(routeYAML, "name: prod-gw") {
		t.Errorf("HTTPRoute YAML missing parentRefs pointing to prod-gw:\n%s", routeYAML)
	}
	if !strings.Contains(routeYAML, "backendRefs:") || !strings.Contains(routeYAML, "name: api-svc") {
		t.Errorf("HTTPRoute YAML missing normalized backendRefs:\n%s", routeYAML)
	}

	// 4. GRPCRoute
	grpcRoute, err := newKubeResource(grpcRouteSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("grpc-route")},
		{starlark.String("parent_refs"), starlark.NewList([]starlark.Value{starlark.String("prod-gw")})},
		{starlark.String("rules"), starlark.NewList([]starlark.Value{rule})},
	})
	if err != nil {
		t.Fatalf("grpc_route error: %v", err)
	}
	grpcYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{grpcRoute}, nil)
	grpcYAML := string(grpcYAMLVal.(starlark.String))
	if !strings.Contains(grpcYAML, "kind: GRPCRoute") {
		t.Errorf("GRPCRoute YAML missing kind:\n%s", grpcYAML)
	}

	// 5. ReferenceGrant
	grant, err := newKubeResource(referenceGrantSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("grant-ref")},
		{starlark.String("from"), starlark.NewList([]starlark.Value{
			starlark.NewDict(2),
		})},
		{starlark.String("to"), starlark.NewList([]starlark.Value{
			starlark.NewDict(2),
		})},
	})
	if err != nil {
		t.Fatalf("reference_grant error: %v", err)
	}
	grantYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{grant}, nil)
	grantYAML := string(grantYAMLVal.(starlark.String))
	if !strings.Contains(grantYAML, "kind: ReferenceGrant") {
		t.Errorf("ReferenceGrant YAML missing kind:\n%s", grantYAML)
	}
}

func TestPhase3_CELAdmissionGovernance(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	m := New()
	dict, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	k8sMod := dict["k8s"].(*libkite.TryModule)
	yamlFnVal, _ := k8sMod.Attr("yaml")
	yamlFn := yamlFnVal.(*starlark.Builtin)

	// 1. ValidatingAdmissionPolicy with match_constraints and validations
	v1 := starlark.NewDict(2)
	v1.SetKey(starlark.String("expression"), starlark.String("!object.spec.containers.exists(c, c.securityContext.?privileged.orValue(false))"))
	v1.SetKey(starlark.String("message"), starlark.String("Privileged containers are not allowed"))

	rule := starlark.NewDict(4)
	rule.SetKey(starlark.String("api_groups"), starlark.NewList([]starlark.Value{starlark.String("")}))
	rule.SetKey(starlark.String("api_versions"), starlark.NewList([]starlark.Value{starlark.String("v1")}))
	rule.SetKey(starlark.String("resources"), starlark.NewList([]starlark.Value{starlark.String("pods")}))
	rule.SetKey(starlark.String("operations"), starlark.NewList([]starlark.Value{starlark.String("CREATE")}))

	matchConstraints := starlark.NewDict(1)
	matchConstraints.SetKey(starlark.String("resource_rules"), starlark.NewList([]starlark.Value{rule}))

	vap, err := newKubeResource(validatingAdmissionPolicySchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("disallow-privileged")},
		{starlark.String("validations"), starlark.NewList([]starlark.Value{v1})},
		{starlark.String("match_constraints"), matchConstraints},
	})
	if err != nil {
		t.Fatalf("validating_admission_policy error: %v", err)
	}
	vapYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{vap}, nil)
	vapYAML := string(vapYAMLVal.(starlark.String))
	if !strings.Contains(vapYAML, "kind: ValidatingAdmissionPolicy") {
		t.Errorf("VAP YAML missing kind:\n%s", vapYAML)
	}
	if !strings.Contains(vapYAML, "matchConstraints:") || !strings.Contains(vapYAML, "resourceRules:") {
		t.Errorf("VAP YAML missing normalized matchConstraints/resourceRules:\n%s", vapYAML)
	}
	if !strings.Contains(vapYAML, "apiGroups:") || !strings.Contains(vapYAML, "apiVersions:") {
		t.Errorf("VAP YAML missing normalized apiGroups/apiVersions:\n%s", vapYAML)
	}

	// 2. ValidatingAdmissionPolicyBinding referencing vap
	binding, err := newKubeResource(validatingAdmissionPolicyBindingSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("disallow-privileged-prod")},
		{starlark.String("policy_name"), vap}, // KubeResource unwraps to "disallow-privileged"
		{starlark.String("validation_actions"), starlark.NewList([]starlark.Value{starlark.String("Deny")})},
	})
	if err != nil {
		t.Fatalf("validating_admission_policy_binding error: %v", err)
	}
	bindingYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{binding}, nil)
	bindingYAML := string(bindingYAMLVal.(starlark.String))
	if !strings.Contains(bindingYAML, "policyName: disallow-privileged") {
		t.Errorf("Binding YAML missing policyName:\n%s", bindingYAML)
	}
	if !strings.Contains(bindingYAML, "validationActions:") {
		t.Errorf("Binding YAML missing validationActions:\n%s", bindingYAML)
	}

	// 3. MutatingAdmissionPolicy
	mut := starlark.NewDict(2)
	mut.SetKey(starlark.String("patch_type"), starlark.String("ApplyConfiguration"))
	appConf := starlark.NewDict(1)
	appConf.SetKey(starlark.String("expression"), starlark.String("Object{metadata: ObjectMeta{annotations: {'sidecar': 'true'}}}"))
	mut.SetKey(starlark.String("apply_configuration"), appConf)

	mapObj, err := newKubeResource(mutatingAdmissionPolicySchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("inject-sidecar")},
		{starlark.String("mutations"), starlark.NewList([]starlark.Value{mut})},
	})
	if err != nil {
		t.Fatalf("mutating_admission_policy error: %v", err)
	}
	mapYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{mapObj}, nil)
	mapYAML := string(mapYAMLVal.(starlark.String))
	if !strings.Contains(mapYAML, "kind: MutatingAdmissionPolicy") {
		t.Errorf("MAP YAML missing kind:\n%s", mapYAML)
	}
	if !strings.Contains(mapYAML, "patchType: ApplyConfiguration") {
		t.Errorf("MAP YAML missing normalized patchType:\n%s", mapYAML)
	}
	if !strings.Contains(mapYAML, "applyConfiguration:") {
		t.Errorf("MAP YAML missing normalized applyConfiguration:\n%s", mapYAML)
	}

	// 4. MutatingAdmissionPolicyBinding
	mapBinding, err := newKubeResource(mutatingAdmissionPolicyBindingSchema, nil, []starlark.Tuple{
		{starlark.String("name"), starlark.String("inject-sidecar-binding")},
		{starlark.String("policy_name"), mapObj},
	})
	if err != nil {
		t.Fatalf("mutating_admission_policy_binding error: %v", err)
	}
	mapBindingYAMLVal, _ := yamlFn.CallInternal(thread, starlark.Tuple{mapBinding}, nil)
	mapBindingYAML := string(mapBindingYAMLVal.(starlark.String))
	if !strings.Contains(mapBindingYAML, "policyName: inject-sidecar") {
		t.Errorf("MAP Binding YAML missing policyName:\n%s", mapBindingYAML)
	}

	// 5. Test alias constructors "vap" and "map"
	objConstructors := ObjConstructors()
	if _, ok := objConstructors["vap"]; !ok {
		t.Error("k8s.obj.vap constructor not found")
	}
	if _, ok := objConstructors["map"]; !ok {
		t.Error("k8s.obj.map constructor not found")
	}
}
