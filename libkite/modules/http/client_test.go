package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestClientResponseLazyLoading(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "value123")
		fmt.Fprint(w, "hello streaming http")
	}))
	defer ts.Close()

	rt := testRuntime(t)
	defer rt.Close()

	// Load the http module
	httpMod := New()
	dict, err := httpMod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"http": dict["http"],
	}

	// 1. Test get_bytes, get_text, and body on-demand
	script := fmt.Sprintf(`
def run():
    resp = http.url("%s").get()
    return resp
`, ts.URL)

	thread := rt.NewThread("test-thread")
	globals, err := starlark.ExecFile(thread, "test.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}

	runFn := globals["run"]
	resVal, err := starlark.Call(thread, runFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, ok := resVal.(*Response)
	if !ok {
		t.Fatalf("expected http.response, got %T", resVal)
	}

	// Verify headers and status
	if resp.statusCode != 200 {
		t.Errorf("statusCode = %d, want 200", resp.statusCode)
	}
	hVal, found, _ := resp.headers.Get(starlark.String("X-Test-Header"))
	if !found || hVal.(starlark.String) != "value123" {
		t.Errorf("expected header value123, got %v", hVal)
	}

	// Read body on-demand via Go
	body, err := resp.getBodyBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello streaming http" {
		t.Errorf("body = %q, want %q", string(body), "hello streaming http")
	}

	// Verify it's cached
	if !resp.bodyCached {
		t.Error("expected body to be cached")
	}
}

func TestClientResponseStreaming(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello streaming http")
	}))
	defer ts.Close()

	rt := testRuntime(t)
	defer rt.Close()

	httpMod := New()
	dict, err := httpMod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"http": dict["http"],
	}

	// Test get_reader, reading partial content, and closing
	script := fmt.Sprintf(`
def test_stream():
    resp = http.url("%s").get()
    reader = resp.get_reader()
    chunk1 = reader.read(5)
    reader.close()
    return chunk1
`, ts.URL)

	thread := rt.NewThread("test-thread")
	globals, err := starlark.ExecFile(thread, "test_stream.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}

	testFn := globals["test_stream"]
	res, err := starlark.Call(thread, testFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	b, ok := res.(starlark.Bytes)
	if !ok {
		t.Fatalf("expected bytes, got %T", res)
	}
	if string(b) != "hello" {
		t.Errorf("got %q, want %q", string(b), "hello")
	}
}
