// Package libkite provides an embeddable Starlark runtime with built-in modules
// and a permission system for sandboxing untrusted scripts.
package libkite

import "fmt"

// Exit codes for libkite scripts.
const (
	ExitSuccess     = 0   // Successful execution
	ExitScriptError = 1   // Script execution error
	ExitFileError   = 2   // File not found or read error
	ExitSyntaxError = 3   // Starlark syntax error
	ExitConfigError = 4   // Configuration error
	ExitTimeout     = 5   // Script timed out
	ExitInterrupt   = 130 // SIGINT received
	ExitTerminate   = 143 // SIGTERM received
)

// PermissionError is returned when an operation is denied by the permission system.
type PermissionError struct {
	Module   string   // Module name (e.g., "fs", "os", "http")
	Category string   // Category within the module (e.g., "read", "write", "exec")
	Function string   // Function name (e.g., "read_file", "exec")
	Resource string   // Resource if applicable (path, URL, command)
	Reason   string   // Why it was denied
	Allowed  []string // What patterns would allow this (for helpful errors)
}

// Error implements the error interface.
func (e *PermissionError) Error() string {
	if e.Resource != "" {
		return fmt.Sprintf("permission denied: %s.%s(%q) - %s",
			e.Module, e.Function, e.Resource, e.Reason)
	}
	return fmt.Sprintf("permission denied: %s.%s - %s",
		e.Module, e.Function, e.Reason)
}

// IsPermissionError returns true if err is a PermissionError.
func IsPermissionError(err error) bool {
	_, ok := err.(*PermissionError)
	return ok
}

// ScriptError represents an error during script execution.
type ScriptError struct {
	Message  string
	ExitCode int
	Cause    error
}

// Error implements the error interface.
func (e *ScriptError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *ScriptError) Unwrap() error {
	return e.Cause
}

// NewScriptError creates a new script error.
func NewScriptError(msg string, code int, cause error) *ScriptError {
	return &ScriptError{
		Message:  msg,
		ExitCode: code,
		Cause:    cause,
	}
}

// ExitError represents an intentional exit with a specific code.
// This is used by the exit() builtin.
type ExitError struct {
	Code int
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	return ""
}

// exitError is an internal error type used for exit() calls within scripts.
// It's wrapped by ExitError when exposed to the caller.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit(%d)", e.code)
}

// SkipError is returned when a test is skipped.
type SkipError struct {
	Reason string
}

// Error implements the error interface.
func (e *SkipError) Error() string {
	if e.Reason != "" {
		return "test skipped: " + e.Reason
	}
	return "test skipped"
}

// IsScriptError returns true if err is a ScriptError.
func IsScriptError(err error) bool {
	_, ok := err.(*ScriptError)
	return ok
}

// IsExitError returns true if err is an ExitError.
func IsExitError(err error) bool {
	_, ok := err.(*ExitError)
	return ok
}

// IsSkipError returns true if err is a SkipError.
func IsSkipError(err error) bool {
	_, ok := err.(*SkipError)
	return ok
}
