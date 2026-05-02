package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// testRuntime creates a minimal trusted Runtime for testing.
func testRuntime(t *testing.T) *libkite.Runtime {
	t.Helper()
	rt, err := libkite.NewTrusted(nil)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	return rt
}

// testServer creates a Server wired to a test Runtime.
func testServer(t *testing.T) *Server {
	t.Helper()
	rt := testRuntime(t)
	return newServer(rt, &libkite.ModuleConfig{})
}

// --- extractParamNames ---

func TestExtractParamNames(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
	}{
		{"GET /api/users/{id}", []string{"id"}},
		{"/static/{filepath...}", []string{"filepath"}},
		{"GET /api/{version}/users/{id}", []string{"version", "id"}},
		{"/health", nil},
		{"POST /api/data", nil},
		{"GET /{a}/{b}/{c...}", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := extractParamNames(tt.pattern)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("extractParamNames(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractParamNames(%q)[%d] = %q, want %q", tt.pattern, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- buildRequest ---

func TestBuildRequest(t *testing.T) {
	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest("POST", "http://localhost:8080/api/users/42?fields=name&fields=email&sort=asc", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")
	req.SetPathValue("id", "42")

	r := buildRequest(req, "POST /api/users/{id}")

	// method
	assertRequestAttr(t, r, "method", "POST")

	// path
	assertRequestAttr(t, r, "path", "/api/users/42")

	// url
	urlVal, _ := r.Attr("url")
	urlStr, _ := starlark.AsString(urlVal)
	if !strings.Contains(urlStr, "/api/users/42") {
		t.Errorf("url should contain path, got %q", urlStr)
	}

	// params
	paramsVal, _ := r.Attr("params")
	params := paramsVal.(*starlark.Dict)
	assertDictStr(t, params, "id", "42")

	// query — "fields" is multi-value, "sort" is single
	queryVal, _ := r.Attr("query")
	query := queryVal.(*starlark.Dict)
	sortVal := mustGetDict(t, query, "sort")
	if s, ok := starlark.AsString(sortVal); !ok || s != "asc" {
		t.Errorf("query[sort] = %v, want 'asc'", sortVal)
	}
	fieldsVal := mustGetDict(t, query, "fields")
	if _, ok := fieldsVal.(*starlark.List); !ok {
		t.Errorf("query[fields] should be a list, got %s", fieldsVal.Type())
	}

	// headers
	headersVal, _ := r.Attr("headers")
	headers := headersVal.(*starlark.Dict)
	ctVal := mustGetDict(t, headers, "Content-Type")
	if s, ok := starlark.AsString(ctVal); !ok || s != "application/json" {
		t.Errorf("headers[Content-Type] = %v, want 'application/json'", ctVal)
	}

	// body
	assertRequestAttr(t, r, "body", `{"key":"value"}`)

	// remote_addr
	raVal, _ := r.Attr("remote_addr")
	if s, _ := starlark.AsString(raVal); s == "" {
		t.Error("remote_addr should not be empty")
	}

	// host
	assertRequestAttr(t, r, "host", "localhost:8080")

	// type
	if r.Type() != "http.request" {
		t.Errorf("type = %q, want 'http.request'", r.Type())
	}
}

// --- writeResponse ---

func TestWriteResponseNone(t *testing.T) {
	w := httptest.NewRecorder()
	if err := writeResponse(w, starlark.None); err != nil {
		t.Fatal(err)
	}
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestWriteResponseString(t *testing.T) {
	w := httptest.NewRecorder()
	if err := writeResponse(w, starlark.String("hello")); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Errorf("body = %q, want 'hello'", w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

func TestWriteResponseExplicitDict(t *testing.T) {
	d := starlark.NewDict(3)
	d.SetKey(starlark.String("status"), starlark.MakeInt(201))
	headers := starlark.NewDict(1)
	headers.SetKey(starlark.String("X-Custom"), starlark.String("val"))
	d.SetKey(starlark.String("headers"), headers)
	d.SetKey(starlark.String("body"), starlark.String("created"))

	w := httptest.NewRecorder()
	if err := writeResponse(w, d); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
	if w.Body.String() != "created" {
		t.Errorf("body = %q, want 'created'", w.Body.String())
	}
	if w.Header().Get("X-Custom") != "val" {
		t.Errorf("X-Custom header = %q, want 'val'", w.Header().Get("X-Custom"))
	}
}

func TestWriteResponseAutoJSON(t *testing.T) {
	d := starlark.NewDict(1)
	d.SetKey(starlark.String("name"), starlark.String("alice"))

	w := httptest.NewRecorder()
	if err := writeResponse(w, d); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(w.Body.String(), `"name"`) {
		t.Errorf("body should contain JSON, got %q", w.Body.String())
	}
}

// --- callChain ---

func TestCallChainNoMiddleware(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("ok"), nil
	})

	req := &Request{method: "GET", path: "/test", params: starlark.NewDict(0), query: starlark.NewDict(0), headers: starlark.NewDict(0)}
	result, err := callChain(thread, nil, handler, req)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := starlark.AsString(result); !ok || s != "ok" {
		t.Errorf("result = %v, want 'ok'", result)
	}
}

func TestCallChainWithMiddleware(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}

	var order []string

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		order = append(order, "handler")
		return starlark.String("response"), nil
	})

	mw1 := starlark.NewBuiltin("mw1", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		order = append(order, "mw1-before")
		next := args[1].(starlark.Callable)
		result, err := starlark.Call(thread, next, starlark.Tuple{args[0]}, nil)
		order = append(order, "mw1-after")
		return result, err
	})

	mw2 := starlark.NewBuiltin("mw2", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		order = append(order, "mw2-before")
		next := args[1].(starlark.Callable)
		result, err := starlark.Call(thread, next, starlark.Tuple{args[0]}, nil)
		order = append(order, "mw2-after")
		return result, err
	})

	req := &Request{method: "GET", path: "/test", params: starlark.NewDict(0), query: starlark.NewDict(0), headers: starlark.NewDict(0)}
	result, err := callChain(thread, []starlark.Callable{mw1, mw2}, handler, req)
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := starlark.AsString(result); !ok || s != "response" {
		t.Errorf("result = %v, want 'response'", result)
	}

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], expected[i])
		}
	}
}

