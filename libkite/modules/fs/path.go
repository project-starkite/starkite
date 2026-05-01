package fs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite"
)

// Path is a Starlark value representing a filesystem path, inspired by Python's pathlib.Path.
// It implements starlark.Value, starlark.HasAttrs, and starlark.HasBinary.
type Path struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value     = (*Path)(nil)
	_ starlark.HasAttrs  = (*Path)(nil)
	_ starlark.HasBinary = (*Path)(nil)
)

func (p *Path) String() string        { return fmt.Sprintf("path(%q)", p.path) }
func (p *Path) Type() string          { return "fs.path" }
func (p *Path) Freeze()               {} // immutable
func (p *Path) Truth() starlark.Bool  { return starlark.Bool(p.path != "") }
func (p *Path) Hash() (uint32, error) { return starlark.String(p.path).Hash() }

func (p *Path) newPath(s string) *Path {
	return &Path{path: s, thread: p.thread, config: p.config}
}

// Binary implements starlark.HasBinary for the `/` operator.
func (p *Path) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	if op == syntax.SLASH && side == starlark.Left {
		if s, ok := starlark.AsString(y); ok {
			return p.newPath(filepath.Join(p.path, s)), nil
		}
		if other, ok := y.(*Path); ok {
			return p.newPath(filepath.Join(p.path, other.path)), nil
		}
	}
	return nil, nil
}

