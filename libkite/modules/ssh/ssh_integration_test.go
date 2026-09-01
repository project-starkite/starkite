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

	"github.com/project-starkite/starkite/libkite/fleet"
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

func TestSSHConfigWithFleet(t *testing.T) {
	mod := New()
	fl := fleet.New([]fleet.Resource{
		{ID: "node-1", Name: "node-1", Address: "10.0.1.10", Kind: "host"},
		{ID: "node-2", Name: "node-2", Address: "10.0.1.11", Kind: "host"},
	})

	thread := &starlark.Thread{Name: "test-ssh-fleet"}
	val, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("fleet"), fl},
		{starlark.String("user"), starlark.String("deploy")},
	})
	if err != nil {
		t.Fatalf("ssh.config with fleet failed: %v", err)
	}

	client := val.(*SSHClient)
	if len(client.hosts) != 2 || client.hosts[0] != "10.0.1.10" || client.hosts[1] != "10.0.1.11" {
		t.Fatalf("unexpected client hosts: %v", client.hosts)
	}
	if client.fleet == nil || client.fleet.Resources()[0].Name != "node-1" {
		t.Fatalf("unexpected client fleet: %v", client.fleet)
	}

	// Test fleet attribute on client
	fleetAttr, err := client.Attr("fleet")
	if err != nil || fleetAttr == starlark.None {
		t.Fatalf("expected fleet attribute on client, got %v (err: %v)", fleetAttr, err)
	}
}

func TestSSHConfigWithHostsShortcut(t *testing.T) {
	mod := New()
	thread := &starlark.Thread{Name: "test-ssh-hosts"}

	// 1. hosts as list
	val1, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.NewList([]starlark.Value{starlark.String("192.168.1.5")})},
		{starlark.String("user"), starlark.String("root")},
	})
	if err != nil {
		t.Fatal(err)
	}
	c1 := val1.(*SSHClient)
	if len(c1.hosts) != 1 || c1.hosts[0] != "192.168.1.5" {
		t.Fatalf("unexpected hosts list result: %v", c1.hosts)
	}
	if c1.fleet == nil || len(c1.fleet.Resources()) != 1 {
		t.Fatalf("expected synthesized fleet from hosts list, got %v", c1.fleet)
	}

	// 2. hosts as single string shortcut
	val2, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("192.168.1.6")},
	})
	if err != nil {
		t.Fatal(err)
	}
	c2 := val2.(*SSHClient)
	if len(c2.hosts) != 1 || c2.hosts[0] != "192.168.1.6" {
		t.Fatalf("unexpected single host result: %v", c2.hosts)
	}
}

func TestSSHConfigExecMaxWorkers(t *testing.T) {
	mod := New()
	thread := &starlark.Thread{Name: "test-ssh-workers"}

	val, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.NewList([]starlark.Value{starlark.String("10.0.0.1"), starlark.String("10.0.0.2")})},
		{starlark.String("exec_max_workers"), starlark.MakeInt(16)},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := val.(*SSHClient)
	if client.execMaxWorkers != 16 {
		t.Fatalf("expected execMaxWorkers=16, got %d", client.execMaxWorkers)
	}

	attrVal, err := client.Attr("exec_max_workers")
	if err != nil {
		t.Fatal(err)
	}
	if i, ok := attrVal.(starlark.Int); !ok || i.String() != "16" {
		t.Fatalf("expected exec_max_workers attribute to be 16, got %v", attrVal)
	}
}

func TestSSHExecOneShot(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "oneshot-ok\n", "", 0
	})

	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	execBuiltin := dict["ssh"].(starlark.HasAttrs)
	execFn, err := execBuiltin.Attr("exec")
	if err != nil || execFn == nil {
		t.Fatalf("failed to get ssh.exec builtin: %v", err)
	}

	thread := &starlark.Thread{Name: "test-ssh-exec-oneshot"}
	res, err := starlark.Call(thread, execFn, starlark.Tuple{starlark.String("whoami")}, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String(host)},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("user"), starlark.String("testuser")},
		{starlark.String("password"), starlark.String("secret")},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("ssh.exec error: %v", err)
	}

	list, ok := res.(*starlark.List)
	if !ok || list.Len() != 1 {
		t.Fatalf("expected 1 result from ssh.exec, got %v", res)
	}
	stdout := mustAttr(t, list.Index(0), "stdout")
	if stdout != "oneshot-ok\n" {
		t.Errorf("stdout = %q, want %q", stdout, "oneshot-ok\n")
	}
}

