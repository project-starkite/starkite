package k8s

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// crdConstructor builds a CustomResourceDefinition as a KubeResource.
// Usage: k8s.obj.crd(group, version, kind, plural, scope="Namespaced", spec={}, status={})
func crdConstructor(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Extract spec and status dicts before startype parsing
	var specDict, statusDict *starlark.Dict
	filtered := filterKwarg(kwargs, "spec", &specDict)
	filtered = filterKwarg(filtered, "status", &statusDict)

	var p struct {
		Group   string `name:"group" position:"0" required:"true"`
		Version string `name:"version" position:"1" required:"true"`
		Kind    string `name:"kind" position:"2" required:"true"`
		Plural  string `name:"plural" position:"3" required:"true"`
		Scope   string `name:"scope"`
	}
	p.Scope = "Namespaced"
	if err := startype.Args(args, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.obj.crd: %w", err)
	}

	if p.Scope != "Namespaced" && p.Scope != "Cluster" {
		return nil, fmt.Errorf("k8s.obj.crd: scope must be 'Namespaced' or 'Cluster', got %q", p.Scope)
	}

	singular := strings.ToLower(p.Kind)

	// Build OpenAPI v3 schema from spec dict
	specSchema := map[string]interface{}{
		"type": "object",
	}
	if specDict != nil && specDict.Len() > 0 {
		props, required, err := buildOpenAPIProperties(specDict)
		if err != nil {
			return nil, fmt.Errorf("k8s.obj.crd: spec: %w", err)
		}
		specSchema["properties"] = props
		if len(required) > 0 {
			specSchema["required"] = required
		}
	}

	// Build status schema
	var statusSchema map[string]interface{}
	if statusDict != nil && statusDict.Len() > 0 {
		props, required, err := buildOpenAPIProperties(statusDict)
		if err != nil {
			return nil, fmt.Errorf("k8s.obj.crd: status: %w", err)
		}
		statusSchema = map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			statusSchema["required"] = required
		}
	}

	// Build the full openAPIV3Schema
	schemaProps := map[string]interface{}{
		"spec": specSchema,
	}
	if statusSchema != nil {
		schemaProps["status"] = statusSchema
	}

	openAPISchema := map[string]interface{}{
		"type": "object",
		"properties": schemaProps,
	}

	// Build version entry
	versionEntry := map[string]interface{}{
		"name":    p.Version,
		"served":  true,
		"storage": true,
		"schema": map[string]interface{}{
			"openAPIV3Schema": openAPISchema,
		},
	}
	if statusSchema != nil {
		versionEntry["subresources"] = map[string]interface{}{
			"status": map[string]interface{}{},
		}
	}

	// Build the CRD as a KubeResource with a custom schema
	crdSchema := &ResourceSchema{
		Kind:       "CustomResourceDefinition",
		APIVersion: "apiextensions.k8s.io/v1",
	}

	// Build the full object data as a nested map that ToDict will serialize
	data := map[string]any{
		"name": p.Plural + "." + p.Group,
		"_raw": map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": p.Plural + "." + p.Group,
			},
			"spec": map[string]interface{}{
				"group": p.Group,
				"names": map[string]interface{}{
					"kind":     p.Kind,
					"plural":   p.Plural,
					"singular": singular,
				},
				"scope":    p.Scope,
				"versions": []interface{}{versionEntry},
			},
		},
	}

	return &CRDResource{
		schema: crdSchema,
		data:   data,
		raw:    data["_raw"].(map[string]interface{}),
	}, nil
}

// CRDResource is a KubeResource variant that serializes from a pre-built raw map.
type CRDResource struct {
	schema *ResourceSchema
	data   map[string]any
	raw    map[string]interface{}
}

var _ KubeObject = (*CRDResource)(nil)
var _ startype.DictConvertible = (*CRDResource)(nil)

func (r *CRDResource) String() string {
	name, _ := r.data["name"].(string)
	return fmt.Sprintf("<crd name=%q>", name)
}
func (r *CRDResource) Type() string          { return "k8s.obj.crd" }
func (r *CRDResource) Freeze()               {}
func (r *CRDResource) Truth() starlark.Bool   { return starlark.True }
func (r *CRDResource) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: k8s.obj.crd") }
func (r *CRDResource) Kind() string           { return "CustomResourceDefinition" }

func (r *CRDResource) Attr(name string) (starlark.Value, error) {
	if name == "to_dict" {
		return starlark.NewBuiltin("k8s.obj.crd.to_dict", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return r.ToDict(), nil
		}), nil
	}
	return nil, nil
}

func (r *CRDResource) AttrNames() []string {
	return []string{"to_dict"}
}

func (r *CRDResource) ToDict() *starlark.Dict {
	dict, err := startype.Map(r.raw).ToDict()
	if err != nil {
		return starlark.NewDict(0)
	}
	return dict
}

// ============================================================================
// OpenAPI schema builder
// ============================================================================

// buildOpenAPIProperties converts a Starlark dict of field definitions to OpenAPI v3 properties.
// Input format: {"field_name": {"type": "string", "required": True, "default": "value"}}
// Returns: (properties map, required field names list, error)
func buildOpenAPIProperties(dict *starlark.Dict) (map[string]interface{}, []string, error) {
	props := make(map[string]interface{})
	var required []string

	for _, item := range dict.Items() {
		fieldName, ok := starlark.AsString(item[0])
		if !ok {
			continue
		}

		fieldDef, ok := item[1].(*starlark.Dict)
		if !ok {
			continue
		}

		prop, isRequired, err := buildOpenAPIProp(fieldDef)
		if err != nil {
			return nil, nil, fmt.Errorf("field %q: %w", fieldName, err)
		}
		props[fieldName] = prop
		if isRequired {
			required = append(required, fieldName)
		}
	}

	sort.Strings(required)
	return props, required, nil
}

func buildOpenAPIProp(def *starlark.Dict) (map[string]interface{}, bool, error) {
	prop := make(map[string]interface{})
	isRequired := false

	for _, item := range def.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			continue
		}

		switch key {
		case "type":
			if s, ok := starlark.AsString(item[1]); ok {
				prop["type"] = s
			}
		case "required":
			if b, ok := item[1].(starlark.Bool); ok {
				isRequired = bool(b)
			}
		case "default":
			var goVal interface{}
			if err := startype.Starlark(item[1]).Go(&goVal); err == nil {
				prop["default"] = goVal
			}
		case "description":
			if s, ok := starlark.AsString(item[1]); ok {
				prop["description"] = s
			}
		case "properties":
			if subDict, ok := item[1].(*starlark.Dict); ok {
				subProps, subRequired, err := buildOpenAPIProperties(subDict)
				if err != nil {
					return nil, false, err
				}
				prop["properties"] = subProps
				if len(subRequired) > 0 {
					prop["required"] = subRequired
				}
			}
		case "items":
			if subDict, ok := item[1].(*starlark.Dict); ok {
				subProp, _, err := buildOpenAPIProp(subDict)
				if err != nil {
					return nil, false, err
				}
				prop["items"] = subProp
			}
		}
	}

	return prop, isRequired, nil
}
