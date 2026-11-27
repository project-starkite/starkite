package fleet

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

const (
	maxFleetFileBytes     = 64 << 20
	maxDiscoveryCallDepth = 8
)

var fleetWrapperKeys = []string{"hosts", "nodes", "servers", "resources", "items"}

func readBoundedFile(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file %q exceeds maximum size of %d bytes", path, maxBytes)
	}
	return data, nil
}

// FromFile loads a Fleet from a YAML or JSON file.
func FromFile(path string) (*Fleet, error) {
	data, err := readBoundedFile(path, maxFleetFileBytes)
	if err != nil {
		return nil, fmt.Errorf("fleet.file: failed to read file %q: %w", path, err)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return New(nil), nil
	}

	if strings.EqualFold(filepath.Ext(path), ".json") {
		return FromJSON(data)
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if f, jsonErr := FromJSON(data); jsonErr == nil {
			return f, nil
		}
	}

	return FromYAML(data)
}

// FromHostsFile loads a Fleet from a standard POSIX hosts file (e.g. /etc/hosts).
// If path is empty, it defaults to "/etc/hosts".
func FromHostsFile(path string, includeLoopback bool) (*Fleet, error) {
	if path == "" {
		path = "/etc/hosts"
	}
	data, err := readBoundedFile(path, maxFleetFileBytes)
	if err != nil {
		return nil, fmt.Errorf("fleet.hosts_file: failed to read %q: %w", path, err)
	}
	return FromHosts(data, includeLoopback)
}

// FromHosts parses compute resources from standard POSIX hosts file content.
func FromHosts(data []byte, includeLoopback bool) (*Fleet, error) {
	var resources []Resource
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Strip comments
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ipStr := fields[0]
		canonicalName := fields[1]
		aliases := fields[2:]

		// Validate IP
		parsedIP := net.ParseIP(ipStr)
		if parsedIP == nil {
			continue
		}

		// Filter loopback addresses if includeLoopback is false
		if !includeLoopback {
			if parsedIP.IsLoopback() ||
				ipStr == "127.0.0.1" || ipStr == "::1" ||
				ipStr == "fe00::0" || ipStr == "ff00::0" ||
				ipStr == "ff02::1" || ipStr == "ff02::2" || ipStr == "ff02::3" ||
				canonicalName == "localhost" || canonicalName == "localhost6" ||
				canonicalName == "broadcasthost" {
				continue
			}
		}

		ipVersion := "v4"
		if parsedIP.To4() == nil {
			ipVersion = "v6"
		}

		labels := map[string]string{
			"hostname":   canonicalName,
			"ip_version": ipVersion,
		}
		for _, alias := range aliases {
			labels[alias] = "true"
		}

		aliasList := make([]any, len(aliases))
		for i, a := range aliases {
			aliasList[i] = a
		}

		dataMap := map[string]any{
			"address":    ipStr,
			"name":       canonicalName,
			"hostname":   canonicalName,
			"ip_version": ipVersion,
			"aliases":    aliasList,
		}

		resources = append(resources, Resource{
			ID:      canonicalName,
			Name:    canonicalName,
			Kind:    "host",
			Address: ipStr,
			Labels:  labels,
			Data:    dataMap,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("fleet.hosts_file: parse error: %w", err)
	}

	return New(resources), nil
}

// FromSource parses a Fleet from a Starlark runtime value (list, callable, string YAML/JSON, or dict).
func FromSource(thread *starlark.Thread, source starlark.Value) (*Fleet, error) {
	return fromSourceDepth(thread, source, 0)
}

func fromSourceDepth(thread *starlark.Thread, source starlark.Value, depth int) (*Fleet, error) {
	if source == nil || source == starlark.None {
		return New(nil), nil
	}

	switch v := source.(type) {
	case *Fleet:
		return v, nil

	case *starlark.List:
		return fromStarlarkSequence(v)

	case starlark.Tuple:
		return fromStarlarkSequence(v)

	case starlark.Callable:
		if depth >= maxDiscoveryCallDepth {
			return nil, fmt.Errorf("fleet.new: discovery callable exceeded maximum nesting depth (%d)", maxDiscoveryCallDepth)
		}
		res, err := starlark.Call(thread, v, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("fleet.new: discovery function failed: %w", err)
		}
		return fromSourceDepth(thread, res, depth+1)

	case starlark.String:
		data := []byte(string(v))
		trimmed := strings.TrimSpace(string(v))
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			if f, err := FromJSON(data); err == nil {
				return f, nil
			}
		}
		return FromYAML(data)

	case *starlark.Dict:
		return fromStarlarkDict(v)

	default:
		return nil, fmt.Errorf("fleet.new: unsupported source type %s (expected list, callable, dict, or JSON string)", source.Type())
	}
}

