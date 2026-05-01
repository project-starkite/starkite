package ssh

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func testClient(t *testing.T, ts *TestServer, opts ...func(*SSHClient)) *SSHClient {
	t.Helper()
	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)
	c := &SSHClient{
		hosts:             []string{host},
		port:              port,
		user:              "testuser",
		timeout:           5 * time.Second,
		maxRetries:        0,
		hostKeyCheck:      false,
		keepAliveInterval: 0,
		execPolicy:        "linear",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func TestExecPasswordAuth(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "hello\n", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "secret"
	})

	result, err := c.execOnHost("127.0.0.1", "echo hello")
	if err != nil {
		t.Fatalf("execOnHost: %v", err)
	}

	stdout := mustAttr(t, result, "stdout")
	if stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", stdout, "hello\n")
	}
	code := mustAttrInt(t, result, "code")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
}

func TestExecPublicKeyAuth(t *testing.T) {
	ts := newTestServerForTest(t)
	keyPath, pubKey := clientKeyForTest(t)
	ts.AddAuthorizedKey(pubKey)
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "key-ok\n", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.keyFile = keyPath
	})

	result, err := c.execOnHost("127.0.0.1", "whoami")
	if err != nil {
		t.Fatalf("execOnHost: %v", err)
	}

	stdout := mustAttr(t, result, "stdout")
	if stdout != "key-ok\n" {
		t.Errorf("stdout = %q, want %q", stdout, "key-ok\n")
	}
}

func TestExecNonZeroExit(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "", "error\n", 42
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	result, err := c.execOnHost("127.0.0.1", "fail")
	if err != nil {
		t.Fatalf("execOnHost: %v", err)
	}

	code := mustAttrInt(t, result, "code")
	if code != 42 {
		t.Errorf("code = %d, want 42", code)
	}

	stderr := mustAttr(t, result, "stderr")
	if stderr != "error\n" {
		t.Errorf("stderr = %q, want %q", stderr, "error\n")
	}
}

func TestExecStderr(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "out\n", "err\n", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	result, err := c.execOnHost("127.0.0.1", "test")
	if err != nil {
		t.Fatalf("execOnHost: %v", err)
	}

	stdout := mustAttr(t, result, "stdout")
	stderr := mustAttr(t, result, "stderr")
	if stdout != "out\n" {
		t.Errorf("stdout = %q, want %q", stdout, "out\n")
	}
	if stderr != "err\n" {
		t.Errorf("stderr = %q, want %q", stderr, "err\n")
	}
}

func TestExecConcurrent(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "ok\n", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
		c.hosts = []string{"127.0.0.1", "127.0.0.1", "127.0.0.1"}
		c.execPolicy = "concurrent"
	})

	result, err := c.execConcurrent("test")
	if err != nil {
		t.Fatalf("execConcurrent: %v", err)
	}

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatal("result should be a *starlark.List")
	}
	if list.Len() != 3 {
		t.Errorf("list.Len() = %d, want 3", list.Len())
	}
}

func TestSCPUpload(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")

	// Create local file to upload
	content := []byte("upload test content")
	localPath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	n, err := c.scpUploadToHost("127.0.0.1", localPath, "/remote/file.txt", "0644")
	if err != nil {
		t.Fatalf("scpUploadToHost: %v", err)
	}

	if n != int64(len(content)) {
		t.Errorf("transferred %d bytes, want %d", n, len(content))
	}

	uploaded := ts.Uploaded("/remote/file.txt")
	if uploaded == nil {
		t.Fatal("server should have received upload")
	}
	if !bytes.Equal(uploaded.Content, content) {
		t.Errorf("uploaded content = %q, want %q", uploaded.Content, content)
	}
}

func TestSCPDownload(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")
	ts.AddFile("/remote/data.txt", []byte("download content"), "0644")

	localPath := filepath.Join(t.TempDir(), "downloaded.txt")

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	n, err := c.scpDownloadFromHost("127.0.0.1", "/remote/data.txt", localPath)
	if err != nil {
		t.Fatalf("scpDownloadFromHost: %v", err)
	}

	if n != int64(len("download content")) {
		t.Errorf("transferred %d bytes, want %d", n, len("download content"))
	}

	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "download content" {
		t.Errorf("downloaded content = %q, want %q", got, "download content")
	}
}

func TestSCPUploadLargeFile(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")

	// 1MB file
	content := bytes.Repeat([]byte("x"), 1024*1024)
	localPath := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	n, err := c.scpUploadToHost("127.0.0.1", localPath, "/remote/large.bin", "0644")
	if err != nil {
		t.Fatalf("scpUploadToHost: %v", err)
	}

	if n != int64(len(content)) {
		t.Errorf("transferred %d bytes, want %d", n, len(content))
	}

	uploaded := ts.Uploaded("/remote/large.bin")
	if uploaded == nil {
		t.Fatal("server should have received upload")
	}
	if len(uploaded.Content) != len(content) {
		t.Errorf("uploaded size = %d, want %d", len(uploaded.Content), len(content))
	}
}

func TestSCPDownloadNotFound(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")

	localPath := filepath.Join(t.TempDir(), "notfound.txt")

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
	})

	_, err := c.scpDownloadFromHost("127.0.0.1", "/nonexistent", localPath)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestAuthFailure(t *testing.T) {
	// Prevent falling through to SSH-agent auth on dev machines with
	// SSH_AUTH_SOCK set — the test server accepts any key when no
	// authorized_keys are configured, so agent auth would succeed and
	// defeat the point of the test.
	t.Setenv("SSH_AUTH_SOCK", "")

	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "correct")

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "wrong"
	})

	_, err := c.dialHost("127.0.0.1")
	if err == nil {
		t.Fatal("expected auth failure, got nil")
	}
}

func TestKeepalive(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "pass")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "ok\n", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "pass"
		c.keepAliveInterval = 100 * time.Millisecond
		c.keepAliveMax = 3
	})

	// dialHostWithRetry calls startKeepalive on success
	client, err := c.dialHostWithRetry("127.0.0.1")
	if err != nil {
		t.Fatalf("dialHostWithRetry: %v", err)
	}
	defer client.Close()

	// Hold connection open long enough for several keepalive rounds
	time.Sleep(350 * time.Millisecond)

	// Connection should still be alive — run a session to prove it
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession after keepalive: %v", err)
	}
	session.Close()
}

// --- test helpers ---

func mustAttr(t *testing.T, val starlark.Value, name string) string {
	t.Helper()
	ha, ok := val.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("value does not have attrs")
	}
	v, err := ha.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%q): %v", name, err)
	}
	s, ok := starlark.AsString(v)
	if !ok {
		t.Fatalf("Attr(%q) is not a string: %v", name, v)
	}
	return s
}

func mustAttrInt(t *testing.T, val starlark.Value, name string) int {
	t.Helper()
	ha, ok := val.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("value does not have attrs")
	}
	v, err := ha.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%q): %v", name, err)
	}
	i, err := starlark.AsInt32(v)
	if err != nil {
		t.Fatalf("Attr(%q) is not an int: %v", name, v)
	}
	return i
}
