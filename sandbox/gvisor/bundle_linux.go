//go:build linux

package gvisor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// bundle is a created OCI bundle on disk plus the cleanup callable.
// rootfsPath is the absolute path to the bundle's rootfs/ subdirectory;
// callers should set Spec.Root.Path to this absolute path so gVisor's
// gofer can stat it regardless of the gofer's working directory.
type bundle struct {
	id         string
	dir        string
	rootfsPath string
	cleanup    func()
}

// allocBundle creates a temp OCI bundle directory and an empty rootfs/
// subdirectory. The caller writes config.json via writeSpec once the
// spec (which depends on rootfsPath) has been built.
func allocBundle() (*bundle, error) {
	id, err := newSandboxID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(os.TempDir(), "starkite-sandbox-"+id)
	rootfs := filepath.Join(dir, "rootfs")
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		return nil, fmt.Errorf("creating bundle dir: %w", err)
	}
	return &bundle{
		id:         id,
		dir:        dir,
		rootfsPath: rootfs,
		cleanup:    func() { _ = os.RemoveAll(dir) },
	}, nil
}

// writeSpec serializes spec to <bundle.dir>/config.json.
func (b *bundle) writeSpec(spec *specs.Spec) error {
	f, err := os.Create(filepath.Join(b.dir, "config.json"))
	if err != nil {
		return fmt.Errorf("creating config.json: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		_ = f.Close()
		return fmt.Errorf("encoding config.json: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing config.json: %w", err)
	}
	return nil
}

// newSandboxID returns a 16-byte hex container ID. We mirror gVisor's
// sandboxexec/sandbox pattern (random, no embedded PID) to avoid name
// collisions when multiple kite invocations run concurrently.
func newSandboxID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating sandbox id: %w", err)
	}
	return "kite-" + hex.EncodeToString(b[:]), nil
}
