package fs

import (
	"os"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/vladimirvivien/starkite/starbase"
)

func newTestPath(path string) *Path {
	return &Path{path: path, thread: &starlark.Thread{Name: "test"}, config: &starbase.ModuleConfig{}}
}

// ============================================================================
// Value interface tests
// ============================================================================

func TestPathString(t *testing.T) {
	p := newTestPath("/etc/hostname")
	if got := p.String(); got != `path("/etc/hostname")` {
		t.Errorf("String() = %q, want %q", got, `path("/etc/hostname")`)
	}
}

func TestPathType(t *testing.T) {
	p := newTestPath("/tmp")
	if got := p.Type(); got != "fs.path" {
		t.Errorf("Type() = %q, want %q", got, "fs.path")
	}
}

func TestPathTruth(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/etc", true},
		{"", false},
	}
	for _, tt := range tests {
		p := newTestPath(tt.path)
		if got := bool(p.Truth()); got != tt.want {
			t.Errorf("Truth(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPathHash(t *testing.T) {
	p1 := newTestPath("/etc/hostname")
	p2 := newTestPath("/etc/hostname")
	h1, err := p1.Hash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := p2.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("same paths should have same hash: %d != %d", h1, h2)
	}
}

// ============================================================================
// Properties tests
// ============================================================================

func TestPathProperties(t *testing.T) {
	p := newTestPath("/var/log/syslog.txt")

	tests := []struct {
		attr string
		want string
	}{
		{"name", "syslog.txt"},
		{"stem", "syslog"},
		{"suffix", ".txt"},
		{"string", "/var/log/syslog.txt"},
	}
	for _, tt := range tests {
		v, err := p.Attr(tt.attr)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", tt.attr, err)
			continue
		}
		s, ok := starlark.AsString(v)
		if !ok {
			t.Errorf("Attr(%q) not a string: %s", tt.attr, v.Type())
			continue
		}
		if s != tt.want {
			t.Errorf("Attr(%q) = %q, want %q", tt.attr, s, tt.want)
		}
	}
}

func TestPathParent(t *testing.T) {
	p := newTestPath("/var/log/syslog")
	v, err := p.Attr("parent")
	if err != nil {
		t.Fatal(err)
	}
	parent, ok := v.(*Path)
	if !ok {
		t.Fatalf("parent is not a Path: %T", v)
	}
	if parent.path != "/var/log" {
		t.Errorf("parent.path = %q, want %q", parent.path, "/var/log")
	}
}

