package containers

import (
	"fmt"
	"strings"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

// AttrDict is an alias to the canonical libkite.AttrDict.
type AttrDict = libkite.AttrDict

// NewAttrDict constructs an AttrDict using libkite.NewAttrDict.
var NewAttrDict = libkite.NewAttrDict

// newContainerAttrDict constructs an AttrDict representation of a container.
func newContainerAttrDict(id, name, image, status string, ports map[string]any) *AttrDict {
	cleanName := strings.TrimPrefix(name, "/")
	if ports == nil {
		ports = make(map[string]any)
	}
	data := map[string]any{
		"id":     id,
		"name":   cleanName,
		"image":  image,
		"status": status,
		"ports":  ports,
	}
	return NewAttrDict(data)
}

// extractTargetID extracts a container ID or name string from an AttrDict, dict, or string.
func extractTargetID(target starlark.Value) (string, error) {
	if target == nil || target == starlark.None {
		return "", fmt.Errorf("containers: target container cannot be None")
	}
	switch v := target.(type) {
	case starlark.String:
		s := string(v)
		if s == "" {
			return "", fmt.Errorf("containers: target container ID/name cannot be empty")
		}
		return s, nil
	case *AttrDict:
		data := v.ToMap()
		if id, ok := data["id"].(string); ok && id != "" {
			return id, nil
		}
		if id, ok := data["Id"].(string); ok && id != "" {
			return id, nil
		}
		if name, ok := data["name"].(string); ok && name != "" {
			return name, nil
		}
		if name, ok := data["Name"].(string); ok && name != "" {
			return name, nil
		}
		return "", fmt.Errorf("containers: target AttrDict has no 'id' or 'name' field")
	case *starlark.Dict:
		for _, key := range []string{"id", "Id", "name", "Name"} {
			if val, found, _ := v.Get(starlark.String(key)); found {
				if s, ok := starlark.AsString(val); ok && s != "" {
					return s, nil
				}
			}
		}
		return "", fmt.Errorf("containers: target dict has no 'id' or 'name' field")
	default:
		return "", fmt.Errorf("containers: expected container AttrDict/dict or string ID/name, got %s", target.Type())
	}
}

// extractTargetName extracts a container name or returns a fallback string.
func extractTargetName(target starlark.Value, fallback string) string {
	switch v := target.(type) {
	case *AttrDict:
		data := v.ToMap()
		if name, ok := data["name"].(string); ok && name != "" {
			return name
		}
		if name, ok := data["Name"].(string); ok && name != "" {
			return name
		}
	case *starlark.Dict:
		for _, key := range []string{"name", "Name"} {
			if val, found, _ := v.Get(starlark.String(key)); found {
				if s, ok := starlark.AsString(val); ok && s != "" {
					return s
				}
			}
		}
	}
	return fallback
}
