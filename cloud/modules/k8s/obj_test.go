package k8s

import (
	"testing"

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
	found := false
	for _, n := range names {
		if n == "to_dict" {
			found = true
			break
		}
	}
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

	// Check a few expected constructors
	expected := []string{"pod", "deployment", "service", "container", "config_map", "secret"}
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
