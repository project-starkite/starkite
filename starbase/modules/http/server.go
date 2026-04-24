package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	gohttp "net/http"
	"strings"
	"sync"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// registeredHandler pairs a route pattern with its Starlark handler.
type registeredHandler struct {
	pattern string
	handler starlark.Callable
}

// Server is a Starlark-visible HTTP server implementing starlark.HasAttrs.
type Server struct {
	// Config (all set in constructor, overridable in start/serve)
	port            int
	host            string
	tlsCert         string
	tlsKey          string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	maxHeaderBytes  int
	maxBodyBytes    int64
	shutdownTimeout time.Duration

	// State
	mux         *gohttp.ServeMux
	server      *gohttp.Server
	listener    net.Listener
	handlers    []registeredHandler
	middlewares []starlark.Callable
	running     bool
	runtime     *starbase.Runtime
	config      *starbase.ModuleConfig

	// Synchronization
	mu   sync.Mutex
	done chan struct{}
}

// newServer creates a Server with default config.
func newServer(runtime *starbase.Runtime, config *starbase.ModuleConfig) *Server {
	return &Server{
		maxHeaderBytes:  1 << 20,      // 1MB
		maxBodyBytes:    10 << 20,     // 10MB
		shutdownTimeout: 5 * time.Second,
		runtime:         runtime,
		config:          config,
	}
}

// --- starlark.Value ---

func (s *Server) String() string        { return "<http.server>" }
func (s *Server) Type() string          { return "http.server" }
func (s *Server) Freeze()               {}
func (s *Server) Truth() starlark.Bool  { return starlark.True }
func (s *Server) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: http.server") }

// --- starlark.HasAttrs ---

func (s *Server) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := s.methodBuiltin(base); method != nil {
			return starbase.TryWrap("http.server."+name, method), nil
		}
		return nil, nil
	}

	if method := s.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (s *Server) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "handle":
		return starlark.NewBuiltin("server.handle", s.handleMethod)
	case "use":
		return starlark.NewBuiltin("server.use", s.useMethod)
	case "start":
		return starlark.NewBuiltin("server.start", s.startMethod)
	case "serve":
		return starlark.NewBuiltin("server.serve", s.serveMethod)
	case "shutdown":
		return starlark.NewBuiltin("server.shutdown", s.shutdownMethod)
	case "port":
		return starlark.NewBuiltin("server.port", s.portMethod)
	}
	return nil
}

func (s *Server) AttrNames() []string {
	return []string{
		"handle", "port", "serve", "shutdown", "start", "use",
		"try_handle", "try_port", "try_serve", "try_shutdown", "try_start", "try_use",
	}
}

// handleMethod registers a route handler: srv.handle(pattern, handler).
func (s *Server) handleMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "handle"); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil, fmt.Errorf("http.server: cannot add handlers while server is running")
	}

	if len(args) != 2 {
		return nil, fmt.Errorf("handle: expected (pattern, handler), got %d args", len(args))
	}

	pattern, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("handle: pattern must be a string, got %s", args[0].Type())
	}

	handler, ok := args[1].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("handle: handler must be callable, got %s", args[1].Type())
	}

	s.handlers = append(s.handlers, registeredHandler{pattern: pattern, handler: handler})
	return starlark.None, nil
}

// useMethod registers a middleware: srv.use(middleware).
func (s *Server) useMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "use"); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil, fmt.Errorf("http.server: cannot add middleware while server is running")
	}

	if len(args) != 1 {
		return nil, fmt.Errorf("use: expected 1 argument (middleware), got %d", len(args))
	}

	mw, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("use: middleware must be callable, got %s", args[0].Type())
	}

	s.middlewares = append(s.middlewares, mw)
	return starlark.None, nil
}

// serveMethod starts the server and blocks: srv.serve(port=8080).
// If no SIGINT/SIGTERM handler is registered, installs a default one
// that calls shutdown() for graceful termination.
func (s *Server) serveMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "serve"); err != nil {
		return nil, err
	}

	if err := s.startInternal(kwargs); err != nil {
		return nil, err
	}

	// Install default SIGINT/SIGTERM handlers if none are registered,
	// so the server can shut down gracefully on Ctrl-C.
	var installedSIGINT, installedSIGTERM bool
	if s.runtime != nil {
		shutdownFn := starlark.NewBuiltin("http.server.shutdown-handler",
			func(thread *starlark.Thread, fn *starlark.Builtin,
				args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				s.shutdownInternal()
				return starlark.None, nil
			})

		if !s.runtime.HasSignalHandler("interrupt") {
			s.runtime.RegisterSignalHandler("interrupt", shutdownFn)
			installedSIGINT = true
		}
		if !s.runtime.HasSignalHandler("terminated") {
			s.runtime.RegisterSignalHandler("terminated", shutdownFn)
			installedSIGTERM = true
		}
	}

	// Block until server stops
	<-s.done

	// Clean up default handlers we installed
	if s.runtime != nil {
		if installedSIGINT {
			s.runtime.UnregisterSignalHandler("interrupt")
		}
		if installedSIGTERM {
			s.runtime.UnregisterSignalHandler("terminated")
		}
	}

	return starlark.None, nil
}

