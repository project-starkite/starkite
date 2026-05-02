package json

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Writer is a Starlark value wrapping data for JSON file writing.
type Writer struct {
	data   starlark.Value
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*Writer)(nil)
	_ starlark.HasAttrs = (*Writer)(nil)
)

func (w *Writer) String() string {
	return fmt.Sprintf("json.writer(%s)", w.data.Type())
}
func (w *Writer) Type() string { return "json.writer" }
func (w *Writer) Freeze()      {}
func (w *Writer) Truth() starlark.Bool {
	return starlark.Bool(w.data != nil && w.data != starlark.None)
}
func (w *Writer) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: json.writer") }

func (w *Writer) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := w.methodBuiltin(base); method != nil {
			return libkite.TryWrap("json.writer."+name, method), nil
		}
		return nil, nil
	}

	if name == "data" {
		return w.data, nil
	}

	if method := w.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (w *Writer) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "encode":
		return starlark.NewBuiltin("json.writer.encode", w.encodeMethod)
	case "write_file":
		return starlark.NewBuiltin("json.writer.write_file", w.writeFileMethod)
	}
	return nil
}

func (w *Writer) AttrNames() []string {
	names := []string{"data", "encode", "write_file", "try_encode", "try_write_file"}
	sort.Strings(names)
	return names
}

func (w *Writer) isDryRun() bool {
	return w.config != nil && w.config.DryRun
}

// encodeMethod encodes data to a JSON string in memory.
// Usage: json.source(data).encode(indent="", prefix="")
func (w *Writer) encodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var indent, prefix string
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "indent":
			if s, ok := starlark.AsString(kv[1]); ok {
				indent = s
			}
		case "prefix":
			if s, ok := starlark.AsString(kv[1]); ok {
				prefix = s
			}
		default:
			return nil, fmt.Errorf("json.writer.encode: unexpected keyword argument %q", key)
		}
	}

	var goVal any
	if err := startype.Starlark(w.data).Go(&goVal); err != nil {
		return nil, fmt.Errorf("json.writer.encode: %w", err)
	}

	goVal = convertForJSON(goVal)

	var output []byte
	var err error
	if indent != "" {
		output, err = json.MarshalIndent(goVal, prefix, indent)
	} else {
		output, err = json.Marshal(goVal)
	}
	if err != nil {
		return nil, fmt.Errorf("json.writer.encode: %w", err)
	}
	return starlark.String(output), nil
}

// writeFileMethod writes JSON data to a file.
// Usage: json.from(data).write_file(path, indent="", prefix="")
func (w *Writer) writeFileMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("json.writer.write_file: missing required argument: path")
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("json.writer.write_file: expected 1 positional argument (path), got %d", len(args))
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("json.writer.write_file: path must be a string, got %s", args[0].Type())
	}

	var indent, prefix string
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "indent":
			if s, ok := starlark.AsString(kv[1]); ok {
				indent = s
			}
		case "prefix":
			if s, ok := starlark.AsString(kv[1]); ok {
				prefix = s
			}
		default:
			return nil, fmt.Errorf("json.writer.write_file: unexpected keyword argument %q", key)
		}
	}

	if err := libkite.Check(w.thread, "fs", "write", "write", path); err != nil {
		return nil, err
	}

	if w.isDryRun() {
		return starlark.None, nil
	}

	var goVal any
	if err := startype.Starlark(w.data).Go(&goVal); err != nil {
		return nil, fmt.Errorf("json.writer.write_file: %w", err)
	}

	// Convert map[interface{}]interface{} to map[string]interface{} for JSON
	goVal = convertForJSON(goVal)

	var output []byte
	var err error
	if indent != "" {
		output, err = json.MarshalIndent(goVal, prefix, indent)
	} else {
		output, err = json.Marshal(goVal)
	}
	if err != nil {
		return nil, fmt.Errorf("json.writer.write_file: %w", err)
	}

	// Add trailing newline for pretty-printed output
	if indent != "" {
		output = append(output, '\n')
	}

	if err := os.WriteFile(path, output, 0644); err != nil {
		return nil, fmt.Errorf("json.writer.write_file: %w", err)
	}
	return starlark.None, nil
}

// convertForJSON recursively converts map[interface{}]interface{} to map[string]interface{}
// since encoding/json doesn't support the former.
func convertForJSON(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = convertForJSON(v)
		}
		return m
	case map[string]interface{}:
		for k, v := range val {
			val[k] = convertForJSON(v)
		}
		return val
	case []interface{}:
		for i, v := range val {
			val[i] = convertForJSON(v)
		}
		return val
	default:
		return v
	}
}
