//go:build !windows

package containers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func newPlatformClient(ep Endpoint, timeout time.Duration) (*http.Client, error) {
	if ep.Scheme != "unix" && ep.Scheme != "tcp" {
		return nil, fmt.Errorf("containers: unsupported scheme %q on this platform", ep.Scheme)
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
