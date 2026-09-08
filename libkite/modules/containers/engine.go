package containers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// CreateContainer creates a container on the daemon.
// Endpoint: POST /{version}/containers/create?name={name}
func (c *EngineClient) CreateContainer(ctx context.Context, name string, config *ContainerConfig) (*CreateContainerResponse, error) {
	bodyBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("containers: marshal create config: %w", err)
	}

	query := url.Values{}
	if name != "" {
		query.Set("name", name)
	}

	resp, err := c.do(ctx, http.MethodPost, "/containers/create", query, bytes.NewReader(bodyBytes), "application/json")
	if err != nil {
		return nil, fmt.Errorf("containers: create request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return nil, fmt.Errorf("containers: create: %w", err)
	}

	var createResp CreateContainerResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("containers: decode create response: %w", err)
	}
	return &createResp, nil
}

// StartContainer starts an existing container.
// Endpoint: POST /{version}/containers/{id}/start
func (c *EngineClient) StartContainer(ctx context.Context, id string) error {
	path := fmt.Sprintf("/containers/%s/start", url.PathEscape(id))
	resp, err := c.do(ctx, http.MethodPost, path, nil, nil, "")
	if err != nil {
		return fmt.Errorf("containers: start request: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content = OK, 304 Not Modified = already started
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if err := c.checkError(resp); err != nil {
		return fmt.Errorf("containers: start: %w", err)
	}
	return nil
}

// StopContainer stops a running container.
// Endpoint: POST /{version}/containers/{id}/stop?t={timeout}
func (c *EngineClient) StopContainer(ctx context.Context, id string, timeoutSeconds int) error {
	path := fmt.Sprintf("/containers/%s/stop", url.PathEscape(id))
	query := url.Values{}
	if timeoutSeconds > 0 {
		query.Set("t", strconv.Itoa(timeoutSeconds))
	}

	resp, err := c.do(ctx, http.MethodPost, path, query, nil, "")
	if err != nil {
		return fmt.Errorf("containers: stop request: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content = OK, 304 Not Modified = already stopped
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if err := c.checkError(resp); err != nil {
		return fmt.Errorf("containers: stop: %w", err)
	}
	return nil
}

// RestartContainer restarts a container.
// Endpoint: POST /{version}/containers/{id}/restart?t={timeout}
func (c *EngineClient) RestartContainer(ctx context.Context, id string, timeoutSeconds int) error {
	path := fmt.Sprintf("/containers/%s/restart", url.PathEscape(id))
	query := url.Values{}
	if timeoutSeconds > 0 {
		query.Set("t", strconv.Itoa(timeoutSeconds))
	}

	resp, err := c.do(ctx, http.MethodPost, path, query, nil, "")
	if err != nil {
		return fmt.Errorf("containers: restart request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return fmt.Errorf("containers: restart: %w", err)
	}
	return nil
}

// WaitContainer blocks until a container stops or reaches a condition.
// Endpoint: POST /{version}/containers/{id}/wait?condition={condition}
func (c *EngineClient) WaitContainer(ctx context.Context, id string, condition string) (int, error) {
	path := fmt.Sprintf("/containers/%s/wait", url.PathEscape(id))
	query := url.Values{}
	if condition != "" {
		query.Set("condition", condition)
	}

	resp, err := c.do(ctx, http.MethodPost, path, query, nil, "")
	if err != nil {
		return -1, fmt.Errorf("containers: wait request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return -1, fmt.Errorf("containers: wait: %w", err)
	}

	var waitResp ContainerWaitResponse
	if err := json.NewDecoder(resp.Body).Decode(&waitResp); err != nil {
		return -1, fmt.Errorf("containers: decode wait response: %w", err)
	}

	if waitResp.Error != nil && waitResp.Error.Message != "" {
		return waitResp.StatusCode, fmt.Errorf("containers: wait container error: %s", waitResp.Error.Message)
	}
	return waitResp.StatusCode, nil
}

// RemoveContainer removes a container from the daemon.
// Endpoint: DELETE /{version}/containers/{id}?force={force}&v={volumes}
func (c *EngineClient) RemoveContainer(ctx context.Context, id string, force, volumes bool) error {
	path := fmt.Sprintf("/containers/%s", url.PathEscape(id))
	query := url.Values{}
	if force {
		query.Set("force", "true")
	}
	if volumes {
		query.Set("v", "true")
	}

	resp, err := c.do(ctx, http.MethodDelete, path, query, nil, "")
	if err != nil {
		return fmt.Errorf("containers: remove request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return fmt.Errorf("containers: remove: %w", err)
	}
	return nil
}

// InspectContainer retrieves detailed state and config of a container.
// Endpoint: GET /{version}/containers/{id}/json
func (c *EngineClient) InspectContainer(ctx context.Context, id string) (map[string]any, error) {
	path := fmt.Sprintf("/containers/%s/json", url.PathEscape(id))
	resp, err := c.do(ctx, http.MethodGet, path, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("containers: inspect request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return nil, fmt.Errorf("containers: inspect: %w", err)
	}

	var inspectResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&inspectResp); err != nil {
		return nil, fmt.Errorf("containers: decode inspect response: %w", err)
	}
	return inspectResp, nil
}

// ListContainers queries containers on the daemon.
// Endpoint: GET /{version}/containers/json?all={all}
func (c *EngineClient) ListContainers(ctx context.Context, all bool) ([]ContainerSummary, error) {
	query := url.Values{}
	if all {
		query.Set("all", "true")
	}

	resp, err := c.do(ctx, http.MethodGet, "/containers/json", query, nil, "")
	if err != nil {
		return nil, fmt.Errorf("containers: list request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return nil, fmt.Errorf("containers: list: %w", err)
	}

	var summaries []ContainerSummary
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		return nil, fmt.Errorf("containers: decode list response: %w", err)
	}
	return summaries, nil
}

// CreateExec creates an exec instance inside a container.
// Endpoint: POST /{version}/containers/{id}/exec
func (c *EngineClient) CreateExec(ctx context.Context, id string, cfg ExecConfig) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("containers: marshal exec config: %w", err)
	}

	path := fmt.Sprintf("/containers/%s/exec", url.PathEscape(id))
	resp, err := c.do(ctx, http.MethodPost, path, nil, bytes.NewReader(data), "application/json")
	if err != nil {
		return "", fmt.Errorf("containers: create exec request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return "", fmt.Errorf("containers: create exec: %w", err)
	}

	var createResp ExecCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("containers: decode create exec response: %w", err)
	}
	return createResp.ID, nil
}

// StartExec starts an exec instance, demultiplexing stdout and stderr.
// Endpoint: POST /{version}/exec/{id}/start
func (c *EngineClient) StartExec(ctx context.Context, execID string, stdout, stderr io.Writer) error {
	body := strings.NewReader(`{"Detach":false,"Tty":false}`)
	path := fmt.Sprintf("/exec/%s/start", url.PathEscape(execID))

	resp, err := c.do(ctx, http.MethodPost, path, nil, body, "application/json")
	if err != nil {
		return fmt.Errorf("containers: start exec request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return fmt.Errorf("containers: start exec: %w", err)
	}

	return DemuxStream(resp.Body, stdout, stderr)
}

// InspectExec checks the status and exit code of an exec instance.
// Endpoint: GET /{version}/exec/{id}/json
func (c *EngineClient) InspectExec(ctx context.Context, execID string) (ExecInspectResponse, error) {
	path := fmt.Sprintf("/exec/%s/json", url.PathEscape(execID))

	resp, err := c.do(ctx, http.MethodGet, path, nil, nil, "")
	if err != nil {
		return ExecInspectResponse{}, fmt.Errorf("containers: inspect exec request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return ExecInspectResponse{}, fmt.Errorf("containers: inspect exec: %w", err)
	}

	var inspectResp ExecInspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&inspectResp); err != nil {
		return ExecInspectResponse{}, fmt.Errorf("containers: decode inspect exec response: %w", err)
	}
	return inspectResp, nil
}

// Exec is a composite method that creates, runs, and inspects an exec command to completion.
func (c *EngineClient) Exec(ctx context.Context, containerID string, cfg ExecConfig) (*ExecResult, error) {
	execID, err := c.CreateExec(ctx, containerID, cfg)
	if err != nil {
		return nil, err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := c.StartExec(ctx, execID, &stdoutBuf, &stderrBuf); err != nil {
		return nil, err
	}

	inspect, err := c.InspectExec(ctx, execID)
	if err != nil {
		return nil, err
	}

	return &ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}, nil
}

// Logs streams container logs via an io.ReadCloser.
// Endpoint: GET /{version}/containers/{id}/logs
func (c *EngineClient) Logs(ctx context.Context, containerID string, opts LogsOptions) (io.ReadCloser, error) {
	path := fmt.Sprintf("/containers/%s/logs", url.PathEscape(containerID))
	query := url.Values{}
	if opts.Stdout {
		query.Set("stdout", "true")
	}
	if opts.Stderr {
		query.Set("stderr", "true")
	}
	if opts.Follow {
		query.Set("follow", "true")
	}
	if opts.Tail != "" {
		query.Set("tail", opts.Tail)
	} else {
		query.Set("tail", "all")
	}
	if opts.Timestamps {
		query.Set("timestamps", "true")
	}

	resp, err := c.do(ctx, http.MethodGet, path, query, nil, "")
	if err != nil {
		return nil, fmt.Errorf("containers: logs request: %w", err)
	}
	if err := c.checkError(resp); err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("containers: logs: %w", err)
	}

	if !opts.Follow {
		// Read and demux non-streaming logs completely
		defer resp.Body.Close()
		var buf bytes.Buffer
		var stdoutDest, stderrDest io.Writer
		if opts.Stdout {
			stdoutDest = &buf
		}
		if opts.Stderr {
			stderrDest = &buf
		}
		if err := DemuxStream(resp.Body, stdoutDest, stderrDest); err != nil {
			return nil, fmt.Errorf("containers: demux logs: %w", err)
		}
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}

	// For follow=true, demux asynchronously using an io.Pipe
	pr, pw := io.Pipe()
	go func() {
		defer resp.Body.Close()
		var stdoutDest, stderrDest io.Writer
		if opts.Stdout {
			stdoutDest = pw
		}
		if opts.Stderr {
			stderrDest = pw
		}
		err := DemuxStream(resp.Body, stdoutDest, stderrDest)
		_ = pw.CloseWithError(err)
	}()

	return pr, nil
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
