// Package table provides table formatting functions for starkite.
package table

import (
	"fmt"
	"strings"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "table"

// Module implements table formatting functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "table provides table formatting: new, print"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"new":   starlark.NewBuiltin("table.new", m.newTable),
				"print": starlark.NewBuiltin("table.print", m.printTable),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// TableValue represents a table with headers and rows
type TableValue struct {
	headers []string
	rows    [][]string
	widths  []int
}

func (t *TableValue) String() string        { return fmt.Sprintf("<table %d cols, %d rows>", len(t.headers), len(t.rows)) }
func (t *TableValue) Type() string          { return "table" }
func (t *TableValue) Freeze()               {}
func (t *TableValue) Truth() starlark.Bool  { return starlark.Bool(len(t.rows) > 0) }
func (t *TableValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: table") }

func (t *TableValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "add_row":
		return starlark.NewBuiltin("table.add_row", t.addRow), nil
	case "render":
		return starlark.NewBuiltin("table.render", t.render), nil
	case "row_count":
		return starlark.MakeInt(len(t.rows)), nil
	default:
		return nil, nil
	}
}

func (t *TableValue) AttrNames() []string {
	return []string{"add_row", "render", "row_count"}
}

func (t *TableValue) addRow(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	row := make([]string, len(args))
	for i, arg := range args {
		s, ok := starlark.AsString(arg)
		if !ok {
			s = arg.String()
		}
		row[i] = s
		if len(s) > t.widths[i] {
			t.widths[i] = len(s)
		}
	}
	t.rows = append(t.rows, row)
	return starlark.None, nil
}

func (t *TableValue) render(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var sb strings.Builder

	// Header row
	for i, header := range t.headers {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(padRight(header, t.widths[i]))
	}
	sb.WriteString("\n")

	// Separator
	for i := range t.headers {
		if i > 0 {
			sb.WriteString("-+-")
		}
		sb.WriteString(strings.Repeat("-", t.widths[i]))
	}
	sb.WriteString("\n")

	// Data rows
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(padRight(cell, t.widths[i]))
		}
		sb.WriteString("\n")
	}

	return starlark.String(sb.String()), nil
}

// newTable creates a new table with headers.
// Usage: table.new(["col1", "col2", "col3"])
func (m *Module) newTable(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// newTable requires a *starlark.List which startype can't handle directly
	if len(args) != 1 {
		return nil, fmt.Errorf("new: expected 1 argument (headers), got %d", len(args))
	}
	headerList, ok := args[0].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("new: argument must be a list, got %s", args[0].Type())
	}

	headers := make([]string, headerList.Len())
	widths := make([]int, headerList.Len())
	for i := 0; i < headerList.Len(); i++ {
		s, ok := starlark.AsString(headerList.Index(i))
		if !ok {
			return nil, fmt.Errorf("headers must be strings")
		}
		headers[i] = s
		widths[i] = len(s)
	}

	return &TableValue{
		headers: headers,
		rows:    nil,
		widths:  widths,
	}, nil
}

// printTable prints a table to stdout.
// Usage: table.print(tbl)
func (m *Module) printTable(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// printTable requires a *TableValue which startype can't handle
	if len(args) != 1 {
		return nil, fmt.Errorf("print: expected 1 argument (table), got %d", len(args))
	}
	tbl, ok := args[0].(*TableValue)
	if !ok {
		return nil, fmt.Errorf("print: argument must be a table, got %s", args[0].Type())
	}

	rendered, err := tbl.render(thread, fn, nil, nil)
	if err != nil {
		return nil, err
	}

	fmt.Print(rendered.(starlark.String))
	return starlark.None, nil
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
