package zip

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// Archive is a Starlark value representing a zip archive path.
// Methods open the archive, perform the operation, and close it.
type Archive struct {
	path   string
	thread *starlark.Thread
	config *starbase.ModuleConfig
}

var (
	_ starlark.Value    = (*Archive)(nil)
	_ starlark.HasAttrs = (*Archive)(nil)
)

func (a *Archive) String() string        { return fmt.Sprintf("zip.file(%q)", a.path) }
func (a *Archive) Type() string          { return "zip.archive" }
func (a *Archive) Freeze()               {} // immutable
func (a *Archive) Truth() starlark.Bool  { return starlark.Bool(a.path != "") }
func (a *Archive) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: zip.archive") }

func (a *Archive) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := a.methodBuiltin(base); method != nil {
			return starbase.TryWrap("zip.archive."+name, method), nil
		}
		return nil, nil
	}

	if method := a.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (a *Archive) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "namelist":
		return starlark.NewBuiltin("zip.archive.namelist", a.namelistMethod)
	case "read":
		return starlark.NewBuiltin("zip.archive.read", a.readMethod)
	case "read_all":
		return starlark.NewBuiltin("zip.archive.read_all", a.readAllMethod)
	case "write":
		return starlark.NewBuiltin("zip.archive.write", a.writeMethod)
	case "write_all":
		return starlark.NewBuiltin("zip.archive.write_all", a.writeAllMethod)
	}
	return nil
}

func (a *Archive) AttrNames() []string {
	names := []string{
		"namelist", "read", "read_all", "write", "write_all",
		"try_namelist", "try_read", "try_read_all", "try_write", "try_write_all",
	}
	sort.Strings(names)
	return names
}

func (a *Archive) isDryRun() bool {
	return a.config != nil && a.config.DryRun
}

// namelistMethod returns the list of entry names in the archive.
// Usage: zip.file("archive.zip").namelist(match="")
func (a *Archive) namelistMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Match string `name:"match"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(a.thread, "fs", "read_file", a.path); err != nil {
		return nil, err
	}

	reader, err := zip.OpenReader(a.path)
	if err != nil {
		return nil, fmt.Errorf("zip.namelist: %w", err)
	}
	defer reader.Close()

	names := make([]starlark.Value, 0, len(reader.File))
	for _, f := range reader.File {
		if p.Match != "" {
			matched, err := filepath.Match(p.Match, f.Name)
			if err != nil {
				return nil, fmt.Errorf("zip.namelist: invalid match pattern: %w", err)
			}
			if !matched {
				// Also try matching just the base name
				matched, _ = filepath.Match(p.Match, filepath.Base(f.Name))
				if !matched {
					continue
				}
			}
		}
		names = append(names, starlark.String(f.Name))
	}
	return starlark.NewList(names), nil
}

// readMethod reads a single entry from the archive.
// Usage: zip.file("archive.zip").read("entry.txt")
func (a *Archive) readMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name string `name:"name" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(a.thread, "fs", "read_file", a.path); err != nil {
		return nil, err
	}

	reader, err := zip.OpenReader(a.path)
	if err != nil {
		return nil, fmt.Errorf("zip.read: %w", err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		if f.Name == p.Name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("zip.read: %w", err)
			}
			defer rc.Close()
			content, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("zip.read: %w", err)
			}
			return starlark.Bytes(content), nil
		}
	}
	return nil, fmt.Errorf("zip.read: file %q not found in archive", p.Name)
}