func TestPathParts(t *testing.T) {
	p := newTestPath("/var/log/syslog")
	v, err := p.Attr("parts")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.(*starlark.List)
	if !ok {
		t.Fatalf("parts is not a list: %T", v)
	}
	expected := []string{"/", "var", "log", "syslog"}
	if list.Len() != len(expected) {
		t.Fatalf("parts len = %d, want %d", list.Len(), len(expected))
	}
	for i, want := range expected {
		got, _ := starlark.AsString(list.Index(i))
		if got != want {
			t.Errorf("parts[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestPathStemNoExt(t *testing.T) {
	p := newTestPath("/etc/hostname")
	v, _ := p.Attr("stem")
	s, _ := starlark.AsString(v)
	if s != "hostname" {
		t.Errorf("stem = %q, want %q", s, "hostname")
	}
}

// ============================================================================
// Path manipulation method tests
// ============================================================================

func TestPathJoin(t *testing.T) {
	p := newTestPath("/etc")
	v, err := callMethod(p, "join", starlark.String("nginx"), starlark.String("nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "/etc/nginx/nginx.conf" {
		t.Errorf("join = %q, want %q", result.path, "/etc/nginx/nginx.conf")
	}
}

func TestPathWithName(t *testing.T) {
	p := newTestPath("/var/log/syslog")
	v, err := callMethod(p, "with_name", starlark.String("messages"))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "/var/log/messages" {
		t.Errorf("with_name = %q, want %q", result.path, "/var/log/messages")
	}
}

func TestPathWithSuffix(t *testing.T) {
	p := newTestPath("/var/log/app.log")
	v, err := callMethod(p, "with_suffix", starlark.String(".txt"))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "/var/log/app.txt" {
		t.Errorf("with_suffix = %q, want %q", result.path, "/var/log/app.txt")
	}
}

func TestPathResolve(t *testing.T) {
	p := newTestPath(".")
	v, err := callMethod(p, "resolve")
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if !filepath.IsAbs(result.path) {
		t.Errorf("resolve should return absolute path, got %q", result.path)
	}
}

func TestPathIsAbsolute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/etc", true},
		{"etc", false},
		{".", false},
	}
	for _, tt := range tests {
		p := newTestPath(tt.path)
		v, err := callMethod(p, "is_absolute")
		if err != nil {
			t.Fatal(err)
		}
		if bool(v.(starlark.Bool)) != tt.want {
			t.Errorf("is_absolute(%q) = %v, want %v", tt.path, v, tt.want)
		}
	}
}

func TestPathIsRelativeTo(t *testing.T) {
	p := newTestPath("/var/log/syslog")
	v, err := callMethod(p, "is_relative_to", starlark.String("/var"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.True {
		t.Error("is_relative_to('/var') should be True")
	}

	v, err = callMethod(p, "is_relative_to", starlark.String("/etc"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.False {
		t.Error("is_relative_to('/etc') should be False")
	}
}

func TestPathRelativeTo(t *testing.T) {
	p := newTestPath("/var/log/syslog")
	v, err := callMethod(p, "relative_to", starlark.String("/var"))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "log/syslog" {
		t.Errorf("relative_to = %q, want %q", result.path, "log/syslog")
	}
}

func TestPathRelativeToError(t *testing.T) {
	p := newTestPath("/etc/hostname")
	_, err := callMethod(p, "relative_to", starlark.String("/var"))
	if err == nil {
		t.Error("relative_to should fail when path is not relative")
	}
}

func TestPathMatch(t *testing.T) {
	p := newTestPath("/var/log/app.log")
	v, err := callMethod(p, "match", starlark.String("*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.True {
		t.Error("match('*.log') should be True")
	}

	v, err = callMethod(p, "match", starlark.String("*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.False {
		t.Error("match('*.txt') should be False")
	}
}

func TestPathExpanduser(t *testing.T) {
	p := newTestPath("~/Documents")
	v, err := callMethod(p, "expanduser")
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "Documents")
	if result.path != expected {
		t.Errorf("expanduser = %q, want %q", result.path, expected)
	}
}

func TestPathExpanduserNoTilde(t *testing.T) {
	p := newTestPath("/etc/hostname")
	v, err := callMethod(p, "expanduser")
	if err != nil {
		t.Fatal(err)
	}
	// Should return same path object
	if v.(*Path).path != "/etc/hostname" {
		t.Error("expanduser should not modify non-tilde path")
	}
}

// ============================================================================
// I/O method tests
// ============================================================================

func TestPathExists(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "exists")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.True {
		t.Error("/etc/passwd should exist")
	}

	p = newTestPath("/nonexistent/path/12345")
	v, err = callMethod(p, "exists")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.False {
		t.Error("nonexistent path should not exist")
	}
}

func TestPathIsFile(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "is_file")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.True {
		t.Error("/etc/passwd should be a file")
	}

	p = newTestPath("/tmp")
	v, err = callMethod(p, "is_file")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.False {
		t.Error("/tmp should not be a file")
	}
}

func TestPathIsDir(t *testing.T) {
	p := newTestPath("/tmp")
	v, err := callMethod(p, "is_dir")
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.True {
		t.Error("/tmp should be a directory")
	}
}

func TestPathIsSymlink(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "is_symlink")
	if err != nil {
		t.Fatal(err)
	}
	if v.(starlark.Bool) != starlark.False {
		t.Error("/etc/passwd should not be a symlink")
	}
}

func TestPathStat(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "stat")
	if err != nil {
		t.Fatal(err)
	}
	dict, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("stat should return dict, got %s", v.Type())
	}
	for _, key := range []string{"name", "size", "mode", "is_dir", "mod_time"} {
		if val, found, _ := dict.Get(starlark.String(key)); !found || val == nil {
			t.Errorf("stat should have key %q", key)
		}
	}
}

func TestPathOwner(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "owner")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := starlark.AsString(v)
	if !ok {
		t.Fatalf("owner should return string, got %s", v.Type())
	}
	if s == "" {
		t.Error("owner should not be empty for /etc/passwd")
	}
}

func TestPathGroup(t *testing.T) {
	p := newTestPath("/etc/passwd")
	v, err := callMethod(p, "group")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := starlark.AsString(v)
	if !ok {
		t.Fatalf("group should return string, got %s", v.Type())
	}
	if s == "" {
		t.Error("group should not be empty for /etc/passwd")
	}
}

func TestPathReadWriteText(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.txt")
	p := newTestPath(tmp)

	_, err := callMethod(p, "write_text", starlark.String("hello world"))
	if err != nil {
		t.Fatal(err)
	}

	v, err := callMethod(p, "read_text")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := starlark.AsString(v)
	if s != "hello world" {
		t.Errorf("read_text = %q, want %q", s, "hello world")
	}
}

func TestPathReadWriteBytes(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.bin")
	p := newTestPath(tmp)

	data := starlark.Bytes("\x00\x01\x02\xff")
	_, err := callMethod(p, "write_bytes", data)
	if err != nil {
		t.Fatal(err)
	}

	v, err := callMethod(p, "read_bytes")
	if err != nil {
		t.Fatal(err)
	}
	if v.(starlark.Bytes) != data {
		t.Error("read_bytes should return same data")
	}
}

func TestPathAppendText(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "append.txt")
	p := newTestPath(tmp)

	callMethod(p, "write_text", starlark.String("hello"))
	callMethod(p, "append_text", starlark.String(" world"))

	v, _ := callMethod(p, "read_text")
	s, _ := starlark.AsString(v)
	if s != "hello world" {
		t.Errorf("after append, content = %q, want %q", s, "hello world")
	}
}