// Attr implements starlark.HasAttrs.
func (p *Path) Attr(name string) (starlark.Value, error) {
	// try_ prefix dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := p.methodBuiltin(base); method != nil {
			return libkite.TryWrap("fs.path."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	switch name {
	case "name":
		return starlark.String(filepath.Base(p.path)), nil
	case "parent":
		return p.newPath(filepath.Dir(p.path)), nil
	case "stem":
		return starlark.String(stem(p.path)), nil
	case "suffix":
		return starlark.String(filepath.Ext(p.path)), nil
	case "string":
		return starlark.String(p.path), nil
	case "parts":
		return pathParts(p.path), nil
	}

	// Methods
	if method := p.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

// AttrNames implements starlark.HasAttrs.
func (p *Path) AttrNames() []string {
	names := []string{
		// Properties
		"name", "parent", "stem", "suffix", "string", "parts",
	}
	methods := p.methodNames()
	names = append(names, methods...)
	// Add try_ variants for I/O methods
	for _, m := range methods {
		if isIOMethod(m) {
			names = append(names, "try_"+m)
		}
	}
	sort.Strings(names)
	return names
}

func (p *Path) methodNames() []string {
	return []string{
		// Path manipulation
		"join", "with_name", "with_suffix", "resolve", "clean",
		"is_absolute", "is_relative_to", "relative_to",
		"match", "expanduser",
		// Type checks & metadata
		"exists", "is_file", "is_dir", "is_symlink",
		"stat", "owner", "group",
		// Read
		"read_text", "read_bytes",
		// Write
		"write_text", "write_bytes", "append_text", "append_bytes",
		// File & directory ops
		"touch", "mkdir", "remove", "rename",
		"copy_to", "move_to", "truncate",
		"chmod", "chown", "symlink_to", "readlink", "hardlink_to",
		"listdir", "glob", "walk", "disk_usage",
	}
}

func isIOMethod(name string) bool {
	switch name {
	case "exists", "is_file", "is_dir", "is_symlink",
		"stat", "owner", "group",
		"read_text", "read_bytes",
		"write_text", "write_bytes", "append_text", "append_bytes",
		"touch", "mkdir", "remove", "rename",
		"copy_to", "move_to", "truncate",
		"chmod", "chown", "symlink_to", "readlink", "hardlink_to",
		"listdir", "glob", "walk", "disk_usage":
		return true
	}
	return false
}

func (p *Path) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	// Path manipulation (no I/O)
	case "join":
		return starlark.NewBuiltin("fs.path.join", p.joinMethod)
	case "with_name":
		return starlark.NewBuiltin("fs.path.with_name", p.withNameMethod)
	case "with_suffix":
		return starlark.NewBuiltin("fs.path.with_suffix", p.withSuffixMethod)
	case "resolve":
		return starlark.NewBuiltin("fs.path.resolve", p.resolveMethod)
	case "is_absolute":
		return starlark.NewBuiltin("fs.path.is_absolute", p.isAbsoluteMethod)
	case "is_relative_to":
		return starlark.NewBuiltin("fs.path.is_relative_to", p.isRelativeToMethod)
	case "relative_to":
		return starlark.NewBuiltin("fs.path.relative_to", p.relativeToMethod)
	case "match":
		return starlark.NewBuiltin("fs.path.match", p.matchMethod)
	case "expanduser":
		return starlark.NewBuiltin("fs.path.expanduser", p.expanduserMethod)
	case "clean":
		return starlark.NewBuiltin("fs.path.clean", p.cleanMethod)
	// Type checks & metadata
	case "exists":
		return starlark.NewBuiltin("fs.path.exists", p.existsMethod)
	case "is_file":
		return starlark.NewBuiltin("fs.path.is_file", p.isFileMethod)
	case "is_dir":
		return starlark.NewBuiltin("fs.path.is_dir", p.isDirMethod)
	case "is_symlink":
		return starlark.NewBuiltin("fs.path.is_symlink", p.isSymlinkMethod)
	case "stat":
		return starlark.NewBuiltin("fs.path.stat", p.statMethod)
	case "owner":
		return starlark.NewBuiltin("fs.path.owner", p.ownerMethod)
	case "group":
		return starlark.NewBuiltin("fs.path.group", p.groupMethod)
	// Read
	case "read_text":
		return starlark.NewBuiltin("fs.path.read_text", p.readTextMethod)
	case "read_bytes":
		return starlark.NewBuiltin("fs.path.read_bytes", p.readBytesMethod)
	// Write
	case "write_text":
		return starlark.NewBuiltin("fs.path.write_text", p.writeTextMethod)
	case "write_bytes":
		return starlark.NewBuiltin("fs.path.write_bytes", p.writeBytesMethod)
	case "append_text":
		return starlark.NewBuiltin("fs.path.append_text", p.appendTextMethod)
	case "append_bytes":
		return starlark.NewBuiltin("fs.path.append_bytes", p.appendBytesMethod)
	// File & directory ops
	case "touch":
		return starlark.NewBuiltin("fs.path.touch", p.touchMethod)
	case "mkdir":
		return starlark.NewBuiltin("fs.path.mkdir", p.mkdirMethod)
	case "remove":
		return starlark.NewBuiltin("fs.path.remove", p.removeMethod)
	case "rename":
		return starlark.NewBuiltin("fs.path.rename", p.renameMethod)
	case "chmod":
		return starlark.NewBuiltin("fs.path.chmod", p.chmodMethod)
	case "symlink_to":
		return starlark.NewBuiltin("fs.path.symlink_to", p.symlinkToMethod)
	case "copy_to":
		return starlark.NewBuiltin("fs.path.copy_to", p.copyToMethod)
	case "move_to":
		return starlark.NewBuiltin("fs.path.move_to", p.moveToMethod)
	case "truncate":
		return starlark.NewBuiltin("fs.path.truncate", p.truncateMethod)
	case "chown":
		return starlark.NewBuiltin("fs.path.chown", p.chownMethod)
	case "readlink":
		return starlark.NewBuiltin("fs.path.readlink", p.readlinkMethod)
	case "hardlink_to":
		return starlark.NewBuiltin("fs.path.hardlink_to", p.hardlinkToMethod)
	case "listdir":
		return starlark.NewBuiltin("fs.path.listdir", p.listdirMethod)
	case "glob":
		return starlark.NewBuiltin("fs.path.glob", p.globMethod)
	case "walk":
		return starlark.NewBuiltin("fs.path.walk", p.walkMethod)
	case "disk_usage":
		return starlark.NewBuiltin("fs.path.disk_usage", p.diskUsageMethod)
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// stem returns the filename without extension.
// For dotfiles like ".hidden", the whole name is the stem (no extension).
func stem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == "" || ext == base {
		return base
	}
	return base[:len(base)-len(ext)]
}

// pathParts splits a path into its components.
func pathParts(path string) *starlark.List {
	if path == "" {
		return starlark.NewList(nil)
	}
	// Clean path first
	cleaned := filepath.Clean(path)
	var parts []string
	if filepath.IsAbs(cleaned) {
		parts = append(parts, "/")
		cleaned = cleaned[1:]
	}
	if cleaned != "" && cleaned != "." {
		parts = append(parts, strings.Split(cleaned, string(filepath.Separator))...)
	}
	elems := make([]starlark.Value, len(parts))
	for i, part := range parts {
		elems[i] = starlark.String(part)
	}
	return starlark.NewList(elems)
}

func (p *Path) isDryRun() bool {
	return p.config != nil && p.config.DryRun
}

// ============================================================================
// Path manipulation methods (no I/O)
// ============================================================================

func (p *Path) joinMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, p.path)
	for _, arg := range args {
		s, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("fs.path.join: expected string arguments, got %s", arg.Type())
		}
		parts = append(parts, s)
	}
	return p.newPath(filepath.Join(parts...)), nil
}

func (p *Path) withNameMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Name string `name:"name" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	return p.newPath(filepath.Join(filepath.Dir(p.path), params.Name)), nil
}

func (p *Path) withSuffixMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Suffix string `name:"suffix" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	base := filepath.Base(p.path)
	ext := filepath.Ext(base)
	newBase := base[:len(base)-len(ext)] + params.Suffix
	return p.newPath(filepath.Join(filepath.Dir(p.path), newBase)), nil
}

