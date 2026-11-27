package k8s

import (
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

// TestClientTryAttr verifies that Attr("try_X") returns a non-nil *starlark.Builtin
// for every method in allMethods.
func TestClientTryAttr(t *testing.T) {
	c := &K8sClient{}

	for name := range allMethods {
		tryName := "try_" + name
		v, err := c.Attr(tryName)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", tryName, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", tryName)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", tryName, v)
		}
	}
}

// TestClientTryAttrNames verifies that AttrNames() has 2*len(allMethods) entries,
// is sorted, and every method has both its name and try_ variant.
func TestClientTryAttrNames(t *testing.T) {
	c := &K8sClient{}
	names := c.AttrNames()

	expectedCount := 2 * len(allMethods)
	if len(names) != expectedCount {
		t.Fatalf("AttrNames() len = %d, want %d", len(names), expectedCount)
	}

	// Verify sorted
	if !sort.StringsAreSorted(names) {
		t.Fatal("AttrNames() is not sorted")
	}

	// Build lookup set
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	for name := range allMethods {
		if !nameSet[name] {
			t.Errorf("AttrNames() missing %q", name)
		}
		tryName := "try_" + name
		if !nameSet[tryName] {
			t.Errorf("AttrNames() missing %q", tryName)
		}
	}
}

// TestClientDirectAttrUnchanged verifies that direct Attr lookups (without try_ prefix)
// still return *starlark.Builtin for all methods.
func TestClientDirectAttrUnchanged(t *testing.T) {
	c := &K8sClient{}

	for name := range allMethods {
		v, err := c.Attr(name)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", name, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", name)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", name, v)
		}
	}
}

// TestClientTryNonexistent verifies that Attr("try_bogus") returns (nil, nil).
func TestClientTryNonexistent(t *testing.T) {
	c := &K8sClient{}

	v, err := c.Attr("try_bogus")
	if err != nil {
		t.Fatalf("Attr(\"try_bogus\") error: %v", err)
	}
	if v != nil {
		t.Fatalf("Attr(\"try_bogus\") = %v, want nil", v)
	}
}

type fakeUtilExitError int

func (e fakeUtilExitError) Error() string   { return "exit status" }
func (e fakeUtilExitError) String() string  { return "exit status" }
func (e fakeUtilExitError) Exited() bool    { return true }
func (e fakeUtilExitError) ExitStatus() int { return int(e) }

func TestK8sHardeningHelpers(t *testing.T) {
	// 1. PortForwardHandle.Hash() must return unhashable error.
	h := &PortForwardHandle{}
	if _, err := h.Hash(); err == nil || !strings.Contains(err.Error(), "unhashable") {
		t.Fatalf("expected unhashable error, got %v", err)
	}

	// 2. classifyExecStreamError returns exit code for utilexec.ExitError and Go error otherwise.
	if code, err := classifyExecStreamError(nil); err != nil || code != 0 {
		t.Fatalf("nil streamErr: got (%d, %v), want (0, nil)", code, err)
	}
	if code, err := classifyExecStreamError(fakeUtilExitError(17)); err != nil || code != 17 {
		t.Fatalf("ExitError streamErr: got (%d, %v), want (17, nil)", code, err)
	}
	if code, err := classifyExecStreamError(errors.New("spdy reset")); err == nil || code != 0 {
		t.Fatalf("transport err: got (%d, %v), want (0, non-nil)", code, err)
	}

	// 3. K8sClient Close/Dialer concurrent access is race-free.
	kc := &K8sClient{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = kc.Close()
		}()
		go func() {
			defer wg.Done()
			_ = kc.Dialer()
		}()
	}
	wg.Wait()

	// 4. packLocalToTar must not emit a tar EOF trailer when an error occurs.
	missingPath := filepath.Join(t.TempDir(), "does-not-exist")
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		_, _, err := packLocalToTar(missingPath, pw)
		_ = pw.Close()
		errCh <- err
	}()
	written, readErr := io.ReadAll(pr)
	_ = pr.Close()
	if readErr != nil {
		t.Fatalf("pipe read failed: %v", readErr)
	}
	if packErr := <-errCh; packErr == nil {
		t.Fatal("expected packLocalToTar error on missing source")
	}
	if len(written) != 0 {
		t.Fatalf("expected 0 bytes written on packLocalToTar error (no EOF trailer), got %d bytes", len(written))
	}
}
