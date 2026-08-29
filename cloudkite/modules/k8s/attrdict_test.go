package k8s

import (
	"testing"

	"go.starlark.net/starlark"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAttrDict_DotAccessAndBracketAccess(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "nginx-pod",
				"namespace": "prod",
				"labels": map[string]any{
					"app": "web",
				},
			},
			"status": map[string]any{
				"phase": "Running",
			},
		},
	}

	attrDict := unstructuredToAttrDict(obj)

	// 1. Top-level dot access
	kindVal, err := attrDict.Attr("kind")
	if err != nil {
		t.Fatalf("attrDict.Attr(kind) error: %v", err)
	}
	if s, ok := kindVal.(starlark.String); !ok || string(s) != "Pod" {
		t.Errorf("kind = %v, want Pod", kindVal)
	}

	// 2. Nested dot access
	metaVal, err := attrDict.Attr("metadata")
	if err != nil {
		t.Fatalf("attrDict.Attr(metadata) error: %v", err)
	}
	metaDict, ok := metaVal.(*AttrDict)
	if !ok {
		t.Fatalf("metadata should be *AttrDict, got %T", metaVal)
	}
	nameVal, err := metaDict.Attr("name")
	if err != nil {
		t.Fatalf("metaDict.Attr(name) error: %v", err)
	}
	if s, ok := nameVal.(starlark.String); !ok || string(s) != "nginx-pod" {
		t.Errorf("name = %v, want nginx-pod", nameVal)
	}

	// 3. Deep nested dot access
	labelsVal, err := metaDict.Attr("labels")
	if err != nil {
		t.Fatalf("metaDict.Attr(labels) error: %v", err)
	}
	labelsDict, ok := labelsVal.(*AttrDict)
	if !ok {
		t.Fatalf("labels should be *AttrDict, got %T", labelsVal)
	}
	appVal, err := labelsDict.Attr("app")
	if err != nil {
		t.Fatalf("labelsDict.Attr(app) error: %v", err)
	}
	if s, ok := appVal.(starlark.String); !ok || string(s) != "web" {
		t.Errorf("app = %v, want web", appVal)
	}

	// 4. Bracket access
	statusVal, found, err := attrDict.Get(starlark.String("status"))
	if err != nil || !found {
		t.Fatalf("attrDict.Get(status) error: %v, found: %v", err, found)
	}
	statusDict, ok := statusVal.(*AttrDict)
	if !ok {
		t.Fatalf("status should be *AttrDict, got %T", statusVal)
	}
	phaseVal, found, err := statusDict.Get(starlark.String("phase"))
	if err != nil || !found {
		t.Fatalf("statusDict.Get(phase) error: %v, found: %v", err, found)
	}
	if s, ok := phaseVal.(starlark.String); !ok || string(s) != "Running" {
		t.Errorf("phase = %v, want Running", phaseVal)
	}

	// 5. Length and iteration
	if attrDict.Len() != 4 {
		t.Errorf("attrDict.Len() = %d, want 4", attrDict.Len())
	}
	items := attrDict.Items()
	if len(items) != 4 {
		t.Errorf("attrDict.Items() len = %d, want 4", len(items))
	}
}

func TestAttrDict_Mutation(t *testing.T) {
	attrDict := NewAttrDict(map[string]any{
		"metadata": map[string]any{
			"name": "old-name",
		},
	})

	metaVal, _ := attrDict.Attr("metadata")
	metaDict := metaVal.(*AttrDict)
	if err := metaDict.SetKey(starlark.String("name"), starlark.String("new-name")); err != nil {
		t.Fatalf("SetKey error: %v", err)
	}

	nameVal, _ := metaDict.Attr("name")
	if s, ok := nameVal.(starlark.String); !ok || string(s) != "new-name" {
		t.Errorf("name = %v, want new-name", nameVal)
	}
}