// readAllMethod reads all entries from the archive.
// Usage: zip.file("archive.zip").read_all(match="", files=[])
func (a *Archive) readAllMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Match string   `name:"match"`
		Files []string `name:"files"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Match != "" && len(p.Files) > 0 {
		return nil, fmt.Errorf("zip.read_all: match and files are mutually exclusive")
	}

	if err := starbase.Check(a.thread, "fs", "read_file", a.path); err != nil {
		return nil, err
	}

	reader, err := zip.OpenReader(a.path)
	if err != nil {
		return nil, fmt.Errorf("zip.read_all: %w", err)
	}
	defer reader.Close()

	// Build file filter set if files kwarg given
	var fileSet map[string]bool
	if len(p.Files) > 0 {
		fileSet = make(map[string]bool, len(p.Files))
		for _, s := range p.Files {
			fileSet[s] = true
		}
	}

	dict := starlark.NewDict(len(reader.File))
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// Apply filter
		if p.Match != "" {
			matched, err := filepath.Match(p.Match, f.Name)
			if err != nil {
				return nil, fmt.Errorf("zip.read_all: invalid match pattern: %w", err)
			}
			if !matched {
				matched, _ = filepath.Match(p.Match, filepath.Base(f.Name))
				if !matched {
					continue
				}
			}
		}
		if fileSet != nil && !fileSet[f.Name] {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("zip.read_all: %w", err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("zip.read_all: %w", err)
		}
		dict.SetKey(starlark.String(f.Name), starlark.Bytes(content))
	}
	return dict, nil
}

// writeMethod writes a single file into a new zip archive.
// Usage: zip.file("output.zip").write("/path/to/file.txt", name="custom.txt")
func (a *Archive) writeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Source string `name:"source" position:"0" required:"true"`
		Name   string `name:"name"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	sourcePath := p.Source
	entryName := p.Name
	if entryName == "" {
		entryName = filepath.Base(sourcePath)
	}

	if err := starbase.Check(a.thread, "fs", "write", a.path); err != nil {
		return nil, err
	}
	if err := starbase.Check(a.thread, "fs", "read_file", sourcePath); err != nil {
		return nil, err
	}

	if a.isDryRun() {
		return starlark.None, nil
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("zip.write: %w", err)
	}

	outFile, err := os.Create(a.path)
	if err != nil {
		return nil, fmt.Errorf("zip.write: %w", err)
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	entry, err := w.Create(entryName)
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("zip.write: %w", err)
	}
	if _, err := entry.Write(content); err != nil {
		w.Close()
		return nil, fmt.Errorf("zip.write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("zip.write: %w", err)
	}
	return starlark.None, nil
}

// writeAllMethod writes multiple files into a new zip archive.
// Usage: zip.file("output.zip").write_all(match="src/**/*.go", files=[], base_dir="", level=-1)
func (a *Archive) writeAllMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Match   string   `name:"match"`
		Files   []string `name:"files"`
		BaseDir string   `name:"base_dir"`
		Level   int      `name:"level"`
	}
	p.Level = flate.DefaultCompression
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Match != "" && len(p.Files) > 0 {
		return nil, fmt.Errorf("zip.write_all: match and files are mutually exclusive")
	}
	if p.Match == "" && len(p.Files) == 0 {
		return nil, fmt.Errorf("zip.write_all: must specify match or files")
	}

	if err := starbase.Check(a.thread, "fs", "write", a.path); err != nil {
		return nil, err
	}

	// Resolve source files
	var sourcePaths []string
	if p.Match != "" {
		resolved, err := resolveGlob(p.Match)
		if err != nil {
			return nil, fmt.Errorf("zip.write_all: %w", err)
		}
		sourcePaths = resolved
	} else {
		sourcePaths = p.Files
	}

	// Check read permissions for all source files
	for _, sp := range sourcePaths {
		if err := starbase.Check(a.thread, "fs", "read_file", sp); err != nil {
			return nil, err
		}
	}

	if a.isDryRun() {
		return starlark.None, nil
	}

	outFile, err := os.Create(a.path)
	if err != nil {
		return nil, fmt.Errorf("zip.write_all: %w", err)
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	if p.Level != flate.DefaultCompression {
		w.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(out, p.Level)
		})
	}

	for _, sp := range sourcePaths {
		content, err := os.ReadFile(sp)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("zip.write_all: %w", err)
		}

		entryName := sp
		if p.BaseDir != "" {
			rel, err := filepath.Rel(p.BaseDir, sp)
			if err == nil {
				entryName = rel
			}
		}

		entry, err := w.Create(entryName)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("zip.write_all: %w", err)
		}
		if _, err := entry.Write(content); err != nil {
			w.Close()
			return nil, fmt.Errorf("zip.write_all: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("zip.write_all: %w", err)
	}
	return starlark.None, nil
}

// resolveGlob expands a glob pattern into matching file paths.
func resolveGlob(pattern string) ([]string, error) {
	if strings.Contains(pattern, "**") {
		// Walk-based resolution for ** patterns
		parts := strings.SplitN(pattern, "**", 2)
		root := parts[0]
		if root == "" {
			root = "."
		}
		root = strings.TrimRight(root, string(filepath.Separator))
		suffix := parts[1]
		if strings.HasPrefix(suffix, string(filepath.Separator)) {
			suffix = suffix[1:]
		}

		var matches []string
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if d.IsDir() {
				return nil
			}
			if suffix == "" {
				matches = append(matches, path)
				return nil
			}
			matched, _ := filepath.Match(suffix, filepath.Base(path))
			if matched {
				matches = append(matches, path)
			}
			return nil
		})
		return matches, err
	}
	return filepath.Glob(pattern)
}