func TestSSHExecBatchStopOnError(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	var executed []string
	ts.HandleExec(func(cmd string) (string, string, int) {
		executed = append(executed, cmd)
		if cmd == "cmd2" {
			return "", "err2", 1
		}
		return "out:" + cmd, "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "secret"
	})

	thread := &starlark.Thread{Name: "test-batch"}
	val, err := c.exec(thread, nil, nil, []starlark.Tuple{
		{starlark.String("commands"), starlark.NewList([]starlark.Value{starlark.String("cmd1"), starlark.String("cmd2"), starlark.String("cmd3")})},
		{starlark.String("exec_on_error"), starlark.String("stop")},
	})
	if err != nil {
		t.Fatal(err)
	}

	list := val.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 host result, got %d", list.Len())
	}
	batchRes := list.Index(0).(starlark.HasAttrs)
	okVal, _ := batchRes.Attr("ok")
	if okVal != starlark.False {
		t.Errorf("batch result ok should be false")
	}
	stoppedEarly, _ := batchRes.Attr("stopped_early")
	if stoppedEarly != starlark.True {
		t.Errorf("batch result stopped_early should be true")
	}
	stepsVal, _ := batchRes.Attr("steps")
	steps := stepsVal.(*starlark.List)
	if steps.Len() != 2 {
		t.Fatalf("expected 2 steps executed before stopping, got %d", steps.Len())
	}
	if len(executed) != 2 || executed[0] != "cmd1" || executed[1] != "cmd2" {
		t.Fatalf("unexpected executed commands: %v", executed)
	}
}

func TestSSHExecBatchContinueOnError(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	var executed []string
	ts.HandleExec(func(cmd string) (string, string, int) {
		executed = append(executed, cmd)
		if cmd == "cmd2" {
			return "", "err2", 1
		}
		return "out:" + cmd, "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.password = "secret"
	})

	thread := &starlark.Thread{Name: "test-batch-continue"}
	val, err := c.exec(thread, nil, nil, []starlark.Tuple{
		{starlark.String("commands"), starlark.NewList([]starlark.Value{starlark.String("cmd1"), starlark.String("cmd2"), starlark.String("cmd3")})},
		{starlark.String("exec_on_error"), starlark.String("continue")},
	})
	if err != nil {
		t.Fatal(err)
	}

	list := val.(*starlark.List)
	batchRes := list.Index(0).(starlark.HasAttrs)
	okVal, _ := batchRes.Attr("ok")
	if okVal != starlark.False {
		t.Errorf("batch result ok should be false")
	}
	stoppedEarly, _ := batchRes.Attr("stopped_early")
	if stoppedEarly != starlark.False {
		t.Errorf("batch result stopped_early should be false")
	}
	stepsVal, _ := batchRes.Attr("steps")
	steps := stepsVal.(*starlark.List)
	if steps.Len() != 3 {
		t.Fatalf("expected 3 steps executed, got %d", steps.Len())
	}
	if len(executed) != 3 {
		t.Fatalf("unexpected executed commands count: %v", executed)
	}
}

func TestSSHExecOneShotWithCommands(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	var executed []string
	ts.HandleExec(func(cmd string) (string, string, int) {
		executed = append(executed, cmd)
		return "res:" + cmd, "", 0
	})

	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	execBuiltin := dict["ssh"].(starlark.HasAttrs)
	execFn, err := execBuiltin.Attr("exec")
	if err != nil || execFn == nil {
		t.Fatalf("failed to get ssh.exec builtin: %v", err)
	}

	thread := &starlark.Thread{Name: "test-ssh-exec-commands"}
	res, err := starlark.Call(thread, execFn, nil, []starlark.Tuple{
		{starlark.String("commands"), starlark.NewList([]starlark.Value{starlark.String("echo a"), starlark.String("echo b")})},
		{starlark.String("hosts"), starlark.String(host)},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("user"), starlark.String("testuser")},
		{starlark.String("password"), starlark.String("secret")},
		{starlark.String("host_key_check"), starlark.False},
		{starlark.String("exec_on_error"), starlark.String("stop")},
	})
	if err != nil {
		t.Fatalf("ssh.exec error: %v", err)
	}

	list, ok := res.(*starlark.List)
	if !ok || list.Len() != 1 {
		t.Fatalf("expected 1 host batch result, got %v", res)
	}
	batch := list.Index(0).(starlark.HasAttrs)
	okVal, _ := batch.Attr("ok")
	if okVal != starlark.True {
		t.Errorf("batch ok should be true")
	}
	stepsVal, _ := batch.Attr("steps")
	steps := stepsVal.(*starlark.List)
	if steps.Len() != 2 {
		t.Fatalf("expected 2 step results, got %d", steps.Len())
	}
	if len(executed) != 2 {
		t.Fatalf("expected 2 commands executed, got %d", len(executed))
	}
}

