//go:build windows

package containers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func newPlatformClient(ep Endpoint, timeout time.Duration) (*http.Client, error) {
	if ep.Scheme == "npipe" {
		return nil, fmt.Errorf("containers: named pipe connections on Windows require TCP endpoint or Unix socket")
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, ep.Scheme, ep.Address)
		},
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