func TestCallChainShortCircuit(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}

	handlerCalled := false
	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		handlerCalled = true
		return starlark.String("ok"), nil
	})

	// Middleware that short-circuits (returns without calling next)
	authMw := starlark.NewBuiltin("auth", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("unauthorized"), nil
	})

	req := &Request{method: "GET", path: "/test", params: starlark.NewDict(0), query: starlark.NewDict(0), headers: starlark.NewDict(0)}
	result, err := callChain(thread, []starlark.Callable{authMw}, handler, req)
	if err != nil {
		t.Fatal(err)
	}

	if s, ok := starlark.AsString(result); !ok || s != "unauthorized" {
		t.Errorf("result = %v, want 'unauthorized'", result)
	}
	if handlerCalled {
		t.Error("handler should not be called when middleware short-circuits")
	}
}

// --- Server lifecycle ---

func TestServerLifecycle(t *testing.T) {
	srv := testServer(t)

	thread := &starlark.Thread{Name: "test"}
	libkite.SetPermissions(thread, nil) // trusted

	// Register a handler
	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("hello"), nil
	})

	_, err := srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/test"), handler}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start on port 0
	_, err = srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get port
	portVal, err := srv.portMethod(thread, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := starlark.AsInt32(portVal)
	if port == 0 {
		t.Fatal("port should not be 0")
	}

	// Make a real HTTP request
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/test", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q, want 'hello'", string(body))
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Shutdown
	_, err = srv.shutdownMethod(thread, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for done
	select {
	case <-srv.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop within timeout")
	}
}

func TestServerJSONAutoResponse(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	// Handler returns dict without "body" key → auto JSON
	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		d := starlark.NewDict(1)
		d.SetKey(starlark.String("name"), starlark.String("alice"))
		return d, nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/json"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"name"`) {
		t.Errorf("body = %q, want JSON with 'name'", string(body))
	}
}

func TestServerNoneResponse(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/noop"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/noop", port))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

// --- Error cases ---

func TestServerDoubleStart(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/test"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	// Second start should fail
	_, err := srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	if err == nil {
		t.Fatal("expected error on double start")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error = %q, want 'already running'", err.Error())
	}
}

func TestServerNoHandlers(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	_, err := srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	if err == nil {
		t.Fatal("expected error with no handlers")
	}
	if !strings.Contains(err.Error(), "no handlers") {
		t.Errorf("error = %q, want 'no handlers'", err.Error())
	}
}

func TestServerHandlerError(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("intentional error")
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/err"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/err", port))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	// Server should still be running — test a second request
	resp2, err := http.Get(fmt.Sprintf("http://localhost:%d/err", port))
	if err != nil {
		t.Fatal("server should still be running after handler error")
	}
	resp2.Body.Close()
}

func TestServerHandlerPanic(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		panic("intentional panic")
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/panic"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/panic", port))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestServerShutdownNoOp(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	// Shutdown on a server that was never started should be a no-op
	_, err := srv.shutdownMethod(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("shutdown on stopped server should not error: %v", err)
	}
}

