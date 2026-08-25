package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
)

// Response is a Starlark value representing an HTTP response.
type Response struct {
	statusCode int
	status     string
	headers    *starlark.Dict

	// Streaming support
	rawResp      *http.Response
	mu           sync.Mutex
	bodyBytes    []byte
	bodyCached   bool
	readerOpened bool
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
			return libkite.TryWrap("http.response."+name, method), nil
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
		val, err := r.getBodyBytes()
		if err != nil {
			return nil, err
		}
		return starlark.Bytes(val), nil
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
	case "get_reader":
		return starlark.NewBuiltin("http.response.get_reader", r.getReaderMethod)
	}
	return nil
}

func (r *Response) AttrNames() []string {
	names := []string{
		"body", "get_bytes", "get_reader", "get_text", "headers", "status", "status_code",
		"try_get_bytes", "try_get_reader", "try_get_text",
	}
	sort.Strings(names)
	return names
}

func (r *Response) getBodyBytes() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.bodyCached {
		return r.bodyBytes, nil
	}
	if r.readerOpened {
		return nil, fmt.Errorf("http.response: stream reader was already opened")
	}
	if r.rawResp == nil || r.rawResp.Body == nil {
		return nil, fmt.Errorf("http.response: response body is not available")
	}

	defer r.rawResp.Body.Close()
	data, err := io.ReadAll(r.rawResp.Body)
	if err != nil {
		return nil, fmt.Errorf("http.response: failed to read body: %w", err)
	}
	r.bodyBytes = data
	r.bodyCached = true
	return r.bodyBytes, nil
}

func (r *Response) getTextMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("http.response.get_text: takes no arguments")
	}
	data, err := r.getBodyBytes()
	if err != nil {
		return nil, err
	}
	return starlark.String(string(data)), nil
}

func (r *Response) getBytesMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("http.response.get_bytes: takes no arguments")
	}
	data, err := r.getBodyBytes()
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(data), nil
}

func (r *Response) getReaderMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("http.response.get_reader: takes no arguments")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.bodyCached {
		return iomod.NewReader(bytes.NewReader(r.bodyBytes), "http-response-cached"), nil
	}
	if r.readerOpened {
		return nil, fmt.Errorf("http.response.get_reader: stream reader was already opened")
	}
	if r.rawResp == nil || r.rawResp.Body == nil {
		return nil, fmt.Errorf("http.response.get_reader: response body is not available")
	}

	r.readerOpened = true
	return iomod.NewReader(r.rawResp.Body, "http-response"), nil
}

func (r *Response) closeBody() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.bodyCached && !r.readerOpened && r.rawResp != nil && r.rawResp.Body != nil {
		r.rawResp.Body.Close()
	}
}

func newResponse(resp *http.Response) *Response {
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
	r := &Response{
		statusCode: resp.StatusCode,
		status:     resp.Status,
		headers:    headersDict,
		rawResp:    resp,
	}
	runtime.SetFinalizer(r, func(x *Response) {
		x.closeBody()
	})
	return r
}

func newDryRunResponse(method, url string) *Response {
	body := fmt.Sprintf("[DRY RUN] Would %s %s", method, url)
	return &Response{
		statusCode: 200,
		status:     "200 OK",
		bodyBytes:  []byte(body),
		bodyCached: true,
		headers:    starlark.NewDict(0),
	}
}
