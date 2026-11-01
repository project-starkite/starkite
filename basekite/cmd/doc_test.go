package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeDocCmd(args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	docJSON = false
	var filteredArgs []string
	for _, a := range args {
		if a == "--json" {
			docJSON = true
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}
	err := runDoc(cmd, filteredArgs)
	return buf.String(), err
}

func TestDocOverview(t *testing.T) {
	out, err := executeDocCmd()
	if err != nil {
		t.Fatalf("unexpected error running kite doc: %v", err)
	}
	for _, expected := range []string{"Starkite Reference Documentation", "fs", "http", "os", "json", "yaml"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected overview to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestDocModuleFS(t *testing.T) {
	out, err := executeDocCmd("fs")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs: %v", err)
	}
	for _, expected := range []string{"MODULE: fs", "path", "read_text", "try_read_text"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected module doc to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestDocSymbolPath(t *testing.T) {
	out, err := executeDocCmd("fs.path")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs.path: %v", err)
	}
	if !strings.Contains(out, "fs.path") {
		t.Errorf("expected output to contain 'fs.path', got:\n%s", out)
	}
}

func TestDocTwoArgs(t *testing.T) {
	out, err := executeDocCmd("fs", "path")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs path: %v", err)
	}
	if !strings.Contains(out, "fs.path") {
		t.Errorf("expected output to contain 'fs.path', got:\n%s", out)
	}
}

func TestDocGlobal(t *testing.T) {
	out, err := executeDocCmd("print")
	if err != nil {
		t.Fatalf("unexpected error running kite doc print: %v", err)
	}
	if !strings.Contains(out, "GLOBAL: print") {
		t.Errorf("expected global print output, got:\n%s", out)
	}
}

func TestDocJSONOverview(t *testing.T) {
	out, err := executeDocCmd("--json")
	if err != nil {
		t.Fatalf("unexpected error running kite doc --json: %v", err)
	}
	var data DocOverviewJSON
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, out)
	}
	if len(data.Modules) == 0 {
		t.Errorf("expected at least 1 module in JSON overview, got 0")
	}
}

func TestDocJSONModule(t *testing.T) {
	out, err := executeDocCmd("fs", "--json")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs --json: %v", err)
	}
	var data ModuleDocJSON
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, out)
	}
	if data.Name != "fs" {
		t.Errorf("expected module name 'fs', got %q", data.Name)
	}
	if len(data.Functions) == 0 {
		t.Errorf("expected functions in fs JSON doc, got 0")
	}
}

func TestDocNotFound(t *testing.T) {
	_, err := executeDocCmd("nonexistent_module_xyz")
	if err == nil {
		t.Fatalf("expected error for nonexistent module, got nil")
	}
	if !strings.Contains(err.Error(), "unknown module or symbol") {
		t.Errorf("expected unknown module error message, got: %v", err)
	}
}

func TestDocJSONSymbol(t *testing.T) {
	out, err := executeDocCmd("fs.path", "--json")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs.path --json: %v", err)
	}
	var data SymbolDocJSON
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, out)
	}
	if data.Symbol.Name != "path" || data.Symbol.Owner != "fs" {
		t.Errorf("expected symbol fs.path, got %+v", data.Symbol)
	}
	if len(data.Methods) == 0 {
		t.Errorf("expected fs.path type methods in JSON output, got 0")
	}
}

func TestDocInvalidSymbolFormat(t *testing.T) {
	for _, bad := range []string{".", ".foo", "foo."} {
		_, err := executeDocCmd(bad)
		if err == nil {
			t.Errorf("expected error for invalid symbol format %q, got nil", bad)
		} else if !strings.Contains(err.Error(), "invalid symbol format") {
			t.Errorf("expected 'invalid symbol format' error for %q, got: %v", bad, err)
		}
	}
}
