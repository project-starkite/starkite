package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/ai/modules/genai"
	"github.com/project-starkite/starkite/libkite"
)

const (
	defaultServerVersion = "0.1.0"
	defaultHTTPHost      = "127.0.0.1"
	defaultHTTPPath      = "/"
	httpShutdownTimeout  = 5 * time.Second
)

// serveBuiltin is the `mcp.serve(...)` Starlark builtin. It validates kwargs,
// coerces tools/resources/prompts, and starts a blocking MCP server on
// either stdio (default) or HTTP (when `port=` is set). Returns None on clean
// shutdown (SIGINT/SIGTERM); returns an error if the server fails to start
// or exits abnormally.
//
// Kwargs:
//
//	name       : string (required)
//	version    : string (default "0.1.0")
//	tools      : list of functions or ai.Tool values
//	resources  : dict[name] → callable (0 or 1 arg)
//	prompts    : dict[name] → callable
//
// HTTP transport kwargs (when port is set):
//
//	port       : int     — listen port; omit or 0 for stdio
//	host       : string  — bind address (default "127.0.0.1")
//	path       : string  — HTTP path to mount the MCP handler at (default "/")
//	tls_cert   : string  — path to PEM cert; must be paired with tls_key
//	tls_key    : string  — path to PEM key;  must be paired with tls_cert
func (m *Module) serveBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("mcp.serve: takes only keyword arguments, got %d positional", len(args))
	}

	var p struct {
		Name      string         `name:"name"`
		Version   string         `name:"version"`
		Tools     starlark.Value `name:"tools"`
		Resources starlark.Value `name:"resources"`
		Prompts   starlark.Value `name:"prompts"`

		// HTTP transport kwargs (Slice 2.4)
		Port    int    `name:"port"`
		Host    string `name:"host"`
		Path    string `name:"path"`
		TLSCert string `name:"tls_cert"`
		TLSKey  string `name:"tls_key"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("mcp.serve: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("mcp.serve: name is required")
	}
	if p.Version == "" {
		p.Version = defaultServerVersion
	}
	if p.Port < 0 {
		return nil, fmt.Errorf("mcp.serve: port must be non-negative, got %d", p.Port)
	}
	// Reject partial TLS config early so the error surfaces at validation time
	// (before we do any expensive work).
	if (p.TLSCert != "") != (p.TLSKey != "") {
		return nil, fmt.Errorf("mcp.serve: tls_cert and tls_key must both be set to enable TLS")
	}
	// TLS without port doesn't make sense (stdio ignores them).
	if p.Port == 0 && (p.TLSCert != "" || p.Host != "" || p.Path != "") {
		return nil, fmt.Errorf("mcp.serve: host/path/tls_* kwargs require port to be set")
	}

	var tools []*genai.Tool
	if p.Tools != nil {
		coerced, err := genai.CoerceTools(p.Tools)
		if err != nil {
			return nil, fmt.Errorf("mcp.serve: %w", err)
		}
		tools = coerced
	}

	var resources []*resourceEntry
	if p.Resources != nil {
		coerced, err := coerceResources(p.Resources)
		if err != nil {
			return nil, fmt.Errorf("mcp.serve: %w", err)
		}
		resources = coerced
	}

	var prompts []*promptEntry
	if p.Prompts != nil {
		coerced, err := coercePrompts(p.Prompts)
		if err != nil {
			return nil, fmt.Errorf("mcp.serve: %w", err)
		}
		prompts = coerced
	}

	if err := libkite.Check(thread, "mcp", "server", "serve", p.Name); err != nil {
		return nil, err
	}

	rt := libkite.GetRuntime(thread)
	if rt == nil {
		return nil, fmt.Errorf("mcp.serve: no runtime available in thread")
	}

	server, err := buildServer(p.Name, p.Version, tools, resources, prompts, rt)
	if err != nil {
		return nil, err
	}

	if p.Port > 0 {
		return starlark.None, runHTTPServer(server, httpOpts{
			host:    firstNonEmpty(p.Host, defaultHTTPHost),
			port:    p.Port,
			path:    firstNonEmpty(p.Path, defaultHTTPPath),
			tlsCert: p.TLSCert,
			tlsKey:  p.TLSKey,
		})
	}
	return starlark.None, runStdioServer(server)
}

// buildServer constructs an MCP server from our inputs and registers the
// provided tools, resources, and prompts. Extracted so unit tests can
// exercise registration logic without calling server.Run (which blocks on stdio).
func buildServer(name, version string, tools []*genai.Tool, resources []*resourceEntry, prompts []*promptEntry, rt *libkite.Runtime) (*mcpsdk.Server, error) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: name, Version: version}, nil)

	for _, t := range tools {
		schema, err := schemaMapToJSONSchema(t.Params())
		if err != nil {
			return nil, fmt.Errorf("mcp.serve: tool %q: %w", t.Name(), err)
		}
		server.AddTool(&mcpsdk.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		}, buildToolHandler(t, rt))
	}

	for _, r := range resources {
		server.AddResource(&mcpsdk.Resource{
			Name:        r.name,
			URI:         r.uri,
			Description: r.description,
			MIMEType:    r.mimeType,
		}, buildResourceHandler(r, rt))
	}

	for _, p := range prompts {
		server.AddPrompt(&mcpsdk.Prompt{
			Name:        p.name,
			Description: p.description,
			Arguments:   p.arguments,
		}, buildPromptHandler(p, rt))
	}

	return server, nil
}

// runStdioServer blocks on server.Run(&StdioTransport{}) with signal-driven
// graceful shutdown. Follows the pattern from libkite/modules/http/server.go.
func runStdioServer(server *mcpsdk.Server) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		cancel()
	}()

	if err := server.Run(ctx, &mcpsdk.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("mcp.serve: %w", err)
	}
	return nil
}

// httpOpts carries the resolved HTTP server configuration from user kwargs.
type httpOpts struct {
	host, path, tlsCert, tlsKey string
	port                        int
}

// runHTTPServer is the production HTTP entry: wires SIGINT/SIGTERM to a cancel
// context, then delegates to runHTTPServerCtx for the actual listen loop.
func runHTTPServer(server *mcpsdk.Server, opts httpOpts) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		cancel()
	}()

	return runHTTPServerCtx(ctx, server, opts)
}

// runHTTPServerCtx runs the HTTP transport under an explicit cancellation
// context, exposing the test seam we need to spin up an ephemeral-port server
// and shut it down deterministically.
//
// Shuts down gracefully when ctx is cancelled (up to httpShutdownTimeout).
// Prints a one-line startup message to stderr on entry.
func runHTTPServerCtx(ctx context.Context, server *mcpsdk.Server, opts httpOpts) error {
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		nil,
	)

	mux := http.NewServeMux()
	mux.Handle(opts.path, handler)

	addr := net.JoinHostPort(opts.host, strconv.Itoa(opts.port))
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	tlsEnabled := opts.tlsCert != "" && opts.tlsKey != ""

	// Shutdown loop — kicks in when the caller cancels ctx.
	go func() {
		<-ctx.Done()
		shutdownCtx, sdCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer sdCancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	fmt.Fprintf(os.Stderr, "mcp.serve: listening on %s://%s%s\n", scheme, addr, opts.path)

	var err error
	if tlsEnabled {
		err = httpSrv.ListenAndServeTLS(opts.tlsCert, opts.tlsKey)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("mcp.serve: %w", err)
}

// firstNonEmpty returns the first string that isn't empty, else the fallback.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
