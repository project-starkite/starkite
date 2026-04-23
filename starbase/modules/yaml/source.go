package yaml

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	goyaml "gopkg.in/yaml.v3"

	"github.com/vladimirvivien/starkite/starbase"
)

// Writer is a Starlark value wrapping data for YAML file writing.
type Writer struct {
	data   starlark.Value
	thread *starlark.Thread
	config *starbase.ModuleConfig
}

var (
	_ starlark.Value    = (*Writer)(nil)
	_ starlark.HasAttrs = (*Writer)(nil)
)

func (w *Writer) String() string {
	return fmt.Sprintf("yaml.writer(%s)", w.data.Type())
}
func (w *Writer) Type() string          { return "yaml.writer" }
func (w *Writer) Freeze()               {}
func (w *Writer) Truth() starlark.Bool  { return starlark.Bool(w.data != nil && w.data != starlark.None) }
func (w *Writer) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: yaml.writer") }

func (w *Writer) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := w.methodBuiltin(base); method != nil {
			return starbase.TryWrap("yaml.writer."+name, method), nil
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
	case "write_file":
		return starlark.NewBuiltin("yaml.writer.write_file", w.writeFileMethod)
	}
	return nil
}

func (w *Writer) AttrNames() []string {
	names := []string{"data", "write_file", "try_write_file"}
	sort.Strings(names)
	return names
}

func (w *Writer) isDryRun() bool {
	return w.config != nil && w.config.DryRun
}

// writeFileMethod writes YAML data to a file.
// If data is a list, writes multi-doc with --- separators.
// Otherwise writes a single document.
// Usage: yaml.from(data).write_file(path)
func (w *Writer) writeFileMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yaml.writer.write_file: expected 1 argument (path), got %d", len(args))
	}
	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("yaml.writer.write_file: path must be a string, got %s", args[0].Type())
	}

	if err := starbase.Check(w.thread, "fs", "write", path); err != nil {
		return nil, err
	}

	if w.isDryRun() {
		return starlark.None, nil
	}

	var output []byte

	if list, ok := w.data.(*starlark.List); ok {
		// Multi-doc: iterate list items, marshal each with --- separator
		var buf bytes.Buffer
		for i := 0; i < list.Len(); i++ {
			if i > 0 {
				buf.WriteString("---\n")
			}
			var goVal any
			if err := startype.Starlark(list.Index(i)).Go(&goVal); err != nil {
				return nil, fmt.Errorf("yaml.writer.write_file[%d]: %w", i, err)
			}
			data, err := goyaml.Marshal(goVal)
			if err != nil {
				return nil, fmt.Errorf("yaml.writer.write_file[%d]: %w", i, err)
			}
			buf.Write(data)
		}
		output = buf.Bytes()
	} else {
		// Single doc
		var goVal any
		if err := startype.Starlark(w.data).Go(&goVal); err != nil {
			return nil, fmt.Errorf("yaml.writer.write_file: %w", err)
		}
		var err error
		output, err = goyaml.Marshal(goVal)
		if err != nil {
			return nil, fmt.Errorf("yaml.writer.write_file: %w", err)
		}
	}

	if err := os.WriteFile(path, output, 0644); err != nil {
		return nil, fmt.Errorf("yaml.writer.write_file: %w", err)
	}
	return starlark.None, nil
}
