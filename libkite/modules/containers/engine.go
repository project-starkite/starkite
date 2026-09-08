package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EngineClient provides raw HTTP REST interactions with a Docker or Podman daemon.
type EngineClient struct {
	httpClient *http.Client
	endpoint   Endpoint
	version    string
}

// NewEngineClient creates a new EngineClient for the given endpoint.
func NewEngineClient(ep Endpoint, timeout time.Duration) (*EngineClient, error) {
	cli, err := newHTTPClient(ep, timeout)
	if err != nil {
		return nil, err
	}
	return &EngineClient{
		httpClient: cli,
		endpoint:   ep,
		version:    DefaultAPIVersion,
	}, nil
}

// Endpoint returns the configured endpoint.
func (c *EngineClient) Endpoint() Endpoint {
	return c.endpoint
}

// VersionTag returns the API version in use.
func (c *EngineClient) VersionTag() string {
	return c.version
}

// Ping checks if the container daemon is responsive.
// Endpoint: GET /_ping
func (c *EngineClient) Ping(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint.URL+"/_ping", nil)
	if err != nil {
		return false, fmt.Errorf("containers: ping request: %w", err)
	}
	req.Host = "localhost"

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("containers: ping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("containers: ping returned status %d", resp.StatusCode)
	}
	return true, nil
}

// Version queries the daemon system version information.
// Endpoint: GET /{version}/version
func (c *EngineClient) Version(ctx context.Context) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, "/version", nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("containers: version: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return nil, fmt.Errorf("containers: version: %w", err)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("containers: decode version: %w", err)
	}
	return data, nil
}

// do executes an HTTP request against the engine API.
func (c *EngineClient) do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	fullPath := "/" + c.version + path
	if !strings.HasPrefix(path, "/") {
		fullPath = "/" + c.version + "/" + path
	}

	reqURL := c.endpoint.URL + fullPath
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.Host = "localhost"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.httpClient.Do(req)
}

// checkError checks the response status code and returns a formatted error if >= 400.
func (c *EngineClient) checkError(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var errResp struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Message != "" {
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, errResp.Message)
	}

	trimmed := strings.TrimSpace(string(bodyBytes))
	if trimmed != "" {
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, trimmed)
	}
	return fmt.Errorf("daemon error (%d %s)", resp.StatusCode, http.StatusText(resp.StatusCode))
}