func (p *Path) resolveMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	absPath, err := filepath.Abs(p.path)
	if err != nil {
		return nil, err
	}
	return p.newPath(filepath.Clean(absPath)), nil
}

func (p *Path) isAbsoluteMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.Bool(filepath.IsAbs(p.path)), nil
}

func (p *Path) isRelativeToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Other string `name:"other" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	_, err := filepath.Rel(params.Other, p.path)
	if err != nil {
		return starlark.False, nil
	}
	rel, _ := filepath.Rel(params.Other, p.path)
	return starlark.Bool(!strings.HasPrefix(rel, "..")), nil
}

func (p *Path) relativeToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Other string `name:"other" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(params.Other, p.path)
	if err != nil {
		return nil, fmt.Errorf("fs.path.relative_to: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("fs.path.relative_to: %q is not relative to %q", p.path, params.Other)
	}
	return p.newPath(rel), nil
}

func (p *Path) matchMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	matched, err := filepath.Match(params.Pattern, filepath.Base(p.path))
	if err != nil {
		return nil, err
	}
	return starlark.Bool(matched), nil
}

func (p *Path) expanduserMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if !strings.HasPrefix(p.path, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("fs.path.expanduser: %w", err)
	}
	if p.path == "~" {
		return p.newPath(home), nil
	}
	if strings.HasPrefix(p.path, "~/") {
		return p.newPath(filepath.Join(home, p.path[2:])), nil
	}
	// ~user form
	parts := strings.SplitN(p.path[1:], "/", 2)
	u, err := user.Lookup(parts[0])
	if err != nil {
		return nil, fmt.Errorf("fs.path.expanduser: %w", err)
	}
	if len(parts) == 1 {
		return p.newPath(u.HomeDir), nil
	}
	return p.newPath(filepath.Join(u.HomeDir, parts[1])), nil
}

// ============================================================================
// Type checks & metadata methods
// ============================================================================

func (p *Path) existsMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "exists", p.path); err != nil {
		return nil, err
	}
	_, err := os.Stat(p.path)
	return starlark.Bool(err == nil), nil
}

func (p *Path) isFileMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "is_file", p.path); err != nil {
		return nil, err
	}
	info, err := os.Stat(p.path)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(info.Mode().IsRegular()), nil
}

func (p *Path) isDirMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "is_dir", p.path); err != nil {
		return nil, err
	}
	info, err := os.Stat(p.path)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(info.IsDir()), nil
}

func (p *Path) isSymlinkMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "is_symlink", p.path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(p.path)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(info.Mode()&fs.ModeSymlink != 0), nil
}

func (p *Path) statMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "file_stat", p.path); err != nil {
		return nil, err
	}
	info, err := os.Stat(p.path)
	if err != nil {
		return nil, err
	}
	dict := starlark.NewDict(5)
	dict.SetKey(starlark.String("name"), starlark.String(info.Name()))
	dict.SetKey(starlark.String("size"), starlark.MakeInt64(info.Size()))
	dict.SetKey(starlark.String("mode"), starlark.String(info.Mode().String()))
	dict.SetKey(starlark.String("is_dir"), starlark.Bool(info.IsDir()))
	dict.SetKey(starlark.String("mod_time"), starlark.String(info.ModTime().Format(time.RFC3339)))
	return dict, nil
}

// ownerMethod and groupMethod are platform-specific:
// see path_owner_unix.go and path_owner_windows.go.

// ============================================================================
// Read methods
// ============================================================================

func (p *Path) readTextMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "read_file", p.path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	return starlark.String(data), nil
}

func (p *Path) readBytesMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "read_bytes", p.path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(data), nil
}

// ============================================================================
// Write methods
// ============================================================================

