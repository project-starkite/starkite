package containers

import (
	"fmt"
	"time"
)

const (
	// DefaultAPIVersion is the default Docker/Podman Engine REST API version.
	DefaultAPIVersion = "v1.45"
	// DefaultTimeout is the default client connection timeout.
	DefaultTimeout = 30 * time.Second
)

// Endpoint represents a parsed container engine connection target.
type Endpoint struct {
	Scheme  string // "unix", "tcp", "npipe"
	Address string // e.g. "/var/run/docker.sock", "127.0.0.1:2375"
	URL     string // Base URL for http.Request, e.g. "http://localhost"
}

func (e Endpoint) String() string {
	if e.Scheme == "unix" {
		return fmt.Sprintf("unix://%s", e.Address)
	}
	if e.Scheme == "npipe" {
		return fmt.Sprintf("npipe://%s", e.Address)
	}
	return e.URL
}