// startMethod starts the server without blocking: srv.start(port=8080).
func (s *Server) startMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "start"); err != nil {
		return nil, err
	}

	if err := s.startInternal(kwargs); err != nil {
		return nil, err
	}

	return starlark.None, nil
}

// shutdownMethod gracefully shuts down the server.
func (s *Server) shutdownMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "shutdown"); err != nil {
		return nil, err
	}

	s.shutdownInternal()
	return starlark.None, nil
}

// shutdownInternal performs the actual graceful shutdown.
// Safe to call from any goroutine (signal handlers, request handlers).
func (s *Server) shutdownInternal() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	srv := s.server
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		// Force close on timeout
		srv.Close()
	}
}

// portMethod returns the server's listening port.
func (s *Server) portMethod(thread *starlark.Thread, fn *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if err := starbase.Check(thread, "http", "server", "port"); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return nil, fmt.Errorf("http.server: server not started, no port available")
	}

	port := s.listener.Addr().(*net.TCPAddr).Port
	return starlark.MakeInt(port), nil
}

// startInternal is shared by serve() and start().
func (s *Server) startInternal(kwargs []starlark.Tuple) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("http.server: server already running, call shutdown() first")
	}

	if len(s.handlers) == 0 {
		return fmt.Errorf("http.server: no handlers registered, call handle() first")
	}

	// Start with constructor defaults, allow kwargs to override
	port := s.port
	host := s.host
	tlsCert := s.tlsCert
	tlsKey := s.tlsKey
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "port":
			if p, err := starlark.AsInt32(kv[1]); err == nil {
				port = int(p)
			}
		case "host":
			if h, ok := starlark.AsString(kv[1]); ok {
				host = h
			}
		case "tls_cert":
			if c, ok := starlark.AsString(kv[1]); ok {
				tlsCert = c
			}
		case "tls_key":
			if k, ok := starlark.AsString(kv[1]); ok {
				tlsKey = k
			}
		}
	}

	// Build mux and register handlers
	s.mux = gohttp.NewServeMux()
	// Snapshot middlewares at start time
	mws := make([]starlark.Callable, len(s.middlewares))
	copy(mws, s.middlewares)

	for _, h := range s.handlers {
		s.mux.HandleFunc(h.pattern, s.makeGoHandler(h.pattern, h.handler, mws))
	}

	// Create listener
	addr := fmt.Sprintf("%s:%d", host, port)
	var ln net.Listener
	var err error

	if tlsCert != "" && tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return fmt.Errorf("http.server: failed to load TLS cert: %w", err)
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
		ln, err = tls.Listen("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("http.server: listen: %w", err)
		}
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("http.server: listen: %w", err)
		}
	}

	s.listener = ln
	s.server = &gohttp.Server{
		Handler:        s.mux,
		ReadTimeout:    s.readTimeout,
		WriteTimeout:   s.writeTimeout,
		IdleTimeout:    s.idleTimeout,
		MaxHeaderBytes: s.maxHeaderBytes,
	}

	s.done = make(chan struct{})
	s.running = true

	go func() {
		defer close(s.done)
		s.server.Serve(ln)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	return nil
}

// makeGoHandler creates an http.HandlerFunc that executes the Starlark handler
// with thread-per-request concurrency.
func (s *Server) makeGoHandler(pattern string, handler starlark.Callable,
	middlewares []starlark.Callable) gohttp.HandlerFunc {

	return func(w gohttp.ResponseWriter, r *gohttp.Request) {
		defer func() {
			if p := recover(); p != nil {
				gohttp.Error(w, fmt.Sprintf("handler panic: %v", p), 500)
			}
		}()

		// Limit request body size to prevent OOM from large payloads
		r.Body = gohttp.MaxBytesReader(w, r.Body, s.maxBodyBytes)

		reqObj := buildRequest(r, pattern)

		// Thread-per-request: new thread, shared frozen globals
		thread := s.runtime.NewThread("http-handler")

		result, err := callChain(thread, middlewares, handler, reqObj)
		if err != nil {
			gohttp.Error(w, fmt.Sprintf("handler error: %v", err), 500)
			return
		}

		if err := writeResponse(w, result); err != nil {
			gohttp.Error(w, fmt.Sprintf("response error: %v", err), 500)
		}
	}
}
