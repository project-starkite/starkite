package wasm

import "fmt"

// WasmError represents an error from a WASM plugin call.
type WasmError struct {
	Plugin   string // plugin name
	Function string // function that failed
	Message  string // error message
	Cause    error  // underlying error
}

func (e *WasmError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("wasm plugin %q function %q: %s: %v", e.Plugin, e.Function, e.Message, e.Cause)
	}
	return fmt.Sprintf("wasm plugin %q function %q: %s", e.Plugin, e.Function, e.Message)
}

func (e *WasmError) Unwrap() error {
	return e.Cause
}

// ManifestError represents an error in a plugin manifest.
type ManifestError struct {
	Path    string // path to the manifest file
	Message string // error description
}

func (e *ManifestError) Error() string {
	return fmt.Sprintf("manifest %q: %s", e.Path, e.Message)
}

// IsWasmError returns true if err is a *WasmError.
func IsWasmError(err error) bool {
	_, ok := err.(*WasmError)
	return ok
}
