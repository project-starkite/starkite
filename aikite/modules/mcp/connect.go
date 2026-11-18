package mcp

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
)

// defaultConnectTimeout caps the initial session handshake (Connect). After
// Connect returns, subsequent calls use their own untimed contexts unless the
// user's script layers one on.
const defaultConnectTimeout = 30 * time.Second

// connectBuiltin is the `mcp.connect(arg, timeout=?)` Starlark builtin.
//
//	arg is a list  → stdio subprocess via CommandTransport
//	arg is a URL   → HTTP via StreamableClientTransport
//	anything else  → error
//
// Returns an *MCPClient on success.
func (m *Module) connectBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mcp.connect: expected 1 positional argument (command list or URL), got %d", len(args))
	}

	var p struct {
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}
	timeout := defaultConnectTimeout
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("mcp.connect: timeout: %w", err)
		}
		timeout = d
	}

	argv, urlStr, err := parseTransportSpec(args[0])
	if err != nil {
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}

	targetDesc := describeTransport(args[0])
	if err := libkite.Check(thread, "mcp", "client", "connect", targetDesc); err != nil {
		return nil, err
	}

	if len(argv) > 0 {
		execTarget := resolveMCPExecTarget(argv[0], m.mcfg)
		if err := libkite.Check(thread, "os", "exec", "exec", execTarget); err != nil {
			return nil, err
		}
	}

	transport, subproc, err := buildTransportWithConfig(thread, argv, urlStr, m.mcfg)
	if err != nil {
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}

	ctx, cancel := context.WithTimeout(libkite.ThreadContext(thread), timeout)
	defer cancel()

	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "starkite", Version: "0.1.0"}, nil)
	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		// Connect may have failed after spawning the subprocess; kill it
		// to avoid leaking the process.
		if subproc != nil && subproc.Process != nil {
			_ = subproc.Process.Kill()
			_ = subproc.Wait()
		}
		return nil, fmt.Errorf("mcp.connect: %w", libkite.WrapOSPermissionError("mcp", "client", targetDesc, err))
	}
	client := newMCPClient(session, subproc)
	client.thread = thread
	libkite.RegisterCleanup(thread, func() {
		_ = client.doClose()
	})
	return client, nil
}

// inferTransport picks an MCP transport based on the Starlark arg's type.
// Returns the transport, the constructed *exec.Cmd (nil for non-stdio), and an
// error for unsupported shapes.
func inferTransport(v starlark.Value) (mcpsdk.Transport, *exec.Cmd, error) {
	argv, urlStr, err := parseTransportSpec(v)
	if err != nil {
		return nil, nil, err
	}
	return buildTransportWithConfig(nil, argv, urlStr, nil)
}

func parseTransportSpec(v starlark.Value) ([]string, string, error) {
	switch vv := v.(type) {
	case *starlark.List:
		argv, err := coerceCommandList(vv)
		if err != nil {
			return nil, "", err
		}
		return argv, "", nil
	case starlark.Tuple:
		argv, err := coerceCommandList(vv)
		if err != nil {
			return nil, "", err
		}
		return argv, "", nil
	case starlark.String:
		rawURL := string(vv)
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			return nil, "", fmt.Errorf("URL must start with http:// or https://, got %q", rawURL)
		}
		u, err := neturl.Parse(rawURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, "", fmt.Errorf("invalid http(s) URL %q", rawURL)
		}
		return nil, rawURL, nil
	default:
		return nil, "", fmt.Errorf("expected a command list or http(s) URL, got %s", v.Type())
	}
}