func attachGroup(r *Resource, groupName string) {
	if r.Labels == nil {
		r.Labels = make(map[string]string)
	}
	r.Labels["_group"] = groupName
	if r.Data == nil {
		r.Data = make(map[string]any)
	}
	r.Data["_group"] = groupName
}

func fromStarlarkDict(d *starlark.Dict) (*Fleet, error) {
	for _, wk := range fleetWrapperKeys {
		val, found, err := d.Get(starlark.String(wk))
		if err != nil {
			return nil, fmt.Errorf("fleet.new: lookup %q: %w", wk, err)
		}
		if found {
			seq, ok := val.(starlark.Sequence)
			if !ok || val.Type() == "string" || val.Type() == "bytes" {
				return nil, fmt.Errorf("fleet.new: wrapper key %q must map to a list or tuple, got %s", wk, val.Type())
			}
			return fromStarlarkSequence(seq)
		}
	}

	groupEntries := make(map[string]starlark.Value, d.Len())
	groupNames := make([]string, 0, d.Len())
	for _, item := range d.Items() {
		groupKey, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("fleet.new: group key must be string, got %s", item[0].Type())
		}
		groupNames = append(groupNames, groupKey)
		groupEntries[groupKey] = item[1]
	}
	slices.Sort(groupNames)

	var items []Resource
	for _, groupKey := range groupNames {
		rawVal := groupEntries[groupKey]
		switch val := rawVal.(type) {
		case *starlark.List, starlark.Tuple:
			seq := val.(starlark.Sequence)
			iter := seq.Iterate()
			var elem starlark.Value
			idx := 0
			for iter.Next(&elem) {
				r, err := NormalizeStarlarkValue(elem)
				if err != nil {
					iter.Done()
					return nil, fmt.Errorf("group %q item %d: %w", groupKey, idx, err)
				}
				attachGroup(&r, groupKey)
				items = append(items, r)
				idx++
			}
			iter.Done()
		default:
			r, err := NormalizeStarlarkValue(rawVal)
			if err != nil {
				return nil, fmt.Errorf("group %q: %w", groupKey, err)
			}
			attachGroup(&r, groupKey)
			items = append(items, r)
		}
	}
	return New(items), nil
}

// FromJSON parses a Fleet from JSON bytes.
func FromJSON(data []byte) (*Fleet, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("fleet.new: invalid JSON payload: %w", err)
	}
	return parseRawData(raw)
}

// FromYAML parses a Fleet from YAML bytes.
func FromYAML(data []byte) (*Fleet, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("fleet.new: invalid YAML payload: %w", err)
	}
	return parseRawData(raw)
}

func parseRawData(raw any) (*Fleet, error) {
	if raw == nil {
		return New(nil), nil
	}

	var resources []Resource

	switch v := raw.(type) {
	case []any:
		for i, item := range v {
			r, err := NormalizeResource(item)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			resources = append(resources, r)
		}
		return New(resources), nil

	case map[string]any:
		// Check for wrapper keys like "hosts", "nodes", "servers", "resources"
		for _, wrapperKey := range fleetWrapperKeys {
			if rawList, exists := v[wrapperKey]; exists {
				subList, ok := rawList.([]any)
				if !ok {
					return nil, fmt.Errorf("wrapper key %q must be a list, got %T", wrapperKey, rawList)
				}
				for i, item := range subList {
					r, err := NormalizeResource(item)
					if err != nil {
						return nil, fmt.Errorf("item %d in %q: %w", i, wrapperKey, err)
					}
					resources = append(resources, r)
				}
				return New(resources), nil
			}
		}

		// Grouped map: groupName -> []items or single resource
		groupNames := slices.Sorted(maps.Keys(v))
		for _, groupName := range groupNames {
			groupVal := v[groupName]
			switch g := groupVal.(type) {
			case []any:
				for i, item := range g {
					r, err := NormalizeResource(item)
					if err != nil {
						return nil, fmt.Errorf("group %q item %d: %w", groupName, i, err)
					}
					attachGroup(&r, groupName)
					resources = append(resources, r)
				}
			default:
				r, err := NormalizeResource(groupVal)
				if err != nil {
					return nil, fmt.Errorf("group %q: %w", groupName, err)
				}
				attachGroup(&r, groupName)
				resources = append(resources, r)
			}
		}
		return New(resources), nil

	default:
		return nil, fmt.Errorf("expected list or map of compute resources, got %T", raw)
	}
}

func fromStarlarkSequence(seq starlark.Sequence) (*Fleet, error) {
	resources := make([]Resource, 0, seq.Len())
	iter := seq.Iterate()
	defer iter.Done()

	var val starlark.Value
	idx := 0
	for iter.Next(&val) {
		r, err := NormalizeStarlarkValue(val)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", idx, err)
		}
		resources = append(resources, r)
		idx++
	}

	return New(resources), nil
}
