package template

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Template is a Starlark value wrapping a parsed Go text/template.
type Template struct {
	source string
	tmpl   *template.Template
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*Template)(nil)
	_ starlark.HasAttrs = (*Template)(nil)
)

func (t *Template) String() string        { return fmt.Sprintf("template(%q)", truncate(t.source, 40)) }
func (t *Template) Type() string          { return "template" }
func (t *Template) Freeze()               {} // immutable
func (t *Template) Truth() starlark.Bool  { return starlark.Bool(t.source != "") }
func (t *Template) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: template") }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Attr implements starlark.HasAttrs.
func (t *Template) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := t.methodBuiltin(base); method != nil {
			return libkite.TryWrap("template."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	if name == "source" {
		return starlark.String(t.source), nil
	}

	// Methods
	if method := t.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (t *Template) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "render":
		return starlark.NewBuiltin("template.render", t.renderMethod)
	}
	return nil
}

// AttrNames implements starlark.HasAttrs.
func (t *Template) AttrNames() []string {
	names := []string{"source", "render", "try_render"}
	sort.Strings(names)
	return names
}

// renderMethod executes the template with the given data.
// Usage: t.render(data, missing_key="default")
func (t *Template) renderMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("template.render: takes exactly 1 argument (data)")
	}

	// Parse kwargs
	missingKey := ""
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "missing_key" {
			if s, ok := starlark.AsString(kv[1]); ok {
				missingKey = s
			}
		}
	}

	// Apply missing_key option
	tmpl := t.tmpl
	if missingKey != "" {
		switch missingKey {
		case "error", "zero", "default":
			// Clone the template so we don't mutate the original
			var err error
			tmpl, err = t.tmpl.Clone()
			if err != nil {
				return nil, fmt.Errorf("template.render: %w", err)
			}
			tmpl = tmpl.Option("missingkey=" + missingKey)
		default:
			return nil, fmt.Errorf("template.render: missing_key must be \"error\", \"zero\", or \"default\", got %q", missingKey)
		}
	}

	// Convert data to Go value
	var goData any
	if err := startype.Starlark(args[0]).Go(&goData); err != nil {
		return nil, fmt.Errorf("template.render: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, goData); err != nil {
		return nil, fmt.Errorf("template.render: %w", err)
	}
	return starlark.String(buf.String()), nil
}

// parseTemplate creates a Template from source text with optional delimiters.
func parseTemplate(source, leftDelim, rightDelim string, thread *starlark.Thread, config *libkite.ModuleConfig) (*Template, error) {
	t := template.New("tmpl")
	if leftDelim != "" && rightDelim != "" {
		t = t.Delims(leftDelim, rightDelim)
	}
	parsed, err := t.Parse(source)
	if err != nil {
		return nil, err
	}
	return &Template{
		source: source,
		tmpl:   parsed,
		thread: thread,
		config: config,
	}, nil
}

// extractDelims extracts the delims kwarg from kwargs.
// Returns left, right delimiter strings (empty if not specified).
func extractDelims(kwargs []starlark.Tuple) (string, string, error) {
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "delims" {
			tuple, ok := kv[1].(starlark.Tuple)
			if !ok || tuple.Len() != 2 {
				return "", "", fmt.Errorf("delims must be a tuple of (left, right), e.g. (\"<%%\", \"%%>\")")
			}
			left, ok1 := starlark.AsString(tuple[0])
			right, ok2 := starlark.AsString(tuple[1])
			if !ok1 || !ok2 {
				return "", "", fmt.Errorf("delims must be a tuple of two strings")
			}
			return left, right, nil
		}
	}
	return "", "", nil
}
