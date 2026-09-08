package containers

import (
	"net/http"
	"time"
)

// newHTTPClient creates an HTTP client configured to connect to the given endpoint.
func newHTTPClient(ep Endpoint, timeout time.Duration) (*http.Client, error) {
	return newPlatformClient(ep, timeout)
}
