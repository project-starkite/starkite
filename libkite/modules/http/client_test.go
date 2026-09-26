package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPTopLevelDryRun(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	httpMod := New()
	dict, err := httpMod.Load(&libkite.ModuleConfig{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"http": dict["http"],
	}

	script := `
def test_dry():
    r1 = http.get("https://example.com/dry-get")
    r2 = http.post("https://example.com/dry-post", body="hello")
    r3 = http.delete("https://example.com/dry-del")
    return (r1.status_code, r2.status_code, r3.status_code)
`
	thread := rt.NewThread("test-dry")
	globals, err := starlark.ExecFile(thread, "test_dry.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}
	res, err := starlark.Call(thread, globals["test_dry"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tup := res.(starlark.Tuple)
	if tup[0].(starlark.Int).BigInt().Int64() != 200 ||
		tup[1].(starlark.Int).BigInt().Int64() != 200 ||
		tup[2].(starlark.Int).BigInt().Int64() != 200 {
		t.Errorf("expected 200 for dry run responses, got %v", res)
	}
}

func TestHTTPTopLevelConvenienceMethods(t *testing.T) {
	var lastMethod string
	var lastPath string
	var lastBody string
	var lastAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		lastPath = r.URL.Path
		lastAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
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

	// 1. Test http.get
	script := fmt.Sprintf(`
def test_get():
    resp = http.get("%s/test-get", headers={"Authorization": "Bearer secret"})
    return resp.status_code
`, ts.URL)
	thread := rt.NewThread("test-thread")
	globals, err := starlark.ExecFile(thread, "test_get.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}
	res, err := starlark.Call(thread, globals["test_get"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.(starlark.Int).BigInt().Int64() != 200 {
		t.Errorf("status = %v, want 200", res)
	}
	if lastMethod != "GET" || lastPath != "/test-get" || lastAuth != "Bearer secret" {
		t.Errorf("unexpected request details: %s %s %s", lastMethod, lastPath, lastAuth)
	}

	// 2. Test http.post
	script = fmt.Sprintf(`
def test_post():
    resp = http.post("%s/test-post", body={"message": "hello"})
    return resp.status_code
`, ts.URL)
	globals, err = starlark.ExecFile(thread, "test_post.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}
	res, err = starlark.Call(thread, globals["test_post"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.(starlark.Int).BigInt().Int64() != 200 {
		t.Errorf("status = %v, want 200", res)
	}
	if lastMethod != "POST" || lastPath != "/test-post" || !strings.Contains(lastBody, "hello") {
		t.Errorf("unexpected post details: %s %s %s", lastMethod, lastPath, lastBody)
	}

	// 3. Test http.try_get on valid url
	script = fmt.Sprintf(`
def test_try_get():
    res = http.try_get("%s/try-endpoint")
    return (res.ok, res.value.status_code)
`, ts.URL)
	globals, err = starlark.ExecFile(thread, "test_try_get.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}
	res, err = starlark.Call(thread, globals["test_try_get"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tup := res.(starlark.Tuple)
	if bool(tup[0].(starlark.Bool)) != true {
		t.Errorf("expected ok=True")
	}

	// 4. Test http.try_get on failing/invalid address
	script = `
def test_try_fail():
    res = http.try_get("http://127.0.0.1:1/invalid-port", timeout="50ms")
    return res.ok
`
	globals, err = starlark.ExecFile(thread, "test_try_fail.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}
	res, err = starlark.Call(thread, globals["test_try_fail"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bool(res.(starlark.Bool)) != false {
		t.Errorf("expected ok=False for failing network call")
	}
}
