package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const (
	// defaultResourceMIME is used when a resource handler returns a plain
	// string (no explicit MIME via dict-form return).
	defaultResourceMIME = "text/plain"

	// resourceURIPrefix prefixes auto-generated URIs for resources. Users
	// supply a short name (e.g. "cluster-info"); we synthesize
	// "starkite://resources/cluster-info" as the MCP URI.
	resourceURIPrefix = "starkite://resources/"
)

// resourceEntry is the registration record built once at mcp.serve() time.
// Handler construction consults arity to decide whether to pass the URI
// into the Starlark function.
type resourceEntry struct {
	name        string
	uri         string
	description string
	mimeType    string
	fn          starlark.Callable
	arity       int // 0 or 1
}

// resourceURI returns the MCP URI for a resource name, preserving explicit URI schemes.
func resourceURI(name string) string {
	if strings.Contains(name, "://") {
		return name
	}
	return resourceURIPrefix + name
}

// coerceResources converts the user's Starlark dict into a slice of registration
// records. Validates that keys are strings, values are callables with compatible
// arity (0 or 1), and rejects *args/**kwargs.
func coerceResources(v starlark.Value) ([]*resourceEntry, error) {
	d, ok, err := libkite.ValueToDict(v)
	if err != nil {
		return nil, fmt.Errorf("resources: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("resources must be a dict or struct, got %s", v.Type())
	}
	out := make([]*resourceEntry, 0, d.Len())
	for _, item := range d.Items() {
		keyStr, ok := item[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("resource keys must be strings, got %s", item[0].Type())
		}
		name := string(keyStr)
		if name == "" {
			return nil, fmt.Errorf("resource key must not be empty")
		}
		callable, ok := item[1].(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("resource %q: value must be callable, got %s", name, item[1].Type())
		}
		arity, err := resourceArity(name, callable)
		if err != nil {
			return nil, err
		}
		out = append(out, &resourceEntry{
			name:        name,
			uri:         resourceURI(name),
			description: starlarkDoc(callable),
			mimeType:    defaultResourceMIME,
			fn:          callable,
			arity:       arity,
		})
	}
	return out, nil
}

// resourceArity returns 0 or 1 for a valid resource callable. Non-def
// callables (starlark.Builtin, etc.) are introspection-free, so 0 is assumed.
// Rejects 2+ params, *args, and **kwargs.
func resourceArity(name string, callable starlark.Callable) (int, error) {
	fn, ok := callable.(*starlark.Function)
	if !ok {
		return 0, nil
	}
	if fn.HasVarargs() {
		return 0, fmt.Errorf("resource %q: function must not use *args", name)
	}
	if fn.HasKwargs() {
		return 0, fmt.Errorf("resource %q: function must not use **kwargs", name)
	}
	switch n := fn.NumParams(); n {
	case 0, 1:
		return n, nil
	default:
		return 0, fmt.Errorf("resource %q: function must take 0 or 1 parameter, got %d", name, n)
	}
}

// starlarkDoc returns the docstring when the callable is a def-function.
// Builtins and other callable types have no docstring to extract.
func starlarkDoc(callable starlark.Callable) string {
	if fn, ok := callable.(*starlark.Function); ok {
		return fn.Doc()
	}
	return ""
}

// buildResourceHandler returns the MCP resource handler closure.
//
// Each read runs on a fresh Starlark thread (preserving permissions). The
// function's return value may be a string (text/plain by default) or a dict
// {"content": "...", "mime_type": "..."} for explicit MIME. Any Starlark
// error is surfaced as the MCP handler error.
func buildResourceHandler(r *resourceEntry, rt *libkite.Runtime) func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	return buildResourceHandlerWithStdio(r, rt, false)
}

func buildResourceHandlerWithStdio(r *resourceEntry, rt *libkite.Runtime, stdioMode bool) func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		thread := rt.NewThread("mcp-resource-" + r.name)
		if stdioMode {
			thread.Print = func(_ *starlark.Thread, msg string) {
				fmt.Fprintln(os.Stderr, msg)
			}
		}
		libkite.SetThreadContext(thread, ctx)
		stopWatch := libkite.WatchContext(ctx, thread)
		defer stopWatch()

		if err := libkite.Check(thread, "mcp", "server", "resource_read", r.name); err != nil {
			return nil, err
		}

		var args starlark.Tuple
		if r.arity == 1 {
			args = starlark.Tuple{starlark.String(req.Params.URI)}
		}

		var text, mime string
		callErr := libkite.WithCallLock(thread, func() error {
			result, err := starlark.Call(thread, r.fn, args, nil)
			if err != nil {
				return err
			}
			var extractErr error
			text, mime, extractErr = extractResourceContent(result, r.mimeType)
			return extractErr
		})
		if callErr != nil {
			return nil, fmt.Errorf("resource %q: %w", r.name, callErr)
		}

		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: mime,
				Text:     text,
			}},
		}, nil
	}
}

// extractResourceContent interprets a resource function's return value.
//
//   - starlark.String → use directly with defaultMime
//   - dict/struct → requires string "content" and optional string "mime_type"
//   - anything else → stringify via String() and use defaultMime
func extractResourceContent(v starlark.Value, defaultMime string) (string, string, error) {
	if s, ok := starlark.AsString(v); ok {
		return s, defaultMime, nil
	}
	d, ok, err := libkite.ValueToDict(v)
	if err != nil {
		return "", "", err
	}
	if ok {
		cv, found, getErr := d.Get(starlark.String("content"))
		if getErr != nil {
			return "", "", getErr
		}
		if !found {
			return "", "", fmt.Errorf("resource dict/struct must include 'content'")
		}
		content, ok := starlark.AsString(cv)
		if !ok {
			return "", "", fmt.Errorf("resource 'content' must be a string, got %s", cv.Type())
		}
		mime := defaultMime
		if mv, found, getErr := d.Get(starlark.String("mime_type")); getErr == nil && found && mv != starlark.None {
			mStr, ok := starlark.AsString(mv)
			if !ok {
				return "", "", fmt.Errorf("resource 'mime_type' must be a string, got %s", mv.Type())
			}
			if mStr != "" {
				mime = mStr
			}
		}
		return content, mime, nil
	}
	return strings.TrimSpace(v.String()), defaultMime, nil
}
