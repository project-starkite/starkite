package wasm

import (
	"errors"
	"testing"
)

func TestWasmError_Error(t *testing.T) {
	err := &WasmError{
		Plugin:   "myplugin",
		Function: "myfunc",
		Message:  "something broke",
	}
	s := err.Error()
	if s != `wasm plugin "myplugin" function "myfunc": something broke` {
		t.Errorf("unexpected error string: %s", s)
	}
}

func TestWasmError_ErrorWithCause(t *testing.T) {
	err := &WasmError{
		Plugin:   "p",
		Function: "f",
		Message:  "failed",
		Cause:    errors.New("root cause"),
	}
	s := err.Error()
	if s != `wasm plugin "p" function "f": failed: root cause` {
		t.Errorf("unexpected error string: %s", s)
	}
}

func TestWasmError_Unwrap(t *testing.T) {
	cause := errors.New("inner")
	err := &WasmError{Plugin: "p", Function: "f", Message: "m", Cause: cause}
	if err.Unwrap() != cause {
		t.Error("Unwrap should return cause")
	}
}

func TestManifestError_Error(t *testing.T) {
	err := &ManifestError{Path: "/path/to/module.yaml", Message: "bad format"}
	s := err.Error()
	if s != `manifest "/path/to/module.yaml": bad format` {
		t.Errorf("unexpected error string: %s", s)
	}
}

func TestIsWasmError(t *testing.T) {
	if !IsWasmError(&WasmError{}) {
		t.Error("expected true for *WasmError")
	}
	if IsWasmError(errors.New("other")) {
		t.Error("expected false for non-WasmError")
	}
	if IsWasmError(nil) {
		t.Error("expected false for nil")
	}
}
