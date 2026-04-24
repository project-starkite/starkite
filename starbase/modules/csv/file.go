package csv

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// CsvFile is a Starlark value representing a file path for CSV reading.
type CsvFile struct {
	path   string
	thread *starlark.Thread
	config *starbase.ModuleConfig
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
			return starbase.TryWrap("csv.file."+name, method), nil
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

// readMethod reads and parses the CSV file.
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

	if err := starbase.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.NewList(nil), nil
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("csv.file.read: %w", err)
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	if len(p.Sep) > 0 {
		reader.Comma = rune(p.Sep[0])
	}
	if len(p.Comment) > 0 {
		reader.Comment = rune(p.Comment[0])
	}
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv.file.read: %w", err)
	}

	// Skip rows if requested
	if p.Skip > 0 && p.Skip < len(records) {
		records = records[p.Skip:]
	} else if p.Skip >= len(records) {
		return starlark.NewList(nil), nil
	}

	if p.Header {
		return f.parseAsDict(records)
	}
	return f.parseAsList(records), nil
}

func (f *CsvFile) parseAsList(records [][]string) starlark.Value {
	rows := make([]starlark.Value, len(records))
	for i, record := range records {
		cols := make([]starlark.Value, len(record))
		for j, field := range record {
			cols[j] = starlark.String(field)
		}
		rows[i] = starlark.NewList(cols)
	}
	return starlark.NewList(rows)
}

func (f *CsvFile) parseAsDict(records [][]string) (starlark.Value, error) {
	if len(records) < 1 {
		return starlark.NewList(nil), nil
	}

	headers := records[0]
	dataRows := records[1:]

	rows := make([]starlark.Value, len(dataRows))
	for i, record := range dataRows {
		dict := starlark.NewDict(len(headers))
		for j, header := range headers {
			value := ""
			if j < len(record) {
				value = record[j]
			}
			dict.SetKey(starlark.String(header), starlark.String(value))
		}
		rows[i] = dict
	}
	return starlark.NewList(rows), nil
}
