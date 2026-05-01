package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	extism "github.com/extism/go-sdk"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

// HostContext provides context for host function calls from WASM guests.
type HostContext struct {
	config     *libkite.ModuleConfig
	moduleName string
	thread     *starlark.Thread
}

// Build creates host functions for the given permission set.
// Only host functions matching the manifest permissions are registered.
func (h *HostContext) Build(permissions []string) []extism.HostFunction {
	permSet := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		permSet[p] = true
	}

	var funcs []extism.HostFunction

	// log is always available
	funcs = append(funcs, h.hostLog())

	if permSet["env"] {
		funcs = append(funcs, h.hostEnv())
	}
	if permSet["var"] {
		funcs = append(funcs, h.hostVar())
	}
	if permSet["exec"] {
		funcs = append(funcs, h.hostExec())
	}
	if permSet["read_file"] {
		funcs = append(funcs, h.hostReadFile())
	}
	if permSet["write_file"] {
		funcs = append(funcs, h.hostWriteFile())
	}
	if permSet["http"] {
		funcs = append(funcs, h.hostHTTPRequest())
	}

	// Set namespace for all host functions
	for i := range funcs {
		funcs[i].SetNamespace("extism:host/user")
	}

	return funcs
}

// checkPermission validates a host call against the permission system.
func (h *HostContext) checkPermission(function, resource string) error {
	if h.thread == nil {
		return nil
	}
	return libkite.Check(h.thread, h.moduleName, function, resource)
}

// hostLog creates the "log" host function. Always allowed.
// Input JSON: {"level": "info", "message": "..."}
func (h *HostContext) hostLog() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"log",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				plugin.Log(extism.LogLevelError, fmt.Sprintf("log: read error: %v", err))
				return
			}

			var req struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				plugin.Log(extism.LogLevelError, fmt.Sprintf("log: parse error: %v", err))
				return
			}

			switch strings.ToLower(req.Level) {
			case "debug":
				log.Printf("[DEBUG] [%s] %s", h.moduleName, req.Message)
			case "warn", "warning":
				log.Printf("[WARN] [%s] %s", h.moduleName, req.Message)
			case "error":
				log.Printf("[ERROR] [%s] %s", h.moduleName, req.Message)
			default:
				log.Printf("[INFO] [%s] %s", h.moduleName, req.Message)
			}
		},
		[]extism.ValueType{extism.ValueTypePTR},
		nil,
	)
}

// hostEnv creates the "env" host function.
// Input JSON: {"name": "VAR_NAME"}
// Output JSON: {"value": "...", "ok": true}
func (h *HostContext) hostEnv() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"env",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("env", req.Name); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			value, ok := os.LookupEnv(req.Name)
			h.writeJSON(plugin, stack, map[string]any{"value": value, "ok": ok})
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// hostVar creates the "var" host function.
// Input JSON: {"key": "..."}
// Output JSON: {"value": ..., "ok": true}
func (h *HostContext) hostVar() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"var",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("var", req.Key); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			var result map[string]any
			if h.config != nil && h.config.VarStore != nil {
				value, ok := h.config.VarStore.Get(req.Key)
				result = map[string]any{"value": value, "ok": ok}
			} else {
				result = map[string]any{"value": nil, "ok": false}
			}
			h.writeJSON(plugin, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// hostExec creates the "exec" host function.
// Input JSON: {"command": "...", "args": ["..."]}
// Output JSON: {"stdout": "...", "stderr": "...", "exit_code": 0}
func (h *HostContext) hostExec() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"exec",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("exec", req.Command); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			cmd := exec.CommandContext(ctx, req.Command, req.Args...)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			exitCode := 0
			if err := cmd.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					h.writeError(plugin, stack, fmt.Sprintf("exec error: %v", err))
					return
				}
			}

			h.writeJSON(plugin, stack, map[string]any{
				"stdout":    stdout.String(),
				"stderr":    stderr.String(),
				"exit_code": exitCode,
			})
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// hostReadFile creates the "read_file" host function.
// Input JSON: {"path": "..."}
// Output JSON: {"content": "...", "error": ""}
func (h *HostContext) hostReadFile() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"read_file",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("read_file", req.Path); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			content, err := os.ReadFile(req.Path)
			if err != nil {
				h.writeJSON(plugin, stack, map[string]any{"content": "", "error": err.Error()})
				return
			}

			h.writeJSON(plugin, stack, map[string]any{"content": string(content), "error": ""})
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// hostWriteFile creates the "write_file" host function.
// Input JSON: {"path": "...", "content": "..."}
// Output JSON: {"error": ""}
func (h *HostContext) hostWriteFile() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"write_file",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("write_file", req.Path); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			err = os.WriteFile(req.Path, []byte(req.Content), 0644)
			if err != nil {
				h.writeJSON(plugin, stack, map[string]any{"error": err.Error()})
				return
			}

			h.writeJSON(plugin, stack, map[string]any{"error": ""})
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// hostHTTPRequest creates the "http_request" host function.
// Input JSON: {"url": "...", "method": "GET", "headers": {...}, "body": "..."}
// Output JSON: {"status": 200, "body": "...", "headers": {...}, "error": ""}
func (h *HostContext) hostHTTPRequest() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"http_request",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			input, err := plugin.ReadBytes(stack[0])
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("read error: %v", err))
				return
			}

			var req struct {
				URL     string            `json:"url"`
				Method  string            `json:"method"`
				Headers map[string]string `json:"headers"`
				Body    string            `json:"body"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("parse error: %v", err))
				return
			}

			if err := h.checkPermission("http_request", req.URL); err != nil {
				h.writeError(plugin, stack, err.Error())
				return
			}

			// Build and execute HTTP request
			httpReq, err := newHTTPRequest(ctx, req.Method, req.URL, req.Headers, req.Body)
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("request error: %v", err))
				return
			}

			resp, err := doHTTPRequest(httpReq)
			if err != nil {
				h.writeError(plugin, stack, fmt.Sprintf("request error: %v", err))
				return
			}

			h.writeJSON(plugin, stack, resp)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// writeJSON marshals val as JSON and writes it to the plugin output via the stack.
func (h *HostContext) writeJSON(plugin *extism.CurrentPlugin, stack []uint64, val any) {
	data, err := json.Marshal(val)
	if err != nil {
		log.Printf("wasm host: json marshal error: %v", err)
		return
	}
	offset, err := plugin.WriteBytes(data)
	if err != nil {
		log.Printf("wasm host: write error: %v", err)
		return
	}
	stack[0] = offset
}

// writeError writes an error response to the plugin output.
func (h *HostContext) writeError(plugin *extism.CurrentPlugin, stack []uint64, msg string) {
	h.writeJSON(plugin, stack, map[string]any{"error": msg})
}
