package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// URL is a Starlark value representing an HTTP URL with method calls.
type URL struct {
	rawURL string
	thread *starlark.Thread
	module *Module
}

var (
	_ starlark.Value    = (*URL)(nil)
	_ starlark.HasAttrs = (*URL)(nil)
)

func (u *URL) String() string        { return fmt.Sprintf("http.url(%q)", u.rawURL) }
func (u *URL) Type() string          { return "http.url" }
func (u *URL) Freeze()               {} // immutable
func (u *URL) Truth() starlark.Bool  { return starlark.Bool(u.rawURL != "") }
func (u *URL) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: http.url") }

func (u *URL) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := u.methodBuiltin(base); method != nil {
			return libkite.TryWrap("http.url."+name, method), nil
		}
		return nil, nil
	}

	// Methods
	if method := u.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (u *URL) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "get":
		return starlark.NewBuiltin("http.url.get", u.getMethod)
	case "post":
		return starlark.NewBuiltin("http.url.post", u.postMethod)
	case "put":
		return starlark.NewBuiltin("http.url.put", u.putMethod)
	case "patch":
		return starlark.NewBuiltin("http.url.patch", u.patchMethod)
	case "delete":
		return starlark.NewBuiltin("http.url.delete", u.deleteMethod)
	}
	return nil
}

func (u *URL) AttrNames() []string {
	names := []string{
		"delete", "get", "patch", "post", "put",
		"try_delete", "try_get", "try_patch", "try_post", "try_put",
	}
	sort.Strings(names)
	return names
}

func (u *URL) getMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return u.doRequest("GET", false, args, kwargs)
}

func (u *URL) postMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return u.doRequest("POST", true, args, kwargs)
}

func (u *URL) putMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return u.doRequest("PUT", true, args, kwargs)
}

func (u *URL) patchMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return u.doRequest("PATCH", true, args, kwargs)
}

func (u *URL) deleteMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return u.doRequest("DELETE", false, args, kwargs)
}

func (u *URL) doRequest(method string, hasBody bool, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var body starlark.Value = starlark.None
	var headers *starlark.Dict
	var timeoutStr string

	// Parse positional body arg for POST/PUT/PATCH
	if hasBody && len(args) > 0 {
		body = args[0]
		args = args[1:]
	}

	if len(args) > 0 {
		return nil, fmt.Errorf("http.url.%s: unexpected positional arguments", strings.ToLower(method))
	}

	// Parse kwargs
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "body":
			body = kv[1]
		case "headers":
			if d, ok := kv[1].(*starlark.Dict); ok {
				headers = d
			}
		case "timeout":
			if s, ok := starlark.AsString(kv[1]); ok {
				timeoutStr = s
			} else {
				return nil, fmt.Errorf("http.url.%s: timeout must be a string (e.g. \"5s\"), got %s", strings.ToLower(method), kv[1].Type())
			}
		}
	}

	// Permission check
	if err := libkite.Check(u.thread, "http", "client", method, u.rawURL); err != nil {
		return nil, err
	}

	// Dry run check
	if u.module.config != nil && u.module.config.DryRun {
		return newDryRunResponse(method, u.rawURL), nil
	}

	// Build request body
	var reqBody io.Reader
	var isDict bool
	if body != starlark.None {
		switch v := body.(type) {
		case starlark.String:
			reqBody = bytes.NewBufferString(string(v))
		case *starlark.Dict:
			isDict = true
			var goVal map[string]any
			if err := startype.Starlark(v).Go(&goVal); err != nil {
				return nil, err
			}
			sanitizeMapKeys(goVal)
			jsonData, err := json.Marshal(goVal)
			if err != nil {
				return nil, err
			}
			reqBody = bytes.NewBuffer(jsonData)
		default:
			reqBody = bytes.NewBufferString(body.String())
		}
	}

	// Create request
	req, err := http.NewRequest(method, u.rawURL, reqBody)
	if err != nil {
		return nil, err
	}

	// Add global headers
	u.module.mu.RLock()
	for k, v := range u.module.headers {
		req.Header.Set(k, v)
	}
	u.module.mu.RUnlock()

	// Add request-specific headers
	if headers != nil {
		for _, item := range headers.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					req.Header.Set(k, v)
				}
			}
		}
	}

	// Set content-type for JSON if body is a dict
	if isDict {
		req.Header.Set("Content-Type", "application/json")
	}

	// Create client with timeout override if specified
	client := u.module.client
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("http.url.%s: invalid timeout %q: %w", strings.ToLower(method), timeoutStr, err)
		}
		client = &http.Client{Timeout: d}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return newResponse(resp, respBody), nil
}
