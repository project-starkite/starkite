package containers

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ParseEndpoint parses an explicit host string into an Endpoint.
func ParseEndpoint(host string) (Endpoint, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return Endpoint{}, fmt.Errorf("containers: host cannot be empty")
	}

	// Windows named pipe
	if after, ok := strings.CutPrefix(host, "npipe://"); ok {
		addr := after
		return Endpoint{
			Scheme:  "npipe",
			Address: addr,
			URL:     "http://localhost",
		}, nil
	}

	// Unix domain socket
	if after, ok := strings.CutPrefix(host, "unix://"); ok {
		addr := after
		return Endpoint{
			Scheme:  "unix",
			Address: addr,
			URL:     "http://localhost",
		}, nil
	}

	// TCP
	if after, ok := strings.CutPrefix(host, "tcp://"); ok {
		addr := after
		return Endpoint{
			Scheme:  "tcp",
			Address: addr,
			URL:     fmt.Sprintf("http://%s", addr),
		}, nil
	}

	// HTTP or HTTPS
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		u, err := url.Parse(host)
		if err != nil {
			return Endpoint{}, fmt.Errorf("containers: invalid host URL: %w", err)
		}
		return Endpoint{
			Scheme:  "tcp",
			Address: u.Host,
			URL:     host,
		}, nil
	}

	// File path (starts with / or . or ~) -> Unix socket
	if strings.HasPrefix(host, "/") || strings.HasPrefix(host, ".") || strings.HasPrefix(host, "~") {
		expanded := expandHome(host)
		return Endpoint{
			Scheme:  "unix",
			Address: expanded,
			URL:     "http://localhost",
		}, nil
	}

	// Host:Port
	if strings.Contains(host, ":") {
		return Endpoint{
			Scheme:  "tcp",
			Address: host,
			URL:     fmt.Sprintf("http://%s", host),
		}, nil
	}

	return Endpoint{}, fmt.Errorf("containers: unrecognized host endpoint format: %q", host)
}

// DiscoverEndpoint discovers the target daemon socket:
// 1. Explicit override if provided.
// 2. DOCKER_HOST environment variable if set.
// 3. Probes candidate sockets that exist on disk.
// 4. Default socket for the platform.
func DiscoverEndpoint(override string) (Endpoint, error) {
	if override != "" {
		return ParseEndpoint(override)
	}

	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
		return ParseEndpoint(dockerHost)
	}

	// Probe candidate sockets in order of priority
	for _, candidate := range candidateSockets() {
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return Endpoint{
				Scheme:  "unix",
				Address: candidate,
				URL:     "http://localhost",
			}, nil
		}
	}

	// Fallback to platform default
	return defaultEndpoint(), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// CandidateSockets returns the list of candidate socket paths probed by auto-discovery on this platform.
func CandidateSockets() []string {
	return candidateSockets()
}

func candidateSockets() []string {
	var candidates []string

	home, _ := os.UserHomeDir()
	uid := os.Getuid()

	if runtime.GOOS == "linux" {
		if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "podman/podman.sock"))
		}
		if uid >= 0 {
			candidates = append(candidates,
				fmt.Sprintf("/run/user/%d/podman/podman.sock", uid),
				fmt.Sprintf("/run/user/%d/docker.sock", uid),
			)
		}
		candidates = append(candidates,
			"/run/podman/podman.sock",
			"/var/run/docker.sock",
		)
	} else if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/var/run/docker.sock",
		)
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".docker/run/docker.sock"),
				filepath.Join(home, ".local/share/containers/podman/machine/podman.sock"),
			)
		}
	}

	return candidates
}

func defaultEndpoint() Endpoint {
	if runtime.GOOS == "windows" {
		return Endpoint{
			Scheme:  "npipe",
			Address: `\\.\pipe\docker_engine`,
			URL:     "http://localhost",
		}
	}
	return Endpoint{
		Scheme:  "unix",
		Address: "/var/run/docker.sock",
		URL:     "http://localhost",
	}
}