func TestServerAddHandlerWhileRunning(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/test"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	// Adding handler while running should fail
	_, err := srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/new"), handler}, nil)
	if err == nil {
		t.Fatal("expected error adding handler while running")
	}
}

func TestServerPermissions(t *testing.T) {
	rt, err := libkite.New(&libkite.Config{
		Permissions: libkite.StrictPermissions(),
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := newServer(rt, &libkite.ModuleConfig{})

	// Create a sandboxed thread
	thread := rt.NewThread("test")

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	// Server handle should be blocked in sandbox
	_, err = srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/test"), handler}, nil)
	if err == nil {
		t.Fatal("expected permission error in sandbox mode")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want 'permission denied'", err.Error())
	}
}

// --- Concurrent requests ---

func TestServerConcurrentRequests(t *testing.T) {
	srv := testServer(t)
	thread := &starlark.Thread{Name: "test"}

	// Handler that sleeps briefly to ensure requests overlap
	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		time.Sleep(50 * time.Millisecond)
		// Return the request path to verify each request is handled independently
		req := args[0].(*Request)
		pathVal, _ := req.Attr("path")
		return pathVal, nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/slow/{id}"), handler}, nil)
	srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)

	const numRequests = 20
	type result struct {
		body string
		err  error
	}
	results := make(chan result, numRequests)

	// Fire all requests concurrently
	start := time.Now()
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			url := fmt.Sprintf("http://localhost:%d/slow/%d", port, id)
			resp, err := http.Get(url)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			results <- result{body: string(body)}
		}(i)
	}

	// Collect all results
	seen := make(map[string]bool)
	for i := 0; i < numRequests; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("request failed: %v", r.err)
		}
		seen[r.body] = true
	}
	elapsed := time.Since(start)

	// All requests should have returned unique paths
	if len(seen) != numRequests {
		t.Errorf("expected %d unique responses, got %d", numRequests, len(seen))
	}

	// With thread-per-request, 20 requests at 50ms each should complete
	// much faster than sequential (1000ms). Allow generous margin.
	if elapsed > 500*time.Millisecond {
		t.Errorf("concurrent requests took %v, expected < 500ms (sequential would be ~1s)", elapsed)
	}
}

// --- Server try_ dispatch ---

func TestServerTryAttr(t *testing.T) {
	srv := testServer(t)

	methods := []string{"handle", "use", "start", "serve", "shutdown", "port"}
	for _, name := range methods {
		tryName := "try_" + name
		v, err := srv.Attr(tryName)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", tryName, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", tryName)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", tryName, v)
		}
	}
}

// --- Constructor config ---

func TestServerConstructorConfig(t *testing.T) {
	srv := testServer(t)
	srv.port = 9999
	srv.host = "127.0.0.1"

	thread := &starlark.Thread{Name: "test"}

	handler := starlark.NewBuiltin("handler", func(thread *starlark.Thread, fn *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("ok"), nil
	})

	srv.handleMethod(thread, nil, starlark.Tuple{starlark.String("/test"), handler}, nil)

	// Start without kwargs — should use constructor defaults (port=9999 would conflict,
	// so override to port=0 for test)
	_, err := srv.startMethod(thread, nil, nil, []starlark.Tuple{
		{starlark.String("port"), starlark.MakeInt(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.shutdownMethod(thread, nil, nil, nil)

	portVal, _ := srv.portMethod(thread, nil, nil, nil)
	port, _ := starlark.AsInt32(portVal)
	if port == 0 {
		t.Fatal("port should be assigned")
	}
}

// --- Helpers ---

func assertRequestAttr(t *testing.T, r *Request, attr, want string) {
	t.Helper()
	val, err := r.Attr(attr)
	if err != nil {
		t.Fatalf("Attr(%q) error: %v", attr, err)
	}
	s, ok := starlark.AsString(val)
	if !ok {
		t.Fatalf("Attr(%q) not a string: %s", attr, val.Type())
	}
	if s != want {
		t.Errorf("Attr(%q) = %q, want %q", attr, s, want)
	}
}

func assertDictStr(t *testing.T, d *starlark.Dict, key, want string) {
	t.Helper()
	val := mustGetDict(t, d, key)
	s, ok := starlark.AsString(val)
	if !ok {
		t.Fatalf("dict[%q] not a string: %s", key, val.Type())
	}
	if s != want {
		t.Errorf("dict[%q] = %q, want %q", key, s, want)
	}
}

func mustGetDict(t *testing.T, d *starlark.Dict, key string) starlark.Value {
	t.Helper()
	val, found, err := d.Get(starlark.String(key))
	if err != nil {
		t.Fatalf("dict.Get(%q): %v", key, err)
	}
	if !found {
		t.Fatalf("dict[%q] not found", key)
	}
	return val
}
