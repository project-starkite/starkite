package args

import (
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// ArgsResult represents the immutable struct of parsed command-line flags and positional arguments.
// It supports attribute access (flags.action), dictionary indexing (flags["action"]), and the .get() method.
type ArgsResult struct {
	data map[string]starlark.Value
}

// NewArgsResult creates a new ArgsResult with the provided data map.
func NewArgsResult(data map[string]starlark.Value) *ArgsResult {
	return &ArgsResult{data: data}
}

// String returns a human-readable representation of the parsed arguments.
func (r *ArgsResult) String() string {
	names := make([]string, 0, len(r.data))
	for k := range r.data {
		names = append(names, k)
	}
	sort.Strings(names)

	var parts []string
	for _, k := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", k, r.data[k]))
	}
	return fmt.Sprintf("args(%s)", strings.Join(parts, ", "))
}

// Type returns the Starlark type name.
func (r *ArgsResult) Type() string {
	return "args"
}

// Freeze freezes all child values in the result.
func (r *ArgsResult) Freeze() {
	for _, v := range r.data {
		v.Freeze()
	}
}

// Truth returns true since parsed arguments are always considered truthy.
func (r *ArgsResult) Truth() starlark.Bool {
	return starlark.True
}

// Hash returns an error as args is unhashable.
func (r *ArgsResult) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: args")
}

// Attr retrieves an attribute by name for dot notation (flags.key_name).
func (r *ArgsResult) Attr(name string) (starlark.Value, error) {
	if val, ok := r.data[name]; ok {
		return val, nil
	}
	if name == "get" {
		return starlark.NewBuiltin("get", r.getBuiltin), nil
	}
	return nil, nil // Return nil, nil to trigger Starlark's NoSuchAttrError
}

// AttrNames returns all available attribute names on the struct.
func (r *ArgsResult) AttrNames() []string {
	names := make([]string, 0, len(r.data)+1)
	for k := range r.data {
		names = append(names, k)
	}
	sort.Strings(names)
	if _, hasGet := r.data["get"]; !hasGet {
		names = append(names, "get")
	}
	return names
}

// Get implements starlark.Mapping for dictionary-style indexing (flags["key_name"]).
func (r *ArgsResult) Get(key starlark.Value) (starlark.Value, bool, error) {
	strKey, ok := starlark.AsString(key)
	if !ok {
		return nil, false, nil
	}
	normKey := libkite.NormalizeAttributeName(strKey)
	if val, ok := r.data[normKey]; ok {
		return val, true, nil
	}
	if val, ok := r.data[strKey]; ok {
		return val, true, nil
	}
	return nil, false, nil
}

// getBuiltin implements flags.get(key, default=None) for safe fallback querying.
func (r *ArgsResult) getBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		key        string
		defaultVal starlark.Value = starlark.None
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "key", &key, "default?", &defaultVal); err != nil {
		return nil, err
	}

	normKey := libkite.NormalizeAttributeName(key)
	if val, ok := r.data[normKey]; ok {
		return val, nil
	}
	if val, ok := r.data[key]; ok {
		return val, nil
	}
	return defaultVal, nil
}