func TestPathTouch(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "touched.txt")
	p := newTestPath(tmp)

	_, err := callMethod(p, "touch")
	if err != nil {
		t.Fatal(err)
	}

	v, _ := callMethod(p, "exists")
	if v != starlark.True {
		t.Error("touch should create file")
	}
}

func TestPathMkdir(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "newdir")
	p := newTestPath(tmp)

	_, err := callMethod(p, "mkdir")
	if err != nil {
		t.Fatal(err)
	}

	v, _ := callMethod(p, "is_dir")
	if v != starlark.True {
		t.Error("mkdir should create directory")
	}
}

func TestPathMkdirParents(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "a", "b", "c")
	p := newTestPath(tmp)

	_, err := callMethodKw(p, "mkdir", nil, kw("parents", starlark.True))
	if err != nil {
		t.Fatal(err)
	}

	v, _ := callMethod(p, "is_dir")
	if v != starlark.True {
		t.Error("mkdir(parents=True) should create nested dirs")
	}
}

func TestPathRemove(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "to_remove.txt")
	p := newTestPath(tmp)
	os.WriteFile(tmp, []byte("x"), 0644)

	_, err := callMethod(p, "remove")
	if err != nil {
		t.Fatal(err)
	}

	v, _ := callMethod(p, "exists")
	if v != starlark.False {
		t.Error("remove should delete file")
	}
}

func TestPathRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	os.WriteFile(src, []byte("data"), 0644)

	p := newTestPath(src)
	v, err := callMethod(p, "rename", starlark.String(dst))
	if err != nil {
		t.Fatal(err)
	}

	result := v.(*Path)
	if result.path != dst {
		t.Errorf("rename returned path %q, want %q", result.path, dst)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Error("renamed file should exist at new path")
	}
}

func TestPathChmod(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chmod_test.txt")
	os.WriteFile(tmp, []byte("x"), 0644)

	p := newTestPath(tmp)
	_, err := callMethod(p, "chmod", starlark.MakeInt(0755))
	if err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(tmp)
	if info.Mode().Perm() != 0755 {
		t.Errorf("chmod: mode = %o, want %o", info.Mode().Perm(), 0755)
	}
}

func TestPathSymlinkTo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("data"), 0644)

	p := newTestPath(link)
	_, err := callMethod(p, "symlink_to", starlark.String(target))
	if err != nil {
		t.Fatal(err)
	}

	v, _ := callMethod(p, "is_symlink")
	if v != starlark.True {
		t.Error("symlink_to should create symlink")
	}
}

func TestPathListdir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	p := newTestPath(dir)
	v, err := callMethod(p, "listdir")
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 2 {
		t.Errorf("listdir len = %d, want 2", list.Len())
	}
	// Entries should be Path objects
	entry := list.Index(0).(*Path)
	if !strings.HasPrefix(entry.path, dir) {
		t.Errorf("listdir entry should have full path, got %q", entry.path)
	}
}

func TestPathGlob(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	p := newTestPath(dir)
	v, err := callMethod(p, "glob", starlark.String("*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() != 2 {
		t.Errorf("glob('*.txt') len = %d, want 2", list.Len())
	}
}

// ============================================================================
// try_ dispatch tests
// ============================================================================

func TestPathTryDispatch(t *testing.T) {
	p := newTestPath("/nonexistent/path/12345.txt")

	// try_read_text should return Result, not error
	v, err := p.Attr("try_read_text")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("try_read_text should return a builtin")
	}

	// Call it
	result, err := starlark.Call(p.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("try_read_text should return Result, got %T", result)
	}
	okVal, _ := r.Attr("ok")
	if okVal != starlark.False {
		t.Error("try_read_text on missing file should have ok=False")
	}
}

func TestPathTryReadTextSuccess(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "try_test.txt")
	os.WriteFile(tmp, []byte("hello"), 0644)
	p := newTestPath(tmp)

	v, _ := p.Attr("try_read_text")
	result, err := starlark.Call(p.thread, v, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*starbase.Result)
	okVal, _ := r.Attr("ok")
	if okVal != starlark.True {
		t.Error("try_read_text on existing file should have ok=True")
	}
	val, _ := r.Attr("value")
	s, _ := starlark.AsString(val)
	if s != "hello" {
		t.Errorf("try_read_text value = %q, want %q", s, "hello")
	}
}

func TestPathTryUnknownMethod(t *testing.T) {
	p := newTestPath("/tmp")
	v, err := p.Attr("try_nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("try_nonexistent should return nil")
	}
}

func TestPathTryPureMethodNotAvailable(t *testing.T) {
	// try_ should still work for pure methods like join since they go through methodBuiltin
	p := newTestPath("/tmp")
	v, err := p.Attr("try_join")
	if err != nil {
		t.Fatal(err)
	}
	// try_join is valid since join is a real method
	if v == nil {
		t.Error("try_join should return a wrapped builtin")
	}
}

// ============================================================================
// Slash operator tests
// ============================================================================

func TestPathSlashOperator(t *testing.T) {
	p := newTestPath("/etc")

	// Path / string
	v, err := p.Binary(syntax.SLASH, starlark.String("nginx"), starlark.Left)
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "/etc/nginx" {
		t.Errorf("/ operator = %q, want %q", result.path, "/etc/nginx")
	}

	// Path / Path
	other := newTestPath("nginx.conf")
	v, err = result.Binary(syntax.SLASH, other, starlark.Left)
	if err != nil {
		t.Fatal(err)
	}
	final := v.(*Path)
	if final.path != "/etc/nginx/nginx.conf" {
		t.Errorf("/ operator = %q, want %q", final.path, "/etc/nginx/nginx.conf")
	}
}