func TestSSHExecOneShotWithFleet(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "fleet-exec-ok\n", "", 0
	})

	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)

	testFleet := fleet.New([]fleet.Resource{
		{ID: "node1", Name: "node1", Kind: "host", Address: host},
	})

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	execBuiltin := dict["ssh"].(starlark.HasAttrs)
	execFn, _ := execBuiltin.Attr("exec")

	thread := &starlark.Thread{Name: "test-ssh-fleet-exec"}
	res, err := starlark.Call(thread, execFn, starlark.Tuple{starlark.String("uptime")}, []starlark.Tuple{
		{starlark.String("fleet"), testFleet},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("user"), starlark.String("testuser")},
		{starlark.String("password"), starlark.String("secret")},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("ssh.exec error: %v", err)
	}

	list := res.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", list.Len())
	}
	stdout := mustAttr(t, list.Index(0), "stdout")
	if stdout != "fleet-exec-ok\n" {
		t.Errorf("stdout = %q, want %q", stdout, "fleet-exec-ok\n")
	}
}

func TestSSHExecOneShotDualArgs(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("testuser", "secret")
	var executedCmd string
	ts.HandleExec(func(cmd string) (string, string, int) {
		executedCmd = cmd
		return "dual-ok\n", "", 0
	})

	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	execBuiltin := dict["ssh"].(starlark.HasAttrs)
	execFn, _ := execBuiltin.Attr("exec")

	thread := &starlark.Thread{Name: "test-ssh-dual-args"}
	res, err := starlark.Call(thread, execFn, starlark.Tuple{
		starlark.String("git"),
		starlark.NewList([]starlark.Value{starlark.String("commit"), starlark.String("-m"), starlark.String("hello world")}),
	}, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String(host)},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("user"), starlark.String("testuser")},
		{starlark.String("password"), starlark.String("secret")},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("ssh.exec error: %v", err)
	}

	list := res.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", list.Len())
	}
	if executedCmd != `git commit -m "hello world"` {
		t.Errorf("executed command = %q, want %q", executedCmd, `git commit -m "hello world"`)
	}
}

func TestSSHTryExecFailure(t *testing.T) {
	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	sshMod := dict["ssh"].(starlark.HasAttrs)
	tryExecFn, err := sshMod.Attr("try_exec")
	if err != nil || tryExecFn == nil {
		t.Fatalf("failed to get ssh.try_exec builtin: %v", err)
	}

	thread := &starlark.Thread{Name: "test-try-exec-fail"}
	// Target an unreachable address with 0 retries and 50ms timeout for instant fail
	res, err := starlark.Call(thread, tryExecFn, starlark.Tuple{starlark.String("whoami")}, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("127.0.0.1")},
		{starlark.String("port"), starlark.MakeInt(65534)},
		{starlark.String("user"), starlark.String("nonexistent")},
		{starlark.String("timeout"), starlark.String("50ms")},
		{starlark.String("max_retries"), starlark.MakeInt(0)},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("try_exec should not raise Go error: %v", err)
	}

	result, ok := res.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected Result implementing HasAttrs from try_exec, got %v", res)
	}
	okAttr, _ := result.Attr("ok")
	if okAttr != starlark.False {
		t.Errorf("try_exec ok should be False on connection failure")
	}
	errAttr, _ := result.Attr("error")
	if errAttr == nil || errAttr == starlark.None || errAttr.(starlark.String) == "" {
		t.Errorf("try_exec error should contain error message")
	}
}

func TestSSHExecValidationErrors(t *testing.T) {
	c := &SSHClient{hosts: []string{"127.0.0.1"}}
	thread := &starlark.Thread{Name: "test-validation"}

	// 1. Invalid exec_on_error
	_, err := c.exec(thread, nil, nil, []starlark.Tuple{
		{starlark.String("commands"), starlark.NewList([]starlark.Value{starlark.String("cmd1")})},
		{starlark.String("exec_on_error"), starlark.String("invalid_policy")},
	})
	if err == nil {
		t.Errorf("expected error for invalid exec_on_error")
	}

	// 2. Non-string item in commands list
	_, err = c.exec(thread, nil, nil, []starlark.Tuple{
		{starlark.String("commands"), starlark.NewList([]starlark.Value{starlark.MakeInt(123)})},
	})
	if err == nil {
		t.Errorf("expected error for non-string item in commands")
	}
}
