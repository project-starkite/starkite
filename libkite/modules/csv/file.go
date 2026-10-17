package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// CsvFile is a Starlark value representing a file path for CSV reading.
type CsvFile struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*CsvFile)(nil)
	_ starlark.HasAttrs = (*CsvFile)(nil)
)

func (f *CsvFile) String() string        { return fmt.Sprintf("csv.file(%q)", f.path) }
func (f *CsvFile) Type() string          { return "csv.file" }
func (f *CsvFile) Freeze()               {}
func (f *CsvFile) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *CsvFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: csv.file") }

func (f *CsvFile) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return libkite.TryWrap("csv.file."+name, method), nil
		}
		return nil, nil
	}

	if name == "path" {
		return starlark.String(f.path), nil
	}

	if method := f.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (f *CsvFile) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "read":
		return starlark.NewBuiltin("csv.file.read", f.readMethod)
	}
	return nil
}

func (f *CsvFile) AttrNames() []string {
	names := []string{"path", "read", "try_read"}
	sort.Strings(names)
	return names
}

func (f *CsvFile) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

// readMethod streams and parses the CSV file.
// Usage: csv.file(path).read(sep=",", comment="", skip=0, header=false)
func (f *CsvFile) readMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Sep     string `name:"sep"`
		Comment string `name:"comment"`
		Skip    int    `name:"skip"`
		Header  bool   `name:"header"`
	}
	p.Sep = ","
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(f.thread, "fs", "read", "read_file", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.NewList(nil), nil
	}

	file, err := os.Open(f.path)
	if err != nil {
		return nil, fmt.Errorf("csv.file.read: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	if len(p.Sep) > 0 {
		reader.Comma = rune(p.Sep[0])
	}
	if len(p.Comment) > 0 {
		reader.Comment = rune(p.Comment[0])
	}
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	// Skip rows if requested without allocating record slices or Starlark values.
	for i := 0; i < p.Skip; i++ {
		_, err := reader.Read()
		if err == io.EOF {
			return starlark.NewList(nil), nil
		}
		if err != nil {
			return nil, fmt.Errorf("csv.file.read: %w", err)
		}
	}

	if p.Header {
		return f.readStreamAsDict(reader)
	}
	return f.readStreamAsList(reader)
}

func (f *CsvFile) readStreamAsList(reader *csv.Reader) (starlark.Value, error) {
	var rows []starlark.Value
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv.file.read: %w", err)
		}
		cols := make([]starlark.Value, len(record))
		for j, field := range record {
			cols[j] = starlark.String(field)
		}
		rows = append(rows, starlark.NewList(cols))
	}
	return starlark.NewList(rows), nil
}

func (f *CsvFile) readStreamAsDict(reader *csv.Reader) (starlark.Value, error) {
	headerRecord, err := reader.Read()
	if err == io.EOF {
		return starlark.NewList(nil), nil
	}
	if err != nil {
		return nil, fmt.Errorf("csv.file.read: %w", err)
	}

	headerKeys := make([]starlark.String, len(headerRecord))
	for i, h := range headerRecord {
		headerKeys[i] = starlark.String(h)
	}

	var rows []starlark.Value
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv.file.read: %w", err)
		}
		dict := starlark.NewDict(len(headerKeys))
		for j, key := range headerKeys {
			value := ""
			if j < len(record) {
				value = record[j]
			}
			dict.SetKey(key, starlark.String(value))
		}
		rows = append(rows, dict)
	}
	return starlark.NewList(rows), nil
}
