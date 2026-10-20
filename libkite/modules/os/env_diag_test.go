package osmod

import (
	"bytes"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestEnvScrubbedDiagnostic_DebugMode(t *testing.T) {
	t.Setenv("KUBECONFIG", "/home/test/.kube/config")

	var buf bytes.Buffer
	m := NewWithConfig(&libkite.ModuleConfig{
		Debug: true,
		Env:   map[string]string{"OTHER": "1"},
	})
	m.SetDiagWriter(&buf)

	thread := &starlark.Thread{Name: "test"}
	thread.SetLocal("libkite.permissions", nil)

	// 1. First call emits debug diagnostic
	v, err := m.env_(thread, nil, starlark.Tuple{starlark.String("KUBECONFIG")}, nil)
	if err != nil {
		t.Fatalf("env_ error: %v", err)
	}
	if v.String() != `""` {
		t.Errorf("expected empty string, got %v", v)
	}
	out := buf.String()
	wantDiag := `[DEBUG] env "KUBECONFIG" requested by script is present on host but scrubbed by sandbox.`
	if !strings.Contains(out, wantDiag) {
		t.Errorf("diagnostic output missing expected text, got: %q", out)
	}
	wantHint := `Hint: pass --env KUBECONFIG on CLI, or declare "KUBECONFIG:" in sandbox.<profile>.env in config.yaml`
	if !strings.Contains(out, wantHint) {
		t.Errorf("diagnostic output missing expected hint, got: %q", out)
	}

	// 2. Second call is deduplicated
	buf.Reset()
	v2, err := m.env_(thread, nil, starlark.Tuple{starlark.String("KUBECONFIG")}, nil)
	if err != nil {
		t.Fatalf("second env_ error: %v", err)
	}
	if v2.String() != `""` {
		t.Errorf("expected empty string, got %v", v2)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no diagnostic on second call, got: %q", buf.String())
	}
}

func TestEnvScrubbedDiagnostic_NoDebug(t *testing.T) {
	t.Setenv("KUBECONFIG", "/home/test/.kube/config")

	var buf bytes.Buffer
	m := NewWithConfig(&libkite.ModuleConfig{
		Debug: false,
		Env:   map[string]string{"OTHER": "1"},
	})
	m.SetDiagWriter(&buf)

	thread := &starlark.Thread{Name: "test"}
	thread.SetLocal("libkite.permissions", nil)

	v, err := m.env_(thread, nil, starlark.Tuple{starlark.String("KUBECONFIG")}, nil)
	if err != nil {
		t.Fatalf("env_ error: %v", err)
	}
	if v.String() != `""` {
		t.Errorf("expected empty string, got %v", v)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no diagnostic when debug is false, got: %q", buf.String())
	}
}

func TestEnvScrubbedDiagnostic_NonExistentHostVar(t *testing.T) {
	var buf bytes.Buffer
	m := NewWithConfig(&libkite.ModuleConfig{
		Debug: true,
		Env:   map[string]string{"OTHER": "1"},
	})
	m.SetDiagWriter(&buf)

	thread := &starlark.Thread{Name: "test"}
	thread.SetLocal("libkite.permissions", nil)

	v, err := m.env_(thread, nil, starlark.Tuple{starlark.String("COMPLETELY_NONEXISTENT_VAR_12345")}, nil)
	if err != nil {
		t.Fatalf("env_ error: %v", err)
	}
	if v.String() != `""` {
		t.Errorf("expected empty string, got %v", v)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no diagnostic for non-existent host var, got: %q", buf.String())
	}
}
