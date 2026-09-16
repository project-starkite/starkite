package libkite

import (
	"go.starlark.net/starlark"
)

const scriptArgsKey = "libkite.scriptArgs"

// ScriptArgsContext holds command-line arguments forwarded to the script.
type ScriptArgsContext struct {
	RawArgs []string
	Parsed  bool
}

// SetScriptArgs stores a ScriptArgsContext in thread.Local.
func SetScriptArgs(thread *starlark.Thread, ctx *ScriptArgsContext) {
	if thread != nil {
		thread.SetLocal(scriptArgsKey, ctx)
	}
}

// GetScriptArgs retrieves the ScriptArgsContext from thread.Local.
// Returns nil if no script args context is set.
func GetScriptArgs(thread *starlark.Thread) *ScriptArgsContext {
	if thread == nil {
		return nil
	}
	ctx, _ := thread.Local(scriptArgsKey).(*ScriptArgsContext)
	return ctx
}
