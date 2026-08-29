package k8s

import (
	"bytes"
	"fmt"
	"io"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Type aliases for brevity across the k8s package.
type (
	unstructuredObj  = unstructured.Unstructured
	unstructuredList = unstructured.UnstructuredList
)

// dictToUnstructured converts a Starlark dictionary or AttrDict to an *unstructured.Unstructured.
func dictToUnstructured(val starlark.Value) (*unstructured.Unstructured, error) {
	switch v := val.(type) {
	case *AttrDict:
		return &unstructured.Unstructured{Object: v.ToMap()}, nil
	case *starlark.Dict:
		m, err := startype.Dict(v).ToMap()
		if err != nil {
			return nil, fmt.Errorf("dict to map: %w", err)
		}
		return &unstructured.Unstructured{Object: m}, nil
	default:
		return nil, fmt.Errorf("expected dict or AttrDict, got %s", val.Type())
	}
}

// unstructuredToDict converts an *unstructured.Unstructured to an *AttrDict.
func unstructuredToDict(obj *unstructured.Unstructured) (*AttrDict, error) {
	if obj == nil {
		return nil, nil
	}
	return unstructuredToAttrDict(obj), nil
}

// parseYAML parses a YAML string into a *starlark.Dict.
func parseYAML(s string) (*starlark.Dict, error) {
	var m map[string]any
	if err := yaml.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return startype.Map(m).ToDict()
}

// yamlToUnstructuredList parses multi-document YAML into a list of Unstructured objects.
func yamlToUnstructuredList(yamlStr string) ([]*unstructured.Unstructured, error) {
	var result []*unstructured.Unstructured
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)))

	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("yaml decode: %w", err)
		}
		if doc == nil {
			continue
		}
		result = append(result, &unstructured.Unstructured{Object: doc})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no documents found in YAML")
	}

	return result, nil
}

// resolveManifest converts a Starlark value to a *starlark.Dict or *AttrDict.
// Handles KubeObject (via ToDict), *AttrDict, *starlark.Dict, and YAML strings.
func resolveManifest(val starlark.Value) (starlark.Value, error) {
	switch v := val.(type) {
	case KubeObject:
		return v.ToDict(), nil
	case *AttrDict:
		return v, nil
	case *starlark.Dict:
		return v, nil
	case starlark.String:
		return parseYAML(string(v))
	default:
		return nil, fmt.Errorf("expected k8s object, dict, or YAML string, got %s", val.Type())
	}
}

// parseManifest converts a Starlark value to a list of Unstructured objects.
// Handles: KubeObject, *AttrDict, *starlark.Dict, YAML string (including multi-doc), *starlark.List.
func parseManifest(val starlark.Value) ([]*unstructured.Unstructured, error) {
	switch v := val.(type) {
	case KubeObject:
		dict := v.ToDict()
		obj, err := dictToUnstructured(dict)
		if err != nil {
			return nil, err
		}
		return []*unstructured.Unstructured{obj}, nil

	case *AttrDict:
		obj, err := dictToUnstructured(v)
		if err != nil {
			return nil, err
		}
		return []*unstructured.Unstructured{obj}, nil

	case *starlark.Dict:
		obj, err := dictToUnstructured(v)
		if err != nil {
			return nil, err
		}
		return []*unstructured.Unstructured{obj}, nil

	case starlark.String:
		return yamlToUnstructuredList(string(v))

	case *starlark.List:
		var result []*unstructured.Unstructured
		for i := 0; i < v.Len(); i++ {
			objs, err := parseManifest(v.Index(i))
			if err != nil {
				return nil, fmt.Errorf("list[%d]: %w", i, err)
			}
			result = append(result, objs...)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("expected k8s object, dict, YAML string, or list, got %s", val.Type())
	}
}

// toYAMLString converts a Starlark value (KubeObject, AttrDict, dict, or list) to a YAML string.
func toYAMLString(val starlark.Value) (string, error) {
	switch v := val.(type) {
	case KubeObject:
		m, err := startype.Dict(v.ToDict()).ToMap()
		if err != nil {
			return "", err
		}
		return marshalYAML(m)

	case *AttrDict:
		return marshalYAML(v.ToMap())

	case *starlark.Dict:
		m, err := startype.Dict(v).ToMap()
		if err != nil {
			return "", err
		}
		return marshalYAML(m)

	case *starlark.List:
		var buf bytes.Buffer
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				buf.WriteString("---\n")
			}
			s, err := toYAMLString(v.Index(i))
			if err != nil {
				return "", fmt.Errorf("list[%d]: %w", i, err)
			}
			buf.WriteString(s)
		}
		return buf.String(), nil

	default:
		return "", fmt.Errorf("expected k8s object, dict, or list, got %s", val.Type())
	}
}

func marshalYAML(m map[string]any) (string, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("yaml marshal: %w", err)
	}
	return string(data), nil
}
