//go:build windows

package containers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

type winPipeConn struct {
	handle       windows.Handle
	mu           sync.Mutex
	readTimer    *time.Timer
	writeTimer   *time.Timer
	readExpired  atomic.Bool
	writeExpired atomic.Bool
	closed       atomic.Bool
}

func (c *winPipeConn) Read(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	if c.readExpired.Load() {
		return 0, os.ErrDeadlineExceeded
	}

	var done uint32
	err := windows.ReadFile(c.handle, b, &done, nil)
	if err != nil {
		if c.readExpired.Load() {
			return int(done), os.ErrDeadlineExceeded
		}
		if c.closed.Load() {
			return int(done), net.ErrClosed
		}
		if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_PIPE_NOT_CONNECTED {
			return int(done), io.EOF
		}
		if err == windows.ERROR_OPERATION_ABORTED {
			if c.readExpired.Load() {
				return int(done), os.ErrDeadlineExceeded
			}
			return int(done), net.ErrClosed
		}
		return int(done), err
	}
	return int(done), nil
}

func (c *winPipeConn) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	if c.writeExpired.Load() {
		return 0, os.ErrDeadlineExceeded
	}

	var done uint32
	err := windows.WriteFile(c.handle, b, &done, nil)
	if err != nil {
		if c.writeExpired.Load() {
			return int(done), os.ErrDeadlineExceeded
		}
		if c.closed.Load() {
			return int(done), net.ErrClosed
		}
		if err == windows.ERROR_OPERATION_ABORTED {
			if c.writeExpired.Load() {
				return int(done), os.ErrDeadlineExceeded
			}
			return int(done), net.ErrClosed
		}
		return int(done), err
	}
	return int(done), err
}

func (c *winPipeConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	c.mu.Lock()
	if c.readTimer != nil {
		c.readTimer.Stop()
		c.readTimer = nil
	}
	if c.writeTimer != nil {
		c.writeTimer.Stop()
		c.writeTimer = nil
	}
	c.mu.Unlock()

	_ = windows.CancelIoEx(c.handle, nil)
	return windows.CloseHandle(c.handle)
}

func (c *winPipeConn) LocalAddr() net.Addr  { return &pipeAddr{addr: "pipe"} }
func (c *winPipeConn) RemoteAddr() net.Addr { return &pipeAddr{addr: "pipe"} }

func (c *winPipeConn) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	return c.SetWriteDeadline(t)
}

func (c *winPipeConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.readTimer != nil {
		c.readTimer.Stop()
		c.readTimer = nil
	}

	if t.IsZero() {
		c.readExpired.Store(false)
		return nil
	}

	d := time.Until(t)
	if d <= 0 {
		c.readExpired.Store(true)
		_ = windows.CancelIoEx(c.handle, nil)
		return nil
	}

	c.readExpired.Store(false)
	c.readTimer = time.AfterFunc(d, func() {
		c.readExpired.Store(true)
		_ = windows.CancelIoEx(c.handle, nil)
	})
	return nil
}

func (c *winPipeConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writeTimer != nil {
		c.writeTimer.Stop()
		c.writeTimer = nil
	}

	if t.IsZero() {
		c.writeExpired.Store(false)
		return nil
	}

	d := time.Until(t)
	if d <= 0 {
		c.writeExpired.Store(true)
		_ = windows.CancelIoEx(c.handle, nil)
		return nil
	}

	c.writeExpired.Store(false)
	c.writeTimer = time.AfterFunc(d, func() {
		c.writeExpired.Store(true)
		_ = windows.CancelIoEx(c.handle, nil)
	})
	return nil
}

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
