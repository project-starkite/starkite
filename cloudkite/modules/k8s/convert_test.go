package k8s

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestDictToUnstructured(t *testing.T) {
	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("apiVersion"), starlark.String("v1"))
	dict.SetKey(starlark.String("kind"), starlark.String("Pod"))

	obj, err := dictToUnstructured(dict)
	if err != nil {
		t.Fatalf("dictToUnstructured error: %v", err)
	}
	if obj.GetKind() != "Pod" {
		t.Errorf("Kind = %q, want %q", obj.GetKind(), "Pod")
	}
	if obj.GetAPIVersion() != "v1" {
		t.Errorf("APIVersion = %q, want %q", obj.GetAPIVersion(), "v1")
	}
}

func TestUnstructuredToDict(t *testing.T) {
	obj := &unstructuredObj{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}

	dict, err := unstructuredToDict(obj)
	if err != nil {
		t.Fatalf("unstructuredToDict error: %v", err)
	}

	kind, _, _ := dict.Get(starlark.String("kind"))
	if s, ok := kind.(starlark.String); !ok || string(s) != "ConfigMap" {
		t.Errorf("kind = %v, want ConfigMap", kind)
	}
}

func TestDictRoundTrip(t *testing.T) {
	original := starlark.NewDict(4)
	original.SetKey(starlark.String("apiVersion"), starlark.String("apps/v1"))
	original.SetKey(starlark.String("kind"), starlark.String("Deployment"))

	metadata := starlark.NewDict(1)
	metadata.SetKey(starlark.String("name"), starlark.String("nginx"))
	original.SetKey(starlark.String("metadata"), metadata)

	obj, err := dictToUnstructured(original)
	if err != nil {
		t.Fatalf("dictToUnstructured error: %v", err)
	}

	result, err := unstructuredToDict(obj)
	if err != nil {
		t.Fatalf("unstructuredToDict error: %v", err)
	}

	kind, _, _ := result.Get(starlark.String("kind"))
	if s, ok := kind.(starlark.String); !ok || string(s) != "Deployment" {
		t.Errorf("kind = %v, want Deployment", kind)
	}
}

func TestParseYAML(t *testing.T) {
	yaml := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value`

	dict, err := parseYAML(yaml)
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}

	kind, _, _ := dict.Get(starlark.String("kind"))
	if s, ok := kind.(starlark.String); !ok || string(s) != "ConfigMap" {
		t.Errorf("kind = %v, want ConfigMap", kind)
	}
}

func TestYamlToUnstructuredList(t *testing.T) {
	yaml := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm2`

	objs, err := yamlToUnstructuredList(yaml)
	if err != nil {
		t.Fatalf("yamlToUnstructuredList error: %v", err)
	}
	if len(objs) != 2 {
		t.Errorf("got %d objects, want 2", len(objs))
	}
	if objs[0].GetName() != "cm1" {
		t.Errorf("obj[0].name = %q, want %q", objs[0].GetName(), "cm1")
	}
	if objs[1].GetName() != "cm2" {
		t.Errorf("obj[1].name = %q, want %q", objs[1].GetName(), "cm2")
	}
}

func TestYamlToUnstructuredListEmpty(t *testing.T) {
	_, err := yamlToUnstructuredList("")
	if err == nil {
		t.Error("expected error for empty YAML")
	}
}

func TestResolveManifest_Dict(t *testing.T) {
	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("kind"), starlark.String("Pod"))

	result, err := resolveManifest(dict)
	if err != nil {
		t.Fatalf("resolveManifest error: %v", err)
	}
	if result != dict {
		t.Error("resolveManifest should pass through dicts")
	}
}

func TestResolveManifest_YAML(t *testing.T) {
	yaml := starlark.String("kind: Pod\napiVersion: v1")
	result, err := resolveManifest(yaml)
	if err != nil {
		t.Fatalf("resolveManifest error: %v", err)
	}
	kind, _, _ := result.Get(starlark.String("kind"))
	if s, ok := kind.(starlark.String); !ok || string(s) != "Pod" {
		t.Errorf("kind = %v, want Pod", kind)
	}
}

func TestResolveManifest_Invalid(t *testing.T) {
	_, err := resolveManifest(starlark.MakeInt(42))
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestParseManifest_List(t *testing.T) {
	dict1 := starlark.NewDict(2)
	dict1.SetKey(starlark.String("apiVersion"), starlark.String("v1"))
	dict1.SetKey(starlark.String("kind"), starlark.String("Pod"))

	dict2 := starlark.NewDict(2)
	dict2.SetKey(starlark.String("apiVersion"), starlark.String("v1"))
	dict2.SetKey(starlark.String("kind"), starlark.String("Service"))

	list := starlark.NewList([]starlark.Value{dict1, dict2})

	objs, err := parseManifest(list)
	if err != nil {
		t.Fatalf("parseManifest error: %v", err)
	}
	if len(objs) != 2 {
		t.Errorf("got %d objects, want 2", len(objs))
	}
}

func TestToYAMLString_Dict(t *testing.T) {
	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("apiVersion"), starlark.String("v1"))
	dict.SetKey(starlark.String("kind"), starlark.String("Pod"))

	yaml, err := toYAMLString(dict)
	if err != nil {
		t.Fatalf("toYAMLString error: %v", err)
	}
	if yaml == "" {
		t.Error("toYAMLString returned empty string")
	}
}

func TestToYAMLString_List(t *testing.T) {
	dict1 := starlark.NewDict(1)
	dict1.SetKey(starlark.String("kind"), starlark.String("Pod"))

	dict2 := starlark.NewDict(1)
	dict2.SetKey(starlark.String("kind"), starlark.String("Service"))

	list := starlark.NewList([]starlark.Value{dict1, dict2})

	yaml, err := toYAMLString(list)
	if err != nil {
		t.Fatalf("toYAMLString error: %v", err)
	}
	if yaml == "" {
		t.Error("toYAMLString returned empty string")
	}
}
