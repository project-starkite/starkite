package csv

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Writer is a Starlark value wrapping list data for CSV file writing.
type Writer struct {
	data   *starlark.List
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*Writer)(nil)
	_ starlark.HasAttrs = (*Writer)(nil)
)

func (w *Writer) String() string {
	return fmt.Sprintf("csv.writer(%d rows)", w.data.Len())
}
func (w *Writer) Type() string          { return "csv.writer" }
func (w *Writer) Freeze()               {}
func (w *Writer) Truth() starlark.Bool  { return starlark.Bool(w.data.Len() > 0) }
func (w *Writer) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: csv.writer") }

func (w *Writer) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := w.methodBuiltin(base); method != nil {
			return libkite.TryWrap("csv.writer."+name, method), nil
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
		return starlark.NewBuiltin("csv.writer.write_file", w.writeFileMethod)
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

// writeFileMethod writes CSV data to a file.
// Usage: csv.from(data).write_file(path, sep=",", headers=None)
func (w *Writer) writeFileMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
		Sep  string `name:"sep"`
	}
	p.Sep = ","

	// Extract headers kwarg manually since it's a *starlark.List
	var headersList *starlark.List
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "headers" {
			if hl, ok := kv[1].(*starlark.List); ok {
				headersList = hl
			}
		} else {
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(w.thread, "fs", "write", "write", p.Path); err != nil {
		return nil, err
	}

	if w.isDryRun() {
		return starlark.None, nil
	}

	if w.data.Len() == 0 {
		if err := os.WriteFile(p.Path, []byte(""), 0644); err != nil {
			return nil, fmt.Errorf("csv.writer.write_file: %w", err)
		}
		return starlark.None, nil
	}

	// Auto-detect: dict rows or list rows
	firstElem := w.data.Index(0)
	if _, ok := firstElem.(*starlark.Dict); ok {
		return w.writeDictRows(p.Path, p.Sep, headersList)
	}
	return w.writeListRows(p.Path, p.Sep)
}

func (w *Writer) writeListRows(path, sep string) (starlark.Value, error) {
	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}

	writer := csv.NewWriter(outFile)
	if len(sep) > 0 {
		writer.Comma = rune(sep[0])
	}

	iter := w.data.Iterate()
	defer iter.Done()
	var row starlark.Value
	var record []string
	for iter.Next(&row) {
		rowList, ok := row.(*starlark.List)
		if !ok {
			outFile.Close()
			os.Remove(path)
			return nil, fmt.Errorf("csv.writer.write_file: expected list of lists, got %s", row.Type())
		}
		n := rowList.Len()
		if cap(record) < n {
			record = make([]string, n)
		} else {
			record = record[:n]
		}
		for i := range n {
			record[i] = stringValue(rowList.Index(i))
		}
		if err := writer.Write(record); err != nil {
			outFile.Close()
			os.Remove(path)
			return nil, fmt.Errorf("csv.writer.write_file: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		outFile.Close()
		os.Remove(path)
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}
	return starlark.None, nil
}

func (w *Writer) writeDictRows(path, sep string, headersList *starlark.List) (starlark.Value, error) {
	// Determine headers
	var headers []string
	if headersList != nil {
		headers = make([]string, headersList.Len())
		for i := 0; i < headersList.Len(); i++ {
			headers[i] = stringValue(headersList.Index(i))
		}
	}

	// If no headers provided, extract from first dict
	if len(headers) == 0 {
		firstDict := w.data.Index(0).(*starlark.Dict)
		for _, item := range firstDict.Items() {
			headers = append(headers, stringValue(item[0]))
		}
	}

	headerKeys := make([]starlark.String, len(headers))
	for i, h := range headers {
		headerKeys[i] = starlark.String(h)
	}

	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}

	writer := csv.NewWriter(outFile)
	if len(sep) > 0 {
		writer.Comma = rune(sep[0])
	}

	// Write header row
	if err := writer.Write(headers); err != nil {
		outFile.Close()
		os.Remove(path)
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}

	// Write data rows
	iter := w.data.Iterate()
	defer iter.Done()
	var record starlark.Value
	row := make([]string, len(headerKeys))
	for iter.Next(&record) {
		dict, ok := record.(*starlark.Dict)
		if !ok {
			outFile.Close()
			os.Remove(path)
			return nil, fmt.Errorf("csv.writer.write_file: expected dict, got %s", record.Type())
		}
		for i, key := range headerKeys {
			if val, found, _ := dict.Get(key); found {
				row[i] = stringValue(val)
			} else {
				row[i] = ""
			}
		}
		if err := writer.Write(row); err != nil {
			outFile.Close()
			os.Remove(path)
			return nil, fmt.Errorf("csv.writer.write_file: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		outFile.Close()
		os.Remove(path)
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return nil, fmt.Errorf("csv.writer.write_file: %w", err)
	}
	return starlark.None, nil
}
