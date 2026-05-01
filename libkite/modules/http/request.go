package http

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"go.starlark.net/starlark"
)

// paramNameRe matches path parameter names in Go 1.22 ServeMux patterns.
// Matches {name} and {name...} forms.
var paramNameRe = regexp.MustCompile(`\{(\w+?)(?:\.\.\.)?\}`)

// extractParamNames returns the parameter names from a ServeMux pattern.
// e.g. "GET /api/users/{id}" → ["id"]
// e.g. "/static/{filepath...}" → ["filepath"]
func extractParamNames(pattern string) []string {
	matches := paramNameRe.FindAllStringSubmatch(pattern, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// Request is a Starlark value representing an HTTP request with dot-access properties.
type Request struct {
	method     string
	path       string
	url        string
	params     *starlark.Dict
	query      *starlark.Dict
	headers    *starlark.Dict
	body       string
	remoteAddr string
	host       string
}

var (
	_ starlark.Value    = (*Request)(nil)
	_ starlark.HasAttrs = (*Request)(nil)
)

func (r *Request) String() string {
	return fmt.Sprintf("http.request(%s %s)", r.method, r.path)
}
func (r *Request) Type() string         { return "http.request" }
func (r *Request) Truth() starlark.Bool { return starlark.True }
func (r *Request) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: http.request")
}

func (r *Request) Freeze() {
	r.params.Freeze()
	r.query.Freeze()
	r.headers.Freeze()
}

func (r *Request) Attr(name string) (starlark.Value, error) {
	switch name {
	case "method":
		return starlark.String(r.method), nil
	case "path":
		return starlark.String(r.path), nil
	case "url":
		return starlark.String(r.url), nil
	case "params":
		return r.params, nil
	case "query":
		return r.query, nil
	case "headers":
		return r.headers, nil
	case "body":
		return starlark.String(r.body), nil
	case "remote_addr":
		return starlark.String(r.remoteAddr), nil
	case "host":
		return starlark.String(r.host), nil
	}
	return nil, nil
}

func (r *Request) AttrNames() []string {
	names := []string{"body", "headers", "host", "method", "params", "path", "query", "remote_addr", "url"}
	sort.Strings(names)
	return names
}

// buildRequest builds a Request from an http.Request and route pattern.
func buildRequest(r *http.Request, pattern string) *Request {
	// url
	url := r.URL.String()
	if r.URL.Scheme == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		url = scheme + "://" + r.Host + r.URL.RequestURI()
	}

	// params
	params := starlark.NewDict(4)
	for _, name := range extractParamNames(pattern) {
		val := r.PathValue(name)
		if val != "" {
			params.SetKey(starlark.String(name), starlark.String(val))
		}
	}

	// query
	query := starlark.NewDict(len(r.URL.Query()))
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			query.SetKey(starlark.String(k), starlark.String(v[0]))
		} else {
			elems := make([]starlark.Value, len(v))
			for i, val := range v {
				elems[i] = starlark.String(val)
			}
			query.SetKey(starlark.String(k), starlark.NewList(elems))
		}
	}

	// headers
	headers := starlark.NewDict(len(r.Header))
	for k, v := range r.Header {
		headers.SetKey(starlark.String(k), starlark.String(strings.Join(v, ", ")))
	}

	// body
	var body string
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err == nil {
			body = string(data)
		}
	}

	return &Request{
		method:     r.Method,
		path:       r.URL.Path,
		url:        url,
		params:     params,
		query:      query,
		headers:    headers,
		body:       body,
		remoteAddr: r.RemoteAddr,
		host:       r.Host,
	}
}
