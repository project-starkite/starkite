// Package http provides HTTP client and server functionality for starkite.
// This is a stateful module that self-configures.
package http

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "http"

// Module implements HTTP client functionality.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig

	// HTTP configuration
	timeout time.Duration
	headers map[string]string
	client  *http.Client
	mu      sync.RWMutex
}

func New() *Module {
	return &Module{
		timeout: 30 * time.Second,
		headers: make(map[string]string),
	}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "http provides HTTP client and server: url, config, server, serve"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.client = &http.Client{Timeout: m.timeout}
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			// Client
			"url":    starlark.NewBuiltin("http.url", m.urlFactory),
			"config": starlark.NewBuiltin("http.config", m.configFn),

			// Server
			"server": starlark.NewBuiltin("http.server", m.serverConstructor),
			"serve":  starlark.NewBuiltin("http.serve", m.serveFn),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "server" }

// urlFactory creates a URL object: http.url("https://example.com")
func (m *Module) urlFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("http.url: expected 1 argument (url), got %d", len(args))
	}
	url, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("http.url: url must be a string, got %s", args[0].Type())
	}
	return &URL{rawURL: url, thread: thread, module: m}, nil
}

// configFn configures the HTTP client.
// Usage: http.config(timeout="30s", headers={"Authorization": "Bearer token"})
func (m *Module) configFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Extract headers dict manually (startype.Args doesn't handle *starlark.Dict)
	var headers *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "headers", &headers)

	var timeoutStr string
	for _, kv := range filteredKwargs {
		key := string(kv[0].(starlark.String))
		if key == "timeout" {
			if s, ok := starlark.AsString(kv[1]); ok {
				timeoutStr = s
			} else {
				return nil, fmt.Errorf("http.config: timeout must be a string (e.g. \"30s\"), got %s", kv[1].Type())
			}
		}
	}

	if err := starbase.Check(thread, "http", "config", ""); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("http.config: invalid timeout %q: %w", timeoutStr, err)
		}
		m.timeout = d
		m.client = &http.Client{Timeout: m.timeout}
	}

	if headers != nil {
		for _, item := range headers.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					m.headers[k] = v
				}
			}
		}
	}

	return starlark.None, nil
}

// serverConstructor creates a new Server: http.server(port=8080, read_timeout="30s", ...).
func (m *Module) serverConstructor(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", ""); err != nil {
		return nil, err
	}

	rt := starbase.GetRuntime(thread)
	if rt == nil {
		return nil, fmt.Errorf("http.server: runtime not available")
	}

	srv := newServer(rt, m.config)

	// Parse optional kwargs — all config consolidated on constructor
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "port":
			if p, err := starlark.AsInt32(kv[1]); err == nil {
				srv.port = int(p)
			}
		case "host":
			if h, ok := starlark.AsString(kv[1]); ok {
				srv.host = h
			}
		case "tls_cert":
			if c, ok := starlark.AsString(kv[1]); ok {
				srv.tlsCert = c
			}
		case "tls_key":
			if k, ok := starlark.AsString(kv[1]); ok {
				srv.tlsKey = k
			}
		case "read_timeout", "write_timeout", "idle_timeout", "shutdown_timeout":
			s, ok := starlark.AsString(kv[1])
			if !ok {
				return nil, fmt.Errorf("http.server: %s must be a string (e.g. \"30s\"), got %s", key, kv[1].Type())
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return nil, fmt.Errorf("http.server: invalid %s %q: %w", key, s, err)
			}
			switch key {
			case "read_timeout":
				srv.readTimeout = d
			case "write_timeout":
				srv.writeTimeout = d
			case "idle_timeout":
				srv.idleTimeout = d
			case "shutdown_timeout":
				srv.shutdownTimeout = d
			}
		case "max_header_bytes":
			if v, err := starlark.AsInt32(kv[1]); err == nil {
				srv.maxHeaderBytes = int(v)
			}
		case "max_body_bytes":
			if v, err := starlark.AsInt32(kv[1]); err == nil {
				srv.maxBodyBytes = int64(v)
			}
		}
	}

	return srv, nil
}

// serveFn is a one-liner shortcut: http.serve(handler_or_routes, port=8080, ...).
// First arg is a callable (single handler) or a dict (pattern→handler mapping).
// Remaining kwargs are server config. Creates a server, registers handlers, and blocks.
func (m *Module) serveFn(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if len(args) != 1 {
		return nil, fmt.Errorf("http.serve: expected 1 argument (handler or routes dict), got %d", len(args))
	}
	handler := args[0]

	// Create server via serverConstructor with kwargs
	srvVal, err := m.serverConstructor(thread, fn, nil, kwargs)
	if err != nil {
		return nil, err
	}
	srv := srvVal.(*Server)

	// Register handler(s)
	switch h := handler.(type) {
	case *starlark.Dict:
		// Dict: pattern → handler mapping
		for _, item := range h.Items() {
			pattern, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("http.serve: route pattern must be a string, got %s", item[0].Type())
			}
			callable, ok := item[1].(starlark.Callable)
			if !ok {
				return nil, fmt.Errorf("http.serve: route handler must be callable, got %s", item[1].Type())
			}
			srv.handlers = append(srv.handlers, registeredHandler{pattern: pattern, handler: callable})
		}
	case starlark.Callable:
		// Single callable → handle all paths
		srv.handlers = append(srv.handlers, registeredHandler{pattern: "/", handler: h})
	default:
		return nil, fmt.Errorf("http.serve: first argument must be callable or dict, got %s", handler.Type())
	}

	// Block via serveMethod
	return srv.serveMethod(thread, fn, nil, nil)
}

// filterKwarg extracts a single kwarg by name (expected to be *starlark.Dict),
// sets *dest if found, and returns remaining kwargs.
func filterKwarg(kwargs []starlark.Tuple, name string, dest **starlark.Dict) []starlark.Tuple {
	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == name {
			if d, ok := kv[1].(*starlark.Dict); ok {
				*dest = d
			}
		} else {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}
