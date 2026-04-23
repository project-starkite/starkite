package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemaMapToJSONSchema converts a raw JSON Schema (as a Go map produced by
// decoding a Starlark dict) into the SDK's *jsonschema.Schema value.
//
// Round-trips via JSON: any standard JSON Schema field (type, properties,
// required, default, enum, min/max, pattern, items, ...) is preserved. Custom
// extensions (`x-*`) may be dropped — documented limitation.
//
// A nil map is treated as an empty object schema (MCP servers still want a
// schema; `{"type":"object"}` is the least-restrictive form).
func schemaMapToJSONSchema(m map[string]any) (*jsonschema.Schema, error) {
	if m == nil {
		return &jsonschema.Schema{Type: "object"}, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return &s, nil
}
