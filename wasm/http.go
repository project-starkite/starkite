package wasm

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// newHTTPRequest creates an *http.Request from the given parameters.
func newHTTPRequest(ctx context.Context, method, url string, headers map[string]string, body string) (*http.Request, error) {
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// doHTTPRequest executes an HTTP request and returns a result map.
func doHTTPRequest(req *http.Request) (map[string]any, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{
			"status":  0,
			"body":    "",
			"headers": map[string]string{},
			"error":   err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return map[string]any{
			"status":  resp.StatusCode,
			"body":    "",
			"headers": map[string]string{},
			"error":   err.Error(),
		}, nil
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	return map[string]any{
		"status":  resp.StatusCode,
		"body":    string(respBody),
		"headers": respHeaders,
		"error":   "",
	}, nil
}
