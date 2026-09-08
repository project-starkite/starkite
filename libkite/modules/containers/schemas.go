package containers

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PortBinding represents a host port mapping.
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort"`
}

// ContainerConfig specifies the configuration for creating a container.
type ContainerConfig struct {
	Image        string              `json:"Image"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	HostConfig   HostConfig          `json:"HostConfig"`
}

// HostConfig specifies container host-level parameters.
type HostConfig struct {
	Binds        []string                 `json:"Binds,omitempty"`
	PortBindings map[string][]PortBinding `json:"PortBindings,omitempty"`
	NetworkMode  string                   `json:"NetworkMode,omitempty"`
	AutoRemove   bool                     `json:"AutoRemove,omitempty"`
	NanoCPUs     int64                    `json:"NanoCPUs,omitempty"`
	Memory       int64                    `json:"Memory,omitempty"`
}

// CreateContainerResponse is the response payload from POST /containers/create.
type CreateContainerResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// ContainerWaitResponse is the response payload from POST /containers/{id}/wait.
type ContainerWaitResponse struct {
	StatusCode int `json:"StatusCode"`
	Error      *struct {
		Message string `json:"Message"`
	} `json:"Error"`
}

// ContainerInspectResponse is the response from GET /containers/{id}/json.
type ContainerInspectResponse struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string   `json:"Image"`
		Cmd   []string `json:"Cmd"`
		Env   []string `json:"Env"`
	} `json:"Config"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		Paused     bool   `json:"Paused"`
		Restarting bool   `json:"Restarting"`
		OOMKilled  bool   `json:"OOMKilled"`
		Dead       bool   `json:"Dead"`
		Pid        int    `json:"Pid"`
		ExitCode   int    `json:"ExitCode"`
		Error      string `json:"Error"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]PortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

// ContainerSummary is an item in the response from GET /containers/json.
type ContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
}

// ParseMemoryBytes parses a memory limit representation (e.g. "512m", "1g", or integer) into bytes.
func ParseMemoryBytes(val any) (int64, error) {
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "" {
			return 0, nil
		}
		// Split numeric part and suffix
		idx := strings.IndexFunc(s, func(r rune) bool {
			return !unicode.IsDigit(r) && r != '.'
		})
		if idx == -1 {
			return strconv.ParseInt(s, 10, 64)
		}
		numStr := s[:idx]
		unit := strings.TrimSpace(s[idx:])

		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid memory number %q: %w", numStr, err)
		}

		var multiplier float64
		switch unit {
		case "b", "bytes":
			multiplier = 1
		case "k", "kb", "kib":
			multiplier = 1024
		case "m", "mb", "mib":
			multiplier = 1024 * 1024
		case "g", "gb", "gib":
			multiplier = 1024 * 1024 * 1024
		case "t", "tb", "tib":
			multiplier = 1024 * 1024 * 1024 * 1024
		default:
			return 0, fmt.Errorf("unrecognized memory unit %q in %q", unit, v)
		}
		return int64(num * multiplier), nil
	default:
		return 0, fmt.Errorf("memory must be an int or string, got %T", val)
	}
}

// ParseNanoCPUs parses a CPU quota (e.g. 0.5, 1, or "1.5") into NanoCPUs.
func ParseNanoCPUs(val any) (int64, error) {
	switch v := val.(type) {
	case int:
		return int64(v) * 1_000_000_000, nil
	case int64:
		return v * 1_000_000_000, nil
	case float64:
		return int64(v * 1e9), nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid cpu quota %q: %w", s, err)
		}
		return int64(f * 1e9), nil
	default:
		return 0, fmt.Errorf("cpu must be a float, int, or string, got %T", val)
	}
}

// NormalizePortKey ensures a container port has a protocol suffix (e.g. "80" -> "80/tcp").
func NormalizePortKey(port string) string {
	port = strings.TrimSpace(port)
	if strings.Contains(port, "/") {
		return port
	}
	return port + "/tcp"
}
