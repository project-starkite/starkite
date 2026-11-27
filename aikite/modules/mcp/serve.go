package mcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const (
	defaultServerVersion = "0.1.0"
	defaultHTTPHost      = "127.0.0.1"
	defaultHTTPPath      = "/"
	httpShutdownTimeout  = 5 * time.Second
)

var stdioMu sync.Mutex

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

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
		p.Name = "starkite"
	}
	if p.Version == "" {
		p.Version = defaultServerVersion
	}
	if p.Port < 0 {
		return nil, fmt.Errorf("mcp.serve: port must be non-negative, got %d", p.Port)
	}
	if p.Port > 65535 {
		return nil, fmt.Errorf("mcp.serve: port must be between 0 and 65535, got %d", p.Port)
	}
	// Reject partial TLS config early so the error surfaces at validation time
	// (before any expensive work runs).
	if (p.TLSCert != "") != (p.TLSKey != "") {
		return nil, fmt.Errorf("mcp.serve: tls_cert and tls_key must both be set to enable TLS")
	}
	// TLS without port doesn't make sense (stdio ignores them).
	if p.Port == 0 && (p.TLSCert != "" || p.Host != "" || p.Path != "") {
		return nil, fmt.Errorf("mcp.serve: host/path/tls_* kwargs require port to be set")
	}
	if p.Port > 0 && p.Path != "" && !strings.HasPrefix(p.Path, "/") {
		return nil, fmt.Errorf("mcp.serve: path must start with '/', got %q", p.Path)
	}

	var tools []*Tool
	if p.Tools != nil {
		coerced, err := CoerceTools(p.Tools)
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
	if p.TLSCert != "" && p.TLSKey != "" {
		if err := libkite.Check(thread, "fs", "read", "tls_cert", libkite.CanonicalizePath(p.TLSCert)); err != nil {
			return nil, err
		}
		if err := libkite.Check(thread, "fs", "read", "tls_key", libkite.CanonicalizePath(p.TLSKey)); err != nil {
			return nil, err
		}
	}

	if m.mcfg != nil && m.mcfg.DryRun {
		if p.Port > 0 {
			fmt.Fprintf(os.Stderr, "[DRY RUN] mcp.serve: would listen on %s:%d%s\n", firstNonEmpty(p.Host, defaultHTTPHost), p.Port, firstNonEmpty(p.Path, defaultHTTPPath))
		} else {
			fmt.Fprintf(os.Stderr, "[DRY RUN] mcp.serve: would serve %q on stdio\n", p.Name)
		}
		return starlark.None, nil
	}

	rt := libkite.GetRuntime(thread)
	if rt == nil {
		return nil, fmt.Errorf("mcp.serve: no runtime available in thread")
	}

	stdioMode := p.Port == 0
	server, err := buildServerWithOptions(p.Name, p.Version, tools, resources, prompts, rt, stdioMode)
	if err != nil {
		return nil, err
	}

	threadCtx := libkite.ThreadContext(thread)
	if p.Port > 0 {
		return starlark.None, runHTTPServer(threadCtx, server, httpOpts{
			host:    firstNonEmpty(p.Host, defaultHTTPHost),
			port:    p.Port,
			path:    firstNonEmpty(p.Path, defaultHTTPPath),
			tlsCert: p.TLSCert,
			tlsKey:  p.TLSKey,
		})
	}
	return starlark.None, runStdioServer(threadCtx, server)
}

// buildServer constructs an MCP server from the resolved inputs and registers
// the provided tools, resources, and prompts.
func buildServer(name, version string, tools []*Tool, resources []*resourceEntry, prompts []*promptEntry, rt *libkite.Runtime) (*mcpsdk.Server, error) {
	return buildServerWithOptions(name, version, tools, resources, prompts, rt, false)
}

func buildServerWithOptions(name, version string, tools []*Tool, resources []*resourceEntry, prompts []*promptEntry, rt *libkite.Runtime, stdioMode bool) (*mcpsdk.Server, error) {
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
		}, buildToolHandlerWithStdio(t, rt, stdioMode))
	}

	for _, r := range resources {
		server.AddResource(&mcpsdk.Resource{
			Name:        r.name,
			URI:         r.uri,
			Description: r.description,
			MIMEType:    r.mimeType,
		}, buildResourceHandlerWithStdio(r, rt, stdioMode))
	}

	for _, p := range prompts {
		server.AddPrompt(&mcpsdk.Prompt{
			Name:        p.name,
			Description: p.description,
			Arguments:   p.arguments,
		}, buildPromptHandlerWithStdio(p, rt, stdioMode))
	}

	return server, nil
}

// runStdioServer runs the MCP server over stdio while diverting accidental
// os.Stdout writes to os.Stderr so JSON-RPC framing on stdout stays clean.
func runStdioServer(parentCtx context.Context, server *mcpsdk.Server) error {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	stdioMu.Lock()
	origStdout := os.Stdout
	os.Stdout = os.Stderr
	stdioMu.Unlock()
	defer func() {
		stdioMu.Lock()
		os.Stdout = origStdout
		stdioMu.Unlock()
	}()

	transport := &mcpsdk.IOTransport{
		Reader: os.Stdin,
		Writer: nopWriteCloser{origStdout},
	}
	if err := server.Run(parentCtx, transport); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("mcp.serve: %w", err)
	}
	return nil
}

// httpOpts carries the resolved HTTP server configuration from user kwargs.
type httpOpts struct {
	host, path, tlsCert, tlsKey string
	port                        int
}

// runHTTPServer delegates to runHTTPServerCtx under parentCtx.
func runHTTPServer(parentCtx context.Context, server *mcpsdk.Server, opts httpOpts) error {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return runHTTPServerCtx(parentCtx, server, opts)
}

// runHTTPServerCtx binds the HTTP listener before reporting readiness and shuts
// down gracefully when ctx is cancelled.
func runHTTPServerCtx(ctx context.Context, server *mcpsdk.Server, opts httpOpts) error {
	if ctx == nil {
		ctx = context.Background()
	}
	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()

	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		nil,
	)

	mux := http.NewServeMux()
	mux.Handle(opts.path, handler)

	addr := net.JoinHostPort(opts.host, strconv.Itoa(opts.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mcp.serve: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	tlsEnabled := opts.tlsCert != "" && opts.tlsKey != ""
	if tlsEnabled {
		httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	go func() {
		<-srvCtx.Done()
		shutdownCtx, sdCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer sdCancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	listenAddr := addr
	if opts.port == 0 {
		listenAddr = ln.Addr().String()
	}
	fmt.Fprintf(os.Stderr, "mcp.serve: listening on %s://%s%s\n", scheme, listenAddr, opts.path)

	if tlsEnabled {
		err = httpSrv.ServeTLS(ln, opts.tlsCert, opts.tlsKey)
	} else {
		err = httpSrv.Serve(ln)
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
