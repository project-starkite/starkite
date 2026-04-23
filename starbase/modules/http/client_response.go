package http

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// Response is a Starlark value representing an HTTP response.
type Response struct {
	statusCode int
	status     string
	bodyBytes  []byte
	headers    *starlark.Dict
}

var (
	_ starlark.Value    = (*Response)(nil)
	_ starlark.HasAttrs = (*Response)(nil)
)

func (r *Response) String() string        { return fmt.Sprintf("http.response(%d)", r.statusCode) }
func (r *Response) Type() string          { return "http.response" }
func (r *Response) Truth() starlark.Bool  { return starlark.True }
func (r *Response) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: http.response") }
func (r *Response) Freeze() {
	r.headers.Freeze()
}

func (r *Response) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := r.methodBuiltin(base); method != nil {
			return starbase.TryWrap("http.response."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	switch name {
	case "status_code":
		return starlark.MakeInt(r.statusCode), nil
	case "status":
		return starlark.String(r.status), nil
	case "body":
		return starlark.Bytes(r.bodyBytes), nil
	case "headers":
		return r.headers, nil
	}

	// Methods
	if method := r.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (r *Response) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "get_text":
		return starlark.NewBuiltin("http.response.get_text", r.getTextMethod)
	case "get_bytes":
		return starlark.NewBuiltin("http.response.get_bytes", r.getBytesMethod)
	}
	return nil
}

func (r *Response) AttrNames() []string {
	names := []string{
		"body", "get_bytes", "get_text", "headers", "status", "status_code",
		"try_get_bytes", "try_get_text",
	}
	sort.Strings(names)
	return names
}

func (r *Response) getTextMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("http.response.get_text: takes no arguments")
	}
	return starlark.String(string(r.bodyBytes)), nil
}

func (r *Response) getBytesMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("http.response.get_bytes: takes no arguments")
	}
	return starlark.Bytes(r.bodyBytes), nil
}

func newResponse(resp *http.Response, bodyBytes []byte) *Response {
	headersDict := starlark.NewDict(len(resp.Header))
	for k, v := range resp.Header {
		if len(v) == 1 {
			headersDict.SetKey(starlark.String(k), starlark.String(v[0]))
		} else {
			elems := make([]starlark.Value, len(v))
			for i, hv := range v {
				elems[i] = starlark.String(hv)
			}
			headersDict.SetKey(starlark.String(k), starlark.NewList(elems))
		}
	}
	return &Response{
		statusCode: resp.StatusCode,
		status:     resp.Status,
		bodyBytes:  bodyBytes,
		headers:    headersDict,
	}
}

func newDryRunResponse(method, url string) *Response {
	body := fmt.Sprintf("[DRY RUN] Would %s %s", method, url)
	return &Response{
		statusCode: 200,
		status:     "200 OK",
		bodyBytes:  []byte(body),
		headers:    starlark.NewDict(0),
	}
}