func TestPathSlashOperatorRightSide(t *testing.T) {
	p := newTestPath("/etc")
	// Right side should return nil (decline)
	v, err := p.Binary(syntax.SLASH, starlark.String("x"), starlark.Right)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("right side of / should return nil")
	}
}

// ============================================================================
// AttrNames tests
// ============================================================================

func TestPathAttrNames(t *testing.T) {
	p := newTestPath("/tmp")
	names := p.AttrNames()

	required := []string{
		"name", "parent", "stem", "suffix", "string", "parts",
		"join", "with_name", "with_suffix", "resolve",
		"is_absolute", "match", "expanduser",
		"exists", "is_file", "is_dir", "is_symlink",
		"stat", "owner", "group",
		"read_text", "read_bytes",
		"write_text", "write_bytes", "append_text", "append_bytes",
		"touch", "mkdir", "remove", "rename",
		"copy_to", "move_to", "truncate",
		"chmod", "chown", "symlink_to", "readlink", "hardlink_to",
		"listdir", "glob", "walk", "disk_usage", "clean",
		"try_read_text", "try_write_text", "try_exists",
		"try_copy_to", "try_move_to", "try_truncate",
		"try_chown", "try_readlink", "try_hardlink_to",
		"try_walk", "try_disk_usage",
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("AttrNames missing %q", r)
		}
	}
}

// ============================================================================
// DryRun tests
// ============================================================================

func TestPathDryRunWrite(t *testing.T) {
	p := &Path{
		path:   "/tmp/dryrun_test.txt",
		thread: &starlark.Thread{Name: "test"},
		config: &starbase.ModuleConfig{DryRun: true},
	}

	v, err := callMethod(p, "write_text", starlark.String("data"))
	if err != nil {
		t.Fatal(err)
	}
	if v != starlark.None {
		t.Error("dry run write should return None")
	}
	if _, err := os.Stat("/tmp/dryrun_test.txt"); err == nil {
		t.Error("dry run should not create file")
	}
}

// ============================================================================
// Helper: stem
// ============================================================================

