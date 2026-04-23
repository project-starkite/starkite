package mcp

import (
	"testing"
)

func TestSchemaMapToJSONSchema_Nil(t *testing.T) {
	s, err := schemaMapToJSONSchema(nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s.Type != "object" {
		t.Errorf("Type = %q, want object", s.Type)
	}
}

func TestSchemaMapToJSONSchema_SimpleObject(t *testing.T) {
	in := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"count": map[string]any{"type": "integer"},
		},
		"required": []any{"name"},
	}
	s, err := schemaMapToJSONSchema(in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s.Type != "object" {
		t.Errorf("Type = %q, want object", s.Type)
	}
	if len(s.Properties) != 2 {
		t.Errorf("Properties len = %d, want 2", len(s.Properties))
	}
	if len(s.Required) != 1 || s.Required[0] != "name" {
		t.Errorf("Required = %v, want [name]", s.Required)
	}
}

func TestSchemaMapToJSONSchema_Nested(t *testing.T) {
	in := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"address": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"street": map[string]any{"type": "string"},
				},
			},
		},
	}
	s, err := schemaMapToJSONSchema(in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	addr, ok := s.Properties["address"]
	if !ok {
		t.Fatal("missing address property")
	}
	if addr.Type != "object" {
		t.Errorf("address.Type = %q, want object", addr.Type)
	}
	if _, ok := addr.Properties["street"]; !ok {
		t.Error("address.properties missing street")
	}
}
