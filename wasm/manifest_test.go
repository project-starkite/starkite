package wasm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	manifest, err := ParseManifest(filepath.Join("testdata", "echo", "module.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.Name != "echo" {
		t.Errorf("name = %q, want %q", manifest.Name, "echo")
	}
	if manifest.Version != "0.1.0" {
		t.Errorf("version = %q, want %q", manifest.Version, "0.1.0")
	}
	if manifest.Wasm != "echo.wasm" {
		t.Errorf("wasm = %q, want %q", manifest.Wasm, "echo.wasm")
	}
	if len(manifest.Functions) != 2 {
		t.Fatalf("functions count = %d, want 2", len(manifest.Functions))
	}

	echo := manifest.Functions[0]
	if echo.Name != "echo" {
		t.Errorf("functions[0].name = %q, want %q", echo.Name, "echo")
	}
	if echo.Returns != "string" {
		t.Errorf("functions[0].returns = %q, want %q", echo.Returns, "string")
	}
	if len(echo.Params) != 1 {
		t.Fatalf("functions[0].params count = %d, want 1", len(echo.Params))
	}
	if echo.Params[0].Name != "input" {
		t.Errorf("functions[0].params[0].name = %q, want %q", echo.Params[0].Name, "input")
	}
	if echo.Params[0].Type != "string" {
		t.Errorf("functions[0].params[0].type = %q, want %q", echo.Params[0].Type, "string")
	}

	add := manifest.Functions[1]
	if add.Name != "add" {
		t.Errorf("functions[1].name = %q, want %q", add.Name, "add")
	}
	if add.Returns != "int" {
		t.Errorf("functions[1].returns = %q, want %q", add.Returns, "int")
	}
	if len(add.Params) != 2 {
		t.Fatalf("functions[1].params count = %d, want 2", len(add.Params))
	}

	if len(manifest.Permissions) != 1 || manifest.Permissions[0] != "log" {
		t.Errorf("permissions = %v, want [log]", manifest.Permissions)
	}
}

func TestParseManifest_MissingFile(t *testing.T) {
	_, err := ParseManifest("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	var mErr *ManifestError
	if me, ok := err.(*ManifestError); !ok {
		t.Fatalf("expected ManifestError, got %T", err)
	} else {
		mErr = me
	}
	if mErr.Path != "nonexistent.yaml" {
		t.Errorf("path = %q, want %q", mErr.Path, "nonexistent.yaml")
	}
}

func TestParseManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yaml")
	os.WriteFile(path, []byte("{{{{invalid yaml"), 0644)

	_, err := ParseManifest(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidate_MissingName(t *testing.T) {
	m := &PluginManifest{Version: "1.0", Wasm: "a.wasm", Functions: []FunctionManifest{{Name: "f", Returns: "string"}}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	m := &PluginManifest{Name: "x", Wasm: "a.wasm", Functions: []FunctionManifest{{Name: "f", Returns: "string"}}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestValidate_MissingWasm(t *testing.T) {
	m := &PluginManifest{Name: "x", Version: "1.0", Functions: []FunctionManifest{{Name: "f", Returns: "string"}}}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing wasm")
	}
}

func TestValidate_NoFunctions(t *testing.T) {
	m := &PluginManifest{Name: "x", Version: "1.0", Wasm: "a.wasm"}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for no functions")
	}
}

func TestValidate_InvalidReturnType(t *testing.T) {
	m := &PluginManifest{
		Name: "x", Version: "1.0", Wasm: "a.wasm",
		Functions: []FunctionManifest{{Name: "f", Returns: "invalid"}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for invalid return type")
	}
}

func TestValidate_InvalidParamType(t *testing.T) {
	m := &PluginManifest{
		Name: "x", Version: "1.0", Wasm: "a.wasm",
		Functions: []FunctionManifest{{
			Name:    "f",
			Returns: "string",
			Params:  []ParamManifest{{Name: "p", Type: "badtype"}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for invalid param type")
	}
}

func TestValidate_EmptyFunctionName(t *testing.T) {
	m := &PluginManifest{
		Name: "x", Version: "1.0", Wasm: "a.wasm",
		Functions: []FunctionManifest{{Name: "", Returns: "string"}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty function name")
	}
}

func TestValidate_EmptyParamName(t *testing.T) {
	m := &PluginManifest{
		Name: "x", Version: "1.0", Wasm: "a.wasm",
		Functions: []FunctionManifest{{
			Name:    "f",
			Returns: "string",
			Params:  []ParamManifest{{Name: "", Type: "string"}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty param name")
	}
}

func TestValidate_EmptyParamType(t *testing.T) {
	m := &PluginManifest{
		Name: "x", Version: "1.0", Wasm: "a.wasm",
		Functions: []FunctionManifest{{
			Name:    "f",
			Returns: "string",
			Params:  []ParamManifest{{Name: "p", Type: ""}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty param type")
	}
}

func TestFunctionManifest_ExportName(t *testing.T) {
	// Export defaults to Name
	fn := FunctionManifest{Name: "foo"}
	if fn.ExportName() != "foo" {
		t.Errorf("ExportName() = %q, want %q", fn.ExportName(), "foo")
	}

	// Export overrides Name
	fn = FunctionManifest{Name: "foo", Export: "bar"}
	if fn.ExportName() != "bar" {
		t.Errorf("ExportName() = %q, want %q", fn.ExportName(), "bar")
	}
}

func TestParamManifest_IsRequired(t *testing.T) {
	// Default is required
	p := ParamManifest{Name: "x", Type: "string"}
	if !p.IsRequired() {
		t.Error("default should be required")
	}

	// Explicitly required
	b := true
	p = ParamManifest{Name: "x", Type: "string", Required: &b}
	if !p.IsRequired() {
		t.Error("required=true should be required")
	}

	// Optional
	b = false
	p = ParamManifest{Name: "x", Type: "string", Required: &b}
	if p.IsRequired() {
		t.Error("required=false should not be required")
	}
}