func TestStem(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/var/log/app.log", "app"},
		{"/etc/hostname", "hostname"},
		{"file.tar.gz", "file.tar"},
		{".hidden", ".hidden"},
	}
	for _, tt := range tests {
		if got := stem(tt.path); got != tt.want {
			t.Errorf("stem(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestPathParts_Relative(t *testing.T) {
	list := pathParts("a/b/c")
	expected := []string{"a", "b", "c"}
	if list.Len() != len(expected) {
		t.Fatalf("parts len = %d, want %d", list.Len(), len(expected))
	}
	for i, want := range expected {
		got, _ := starlark.AsString(list.Index(i))
		if got != want {
			t.Errorf("parts[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestPathParts_Empty(t *testing.T) {
	list := pathParts("")
	if list.Len() != 0 {
		t.Errorf("parts of empty = %d, want 0", list.Len())
	}
}

// ============================================================================
// Owner/group test helper
// ============================================================================

func TestPathOwnerCurrentUser(t *testing.T) {
	// Write to temp file owned by current user
	tmp := filepath.Join(t.TempDir(), "owner_test.txt")
	os.WriteFile(tmp, []byte("x"), 0644)

	p := newTestPath(tmp)
	v, err := callMethod(p, "owner")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := starlark.AsString(v)
	u, _ := user.Current()
	if s != u.Username {
		t.Errorf("owner = %q, want %q", s, u.Username)
	}
}

// ============================================================================
// New method tests: copy_to, move_to, truncate, chown, readlink, hardlink_to, walk, disk_usage, clean
// ============================================================================

func TestPathClean(t *testing.T) {
	p := newTestPath("/var/log/../log/./syslog")
	v, err := callMethod(p, "clean")
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != "/var/log/syslog" {
		t.Errorf("clean = %q, want %q", result.path, "/var/log/syslog")
	}
}

func TestPathCopyTo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("copy test"), 0644)

	p := newTestPath(src)
	v, err := callMethod(p, "copy_to", starlark.String(dst))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != dst {
		t.Errorf("copy_to returned path %q, want %q", result.path, dst)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "copy test" {
		t.Errorf("copied content = %q, want %q", string(data), "copy test")
	}
	// Source should still exist
	if _, err := os.Stat(src); err != nil {
		t.Error("source should still exist after copy")
	}
}

func TestPathMoveTo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("move test"), 0644)

	p := newTestPath(src)
	v, err := callMethod(p, "move_to", starlark.String(dst))
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != dst {
		t.Errorf("move_to returned path %q, want %q", result.path, dst)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "move test" {
		t.Errorf("moved content = %q, want %q", string(data), "move test")
	}
	if _, err := os.Stat(src); err == nil {
		t.Error("source should not exist after move")
	}
}

func TestPathTruncate(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "trunc.txt")
	os.WriteFile(tmp, []byte("hello world"), 0644)

	p := newTestPath(tmp)
	_, err := callMethod(p, "truncate", starlark.MakeInt64(5))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(tmp)
	if string(data) != "hello" {
		t.Errorf("truncated content = %q, want %q", string(data), "hello")
	}
}

func TestPathReadlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("data"), 0644)
	os.Symlink(target, link)

	p := newTestPath(link)
	v, err := callMethod(p, "readlink")
	if err != nil {
		t.Fatal(err)
	}
	result := v.(*Path)
	if result.path != target {
		t.Errorf("readlink = %q, want %q", result.path, target)
	}
}

func TestPathHardlinkTo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "hardlink.txt")
	os.WriteFile(target, []byte("hard link data"), 0644)

	p := newTestPath(link)
	_, err := callMethod(p, "hardlink_to", starlark.String(target))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(link)
	if string(data) != "hard link data" {
		t.Errorf("hardlink content = %q, want %q", string(data), "hard link data")
	}
}

func TestPathWalk(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644)

	p := newTestPath(dir)
	v, err := callMethod(p, "walk")
	if err != nil {
		t.Fatal(err)
	}
	list := v.(*starlark.List)
	if list.Len() < 2 {
		t.Errorf("walk should return at least 2 entries (root + sub), got %d", list.Len())
	}
	// First entry should be the root dir
	first := list.Index(0).(starlark.Tuple)
	rootPath := first[0].(*Path)
	if rootPath.path != dir {
		t.Errorf("walk root = %q, want %q", rootPath.path, dir)
	}
}

func TestPathDiskUsage(t *testing.T) {
	p := newTestPath("/")
	v, err := callMethod(p, "disk_usage")
	if err != nil {
		t.Fatal(err)
	}
	dict, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("disk_usage should return dict, got %s", v.Type())
	}
	for _, key := range []string{"total", "used", "free"} {
		if val, found, _ := dict.Get(starlark.String(key)); !found || val == nil {
			t.Errorf("disk_usage should have key %q", key)
		}
	}
}

// ============================================================================
// Test helpers
// ============================================================================

func callMethod(p *Path, name string, args ...starlark.Value) (starlark.Value, error) {
	method, err := p.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("%s has no attribute %q", p.Type(), name))
	}
	return starlark.Call(p.thread, method, args, nil)
}

func callMethodKw(p *Path, name string, args starlark.Tuple, kwargs ...starlark.Tuple) (starlark.Value, error) {
	method, err := p.Attr(name)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("%s has no attribute %q", p.Type(), name))
	}
	return starlark.Call(p.thread, method, args, kwargs)
}

func kw(name string, val starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), val}
}