func (p *Path) writeTextMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Content string `name:"content" position:"0" required:"true"`
		Mode    int    `name:"mode"`
	}
	params.Mode = 0644
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "write", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.WriteFile(p.path, []byte(params.Content), fs.FileMode(params.Mode)); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) writeBytesMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Content starlark.Value `name:"content" position:"0" required:"true"`
		Mode    int            `name:"mode"`
	}
	params.Mode = 0644
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	var data []byte
	switch v := params.Content.(type) {
	case starlark.Bytes:
		data = []byte(string(v))
	case starlark.String:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("fs.path.write_bytes: content must be bytes or string, got %s", params.Content.Type())
	}
	if err := libkite.Check(p.thread, "fs", "write", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.WriteFile(p.path, data, fs.FileMode(params.Mode)); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) appendTextMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Content string `name:"content" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "write", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	f, err := os.OpenFile(p.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.WriteString(params.Content); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) appendBytesMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Content starlark.Value `name:"content" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	var data []byte
	switch v := params.Content.(type) {
	case starlark.Bytes:
		data = []byte(string(v))
	case starlark.String:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("fs.path.append_bytes: content must be bytes or string, got %s", params.Content.Type())
	}
	if err := libkite.Check(p.thread, "fs", "write", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	f, err := os.OpenFile(p.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

// ============================================================================
// File & directory operation methods
// ============================================================================

func (p *Path) touchMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		ExistOk bool `name:"exist_ok"`
	}
	params.ExistOk = true
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "touch", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	_, err := os.Stat(p.path)
	if os.IsNotExist(err) {
		f, err := os.Create(p.path)
		if err != nil {
			return nil, err
		}
		f.Close()
		return starlark.None, nil
	}
	if err != nil {
		return nil, err
	}
	if !params.ExistOk {
		return nil, fmt.Errorf("file already exists: %s", p.path)
	}
	now := time.Now()
	if err := os.Chtimes(p.path, now, now); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) mkdirMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Parents bool `name:"parents"`
		Mode    int  `name:"mode"`
	}
	params.Mode = 0755
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "mkdir", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	var err error
	if params.Parents {
		err = os.MkdirAll(p.path, fs.FileMode(params.Mode))
	} else {
		err = os.Mkdir(p.path, fs.FileMode(params.Mode))
	}
	if err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) removeMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "remove", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Remove(p.path); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) renameMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Target string `name:"target" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	target := params.Target
	if err := libkite.Check(p.thread, "fs", "rename", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return p.newPath(target), nil
	}
	if err := os.Rename(p.path, target); err != nil {
		return nil, err
	}
	return p.newPath(target), nil
}

func (p *Path) chmodMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Mode int `name:"mode" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "chmod", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Chmod(p.path, fs.FileMode(params.Mode)); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) symlinkToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Target string `name:"target" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "symlink", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Symlink(params.Target, p.path); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) listdirMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "listdir", p.path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p.path)
	if err != nil {
		return nil, err
	}
	elems := make([]starlark.Value, len(entries))
	for i, entry := range entries {
		elems[i] = p.newPath(filepath.Join(p.path, entry.Name()))
	}
	return starlark.NewList(elems), nil
}

func (p *Path) globMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "glob", p.path); err != nil {
		return nil, err
	}
	fullPattern := filepath.Join(p.path, params.Pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}
	elems := make([]starlark.Value, len(matches))
	for i, match := range matches {
		elems[i] = p.newPath(match)
	}
	return starlark.NewList(elems), nil
}

// ============================================================================
// New methods: copy, move, truncate, chown, readlink, hardlink, walk, disk_usage, clean
// ============================================================================

func (p *Path) cleanMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return p.newPath(filepath.Clean(p.path)), nil
}

func (p *Path) copyToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Target string `name:"target" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "copy", p.path); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "copy", params.Target); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return p.newPath(params.Target), nil
	}
	srcFile, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(params.Target)
	if err != nil {
		return nil, err
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return nil, err
	}
	return p.newPath(params.Target), nil
}

func (p *Path) moveToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Target string `name:"target" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "move", p.path); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "move", params.Target); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return p.newPath(params.Target), nil
	}
	if err := os.Rename(p.path, params.Target); err != nil {
		return nil, err
	}
	return p.newPath(params.Target), nil
}

func (p *Path) truncateMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Size int64 `name:"size" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "truncate", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Truncate(p.path, params.Size); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) chownMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Uid int `name:"uid"`
		Gid int `name:"gid"`
	}
	params.Uid = -1
	params.Gid = -1
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "chown", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Chown(p.path, params.Uid, params.Gid); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) readlinkMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "readlink", p.path); err != nil {
		return nil, err
	}
	target, err := os.Readlink(p.path)
	if err != nil {
		return nil, err
	}
	return p.newPath(target), nil
}

func (p *Path) hardlinkToMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		Target string `name:"target" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "hardlink", p.path); err != nil {
		return nil, err
	}
	if p.isDryRun() {
		return starlark.None, nil
	}
	if err := os.Link(params.Target, p.path); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (p *Path) walkMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "walk", p.path); err != nil {
		return nil, err
	}
	var results []starlark.Value
	err := filepath.WalkDir(p.path, func(walkPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(walkPath)
		if err != nil {
			return err
		}
		var dirs, files []starlark.Value
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, starlark.String(entry.Name()))
			} else {
				files = append(files, starlark.String(entry.Name()))
			}
		}
		tuple := starlark.Tuple{
			p.newPath(walkPath),
			starlark.NewList(dirs),
			starlark.NewList(files),
		}
		results = append(results, tuple)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return starlark.NewList(results), nil
}

func (p *Path) diskUsageMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "disk_usage", p.path); err != nil {
		return nil, err
	}
	return diskUsageInfo(p.path)
}
