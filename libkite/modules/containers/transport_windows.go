//go:build windows

package containers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/sys/windows"
)

type winPipeConn struct {
	handle windows.Handle
}

func (c *winPipeConn) Read(b []byte) (int, error) {
	var done uint32
	err := windows.ReadFile(c.handle, b, &done, nil)
	if err != nil {
		if err == windows.ERROR_BROKEN_PIPE {
			return int(done), io.EOF
		}
		return int(done), err
	}
	return int(done), nil
}

func (c *winPipeConn) Write(b []byte) (int, error) {
	var done uint32
	err := windows.WriteFile(c.handle, b, &done, nil)
	return int(done), err
}

func (c *winPipeConn) Close() error {
	return windows.CloseHandle(c.handle)
}

func (c *winPipeConn) LocalAddr() net.Addr                { return &pipeAddr{addr: "pipe"} }
func (c *winPipeConn) RemoteAddr() net.Addr               { return &pipeAddr{addr: "pipe"} }
func (c *winPipeConn) SetDeadline(t time.Time) error      { return nil }
func (c *winPipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *winPipeConn) SetWriteDeadline(t time.Time) error { return nil }

type pipeAddr struct {
	addr string
}

func (a *pipeAddr) Network() string { return "pipe" }
func (a *pipeAddr) String() string  { return a.addr }

func dialPipe(pipePath string) (net.Conn, error) {
	path16, err := windows.UTF16PtrFromString(pipePath)
	if err != nil {
		return nil, fmt.Errorf("containers: invalid pipe path %q: %w", pipePath, err)
	}

	handle, err := windows.CreateFile(
		path16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("containers: dial named pipe %q: %w", pipePath, err)
	}

	return &winPipeConn{handle: handle}, nil
}

func newPlatformClient(ep Endpoint, timeout time.Duration) (*http.Client, error) {
	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	switch ep.Scheme {
	case "npipe":
		dialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialPipe(ep.Address)
		}
	case "tcp", "unix":
		dialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, ep.Scheme, ep.Address)
		}
	default:
		return nil, fmt.Errorf("containers: unsupported scheme %q on Windows", ep.Scheme)
	}

	transport := &http.Transport{
		DialContext:       dialContext,
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
