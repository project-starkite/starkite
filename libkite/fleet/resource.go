package fleet

import (
	"fmt"
	"maps"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Resource represents a single compute entity in a fleet.
type Resource struct {
	ID      string            `json:"id" yaml:"id"`
	Name    string            `json:"name" yaml:"name"`
	Kind    string            `json:"kind" yaml:"kind"`
	Address string            `json:"address" yaml:"address"`
	Labels  map[string]string `json:"labels" yaml:"labels"`
	Data    map[string]any    `json:"data" yaml:"data"`
}

// ToStarlarkDict converts the resource into a Starlark dictionary representation.
func (r Resource) ToStarlarkDict() *starlark.Dict {
	d := starlark.NewDict(6 + len(r.Labels) + len(r.Data))

	d.SetKey(starlark.String("id"), starlark.String(r.ID))
	d.SetKey(starlark.String("name"), starlark.String(r.Name))
	d.SetKey(starlark.String("kind"), starlark.String(r.Kind))
	d.SetKey(starlark.String("address"), starlark.String(r.Address))

	// Labels sub-dict
	labelDict := starlark.NewDict(len(r.Labels))
	for k, v := range r.Labels {
		labelDict.SetKey(starlark.String(k), starlark.String(v))
		// Top-level accessibility for labels
		d.SetKey(starlark.String(k), starlark.String(v))
	}
	d.SetKey(starlark.String("labels"), labelDict)

	// Data sub-dict
	dataDict := starlark.NewDict(len(r.Data))
	for k, v := range r.Data {
		var starVal starlark.Value
		if err := startype.Go(v).Starlark(&starVal); err == nil {
			dataDict.SetKey(starlark.String(k), starVal)
			// Also allow top-level access if not colliding with core fields
			if _, found, _ := d.Get(starlark.String(k)); !found {
				d.SetKey(starlark.String(k), starVal)
			}
		}
	}
	d.SetKey(starlark.String("data"), dataDict)

	return d
}

// NormalizeResource converts arbitrary map or string data into a normalized Resource.
func NormalizeResource(raw any) (Resource, error) {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return Resource{}, fmt.Errorf("resource address cannot be empty")
		}
		return Resource{
			ID:      s,
			Name:    s,
			Kind:    "host",
			Address: s,
			Labels:  make(map[string]string),
			Data:    make(map[string]any),
		}, nil

	case map[string]any:
		return normalizeMap(v)

	case map[any]any:
		converted := make(map[string]any, len(v))
		for k, val := range v {
			converted[fmt.Sprintf("%v", k)] = val
		}
		return normalizeMap(converted)

	default:
		return Resource{}, fmt.Errorf("unsupported resource format: %T", raw)
	}
}

// NormalizeStarlarkValue converts a Starlark value into a normalized Resource.
func NormalizeStarlarkValue(val starlark.Value) (Resource, error) {
	switch v := val.(type) {
	case starlark.String:
		return NormalizeResource(string(v))

	case *starlark.Dict:
		raw := make(map[string]any, v.Len())
		for _, item := range v.Items() {
			k, ok := starlark.AsString(item[0])
			if !ok {
				continue
			}
			var goVal any
			if err := startype.Starlark(item[1]).Go(&goVal); err != nil {
				goVal = item[1].String()
			}
			raw[k] = goVal
		}
		return normalizeMap(raw)

	default:
		return Resource{}, fmt.Errorf("unsupported Starlark resource type: %s", val.Type())
	}
}

func normalizeMap(m map[string]any) (Resource, error) {
	res := Resource{
		Labels: make(map[string]string),
		Data:   make(map[string]any, len(m)),
	}

	// Copy all raw fields to Data
	maps.Copy(res.Data, m)

	// Address extraction (fallback: address -> host -> ip -> hostname -> name)
	if a, ok := getString(m, "address"); ok && a != "" {
		res.Address = a
	} else if a, ok := getString(m, "host"); ok && a != "" {
		res.Address = a
	} else if a, ok := getString(m, "ip"); ok && a != "" {
		res.Address = a
	} else if a, ok := getString(m, "hostname"); ok && a != "" {
		res.Address = a
	} else if a, ok := getString(m, "name"); ok && a != "" {
		res.Address = a
	}

	if res.Address == "" {
		return Resource{}, fmt.Errorf("resource requires at least an address, host, or name")
	}

	// Name extraction (default to Address)
	if n, ok := getString(m, "name"); ok && n != "" {
		res.Name = n
	} else {
		res.Name = res.Address
	}

	// ID extraction (default to Name)
	if id, ok := getString(m, "id"); ok && id != "" {
		res.ID = id
	} else {
		res.ID = res.Name
	}

	// Kind extraction (default to "host")
	if k, ok := getString(m, "kind"); ok && k != "" {
		res.Kind = k
	} else {
		res.Kind = "host"
	}

	// Process explicit labels map if provided
	if rawLabels, ok := m["labels"]; ok {
		if labelMap, ok := rawLabels.(map[string]any); ok {
			for k, v := range labelMap {
				res.Labels[k] = fmt.Sprintf("%v", v)
			}
		} else if labelMap, ok := rawLabels.(map[string]string); ok {
			maps.Copy(res.Labels, labelMap)
		}
	}

	// Index all top-level primitive values as searchable labels for convenience
	for k, v := range m {
		if k == "labels" || k == "data" {
			continue
		}
		switch val := v.(type) {
		case string:
			res.Labels[k] = val
		case int, int64, float64, bool:
			res.Labels[k] = fmt.Sprintf("%v", val)
		}
	}

	return res, nil
}

func getString(m map[string]any, key string) (string, bool) {
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s), true
		}
		return fmt.Sprintf("%v", val), true
	}
	return "", false
}