func buildTransportWithConfig(thread *starlark.Thread, argv []string, urlStr string, cfg *libkite.ModuleConfig) (mcpsdk.Transport, *exec.Cmd, error) {
	if len(argv) > 0 {
		cmdPath := argv[0]
		isolated := cfg != nil && cfg.Env != nil
		if isolated && !strings.ContainsAny(cmdPath, `/\`) && !filepath.IsAbs(cmdPath) {
			pathEnv := lookupMCPPath(cfg.Env)
			if pathEnv == "" {
				return nil, nil, fmt.Errorf("executable %q not found in isolated PATH", cmdPath)
			}
			resolved, err := lookPathInList(cmdPath, pathEnv)
			if err != nil {
				return nil, nil, fmt.Errorf("executable %q not found in isolated PATH: %w", cmdPath, err)
			}
			cmdPath = resolved
		}
		cmd := exec.Command(cmdPath, argv[1:]...)
		if isolated {
			cmd.Env = mapToSortedEnvSlice(cfg.Env)
		} else {
			cmd.Env = mapToSortedEnvSlice(sandbox.SanitizedBaselineEnv(os.Environ()))
		}
		return &mcpsdk.CommandTransport{Command: cmd}, cmd, nil
	}

	var httpClient *http.Client
	if thread != nil {
		httpClient = &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return libkite.Check(thread, "mcp", "client", "connect", req.URL.String())
			},
		}
	}
	return &mcpsdk.StreamableClientTransport{
		Endpoint:   urlStr,
		HTTPClient: httpClient,
	}, nil, nil
}

func resolveMCPExecTarget(bin string, cfg *libkite.ModuleConfig) string {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return bin
	}
	if strings.ContainsAny(bin, `/\`) || filepath.IsAbs(bin) {
		return libkite.CanonicalizePath(bin)
	}
	if cfg != nil && cfg.Env != nil {
		if pathEnv := lookupMCPPath(cfg.Env); pathEnv != "" {
			if resolved, err := lookPathInList(bin, pathEnv); err == nil {
				return libkite.CanonicalizePath(resolved)
			}
		}
		return bin
	}
	if resolved, err := exec.LookPath(bin); err == nil {
		return libkite.CanonicalizePath(resolved)
	}
	return bin
}

func lookupMCPPath(env map[string]string) string {
	if v, ok := env["PATH"]; ok {
		return v
	}
	if runtime.GOOS == "windows" {
		for k, v := range env {
			if strings.EqualFold(k, "PATH") {
				return v
			}
		}
	}
	return ""
}

func lookPathInList(file, pathList string) (string, error) {
	exts := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(file) == "" {
		pathext := os.Getenv("PATHEXT")
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD"
		}
		exts = filepath.SplitList(pathext)
	}
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			dir = "."
		}
		for _, ext := range exts {
			target := filepath.Join(dir, file+ext)
			if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
				if runtime.GOOS == "windows" || fi.Mode()&0111 != 0 {
					return target, nil
				}
			}
		}
	}
	return "", exec.ErrNotFound
}

func mapToSortedEnvSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// coerceCommandList walks a Starlark iterable of strings into []string.
// Rejects empty lists, empty executables, and non-string elements.
func coerceCommandList(v starlark.Value) ([]string, error) {
	iter, ok := v.(starlark.Iterable)
	if !ok {
		return nil, fmt.Errorf("command list: expected iterable, got %s", v.Type())
	}
	it := iter.Iterate()
	defer it.Done()

	var out []string
	var elem starlark.Value
	for it.Next(&elem) {
		s, ok := starlark.AsString(elem)
		if !ok {
			return nil, fmt.Errorf("command list elements must be strings, got %s", elem.Type())
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("command list must not be empty")
	}
	if strings.TrimSpace(out[0]) == "" {
		return nil, fmt.Errorf("command executable name must not be empty")
	}
	return out, nil
}

// describeTransport returns a short string used as the permission "resource"
// for the mcp.connect check. For a list, the resource is argv[0]; for a URL,
// the URL itself. Gives operators something meaningful to match against in
// permission policies.
func describeTransport(v starlark.Value) string {
	switch vv := v.(type) {
	case *starlark.List:
		if vv.Len() > 0 {
			if s, ok := starlark.AsString(vv.Index(0)); ok {
				return s
			}
		}
		return "stdio"
	case starlark.Tuple:
		if len(vv) > 0 {
			if s, ok := starlark.AsString(vv[0]); ok {
				return s
			}
		}
		return "stdio"
	case starlark.String:
		return string(vv)
	}
	return ""
}
