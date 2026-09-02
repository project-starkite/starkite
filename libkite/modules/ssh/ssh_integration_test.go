package ssh

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

	authDict := starlark.NewDict(1)
	authDict.SetKey(starlark.String("user"), starlark.String("deploy"))

	thread := &starlark.Thread{Name: "test-ssh-fleet"}
	val, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("fleet"), fl},
		{starlark.String("auth"), authDict},
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

	authDict := starlark.NewDict(1)
	authDict.SetKey(starlark.String("user"), starlark.String("root"))

	// 1. hosts as list
	val1, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.NewList([]starlark.Value{starlark.String("192.168.1.5")})},
		{starlark.String("auth"), authDict},
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

func TestSSHExecWithFleetClient(t *testing.T) {
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

	configBuiltin := dict["ssh"].(starlark.HasAttrs)
	configFn, _ := configBuiltin.Attr("config")

	authDict := starlark.NewDict(2)
	authDict.SetKey(starlark.String("user"), starlark.String("testuser"))
	authDict.SetKey(starlark.String("password"), starlark.String("secret"))

	thread := &starlark.Thread{Name: "test-ssh-fleet-exec"}
	clientVal, err := starlark.Call(thread, configFn, nil, []starlark.Tuple{
		{starlark.String("fleet"), testFleet},
		{starlark.String("auth"), authDict},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("ssh.config error: %v", err)
	}

	client := clientVal.(*SSHClient)
	res, err := client.exec(thread, nil, starlark.Tuple{starlark.String("uptime")}, nil)
	if err != nil {
		t.Fatalf("client.exec error: %v", err)
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

func TestSSHConfigJumpHostSettings(t *testing.T) {
	mod := New()
	thread := &starlark.Thread{Name: "test-jump-config"}

	authDict := starlark.NewDict(1)
	authDict.SetKey(starlark.String("user"), starlark.String("pi"))

	jumpDict := starlark.NewDict(4)
	jumpDict.SetKey(starlark.String("host"), starlark.String("bastion.lan"))
	jumpDict.SetKey(starlark.String("user"), starlark.String("vladimir"))
	jumpDict.SetKey(starlark.String("port"), starlark.MakeInt(2222))
	jumpDict.SetKey(starlark.String("key"), starlark.String("/tmp/bastion_key"))

	val, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("10.0.0.5")},
		{starlark.String("auth"), authDict},
		{starlark.String("jump"), jumpDict},
	})
	if err != nil {
		t.Fatal(err)
	}

	client := val.(*SSHClient)
	if client.jumpHost != "bastion.lan" {
		t.Errorf("jumpHost = %q, want %q", client.jumpHost, "bastion.lan")
	}
	if client.jumpUser != "vladimir" {
		t.Errorf("jumpUser = %q, want %q", client.jumpUser, "vladimir")
	}
	if client.jumpPort != 2222 {
		t.Errorf("jumpPort = %d, want 2222", client.jumpPort)
	}
	if client.jumpKeyFile != "/tmp/bastion_key" {
		t.Errorf("jumpKeyFile = %q, want /tmp/bastion_key", client.jumpKeyFile)
	}

	jumpAttr, _ := client.Attr("jump")
	jumpD, ok := jumpAttr.(*starlark.Dict)
	if !ok {
		t.Fatalf("jump attr is not dict: %v", jumpAttr)
	}
	uVal, _, _ := jumpD.Get(starlark.String("user"))
	if s, ok := uVal.(starlark.String); !ok || string(s) != "vladimir" {
		t.Errorf("jump.user attr = %v, want vladimir", uVal)
	}
}

func TestSSHCopyIdBasic(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("pi", "secret")
	var executedCmd string
	ts.HandleExec(func(cmd string) (string, string, int) {
		executedCmd = cmd
		return "", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.user = "pi"
		c.password = "secret"
	})

	kp, err := GenerateKeyPair("ed25519", 0, "test-copy-id")
	if err != nil {
		t.Fatal(err)
	}

	thread := &starlark.Thread{Name: "test-copy-id"}
	val, err := c.copyId(thread, nil, nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String(kp.PublicKey)},
	})
	if err != nil {
		t.Fatalf("copyId failed: %v", err)
	}

	list := val.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", list.Len())
	}
	res := list.Index(0).(starlark.HasAttrs)
	okVal, _ := res.Attr("ok")
	if okVal != starlark.True {
		t.Errorf("expected ok=True")
	}

	if !strings.Contains(executedCmd, "authorized_keys") {
		t.Errorf("expected command to reference authorized_keys, got: %q", executedCmd)
	}
}

func TestSSHCopyIdFromFile(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("pi", "secret")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "", "", 0
	})

	c := testClient(t, ts, func(c *SSHClient) {
		c.user = "pi"
		c.password = "secret"
	})

	tmpDir := t.TempDir()
	pubFile := filepath.Join(tmpDir, "id_test.pub")
	kp, _ := GenerateKeyPair("ed25519", 0, "from-file")
	if err := os.WriteFile(pubFile, []byte(kp.PublicKey), 0644); err != nil {
		t.Fatal(err)
	}

	thread := &starlark.Thread{Name: "test-copy-id-file"}
	val, err := c.copyId(thread, nil, nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String(pubFile)},
	})
	if err != nil {
		t.Fatalf("copyId with file path failed: %v", err)
	}

	list := val.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", list.Len())
	}
	res := list.Index(0).(starlark.HasAttrs)
	okVal, _ := res.Attr("ok")
	if okVal != starlark.True {
		t.Errorf("expected ok=True")
	}
}

func TestSSHCopyIdInvalidKey(t *testing.T) {
	c := &SSHClient{hosts: []string{"127.0.0.1"}}
	thread := &starlark.Thread{Name: "test-copy-id-invalid"}

	_, err := c.copyId(thread, nil, nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String("invalid-key-data")},
	})
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestSSHCopyIdOneShotModule(t *testing.T) {
	ts := newTestServerForTest(t)
	ts.AddPassword("pi", "secret")
	ts.HandleExec(func(cmd string) (string, string, int) {
		return "", "", 0
	})

	host, portStr, _ := net.SplitHostPort(ts.Addr())
	port, _ := strconv.Atoi(portStr)

	kp, _ := GenerateKeyPair("ed25519", 0, "oneshot")

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	copyIdFn := dict["ssh"].(starlark.HasAttrs)
	fn, err := copyIdFn.Attr("copy_id")
	if err != nil || fn == nil {
		t.Fatalf("failed to get ssh.copy_id builtin: %v", err)
	}

	thread := &starlark.Thread{Name: "test-ssh-copy-id-oneshot"}
	res, err := starlark.Call(thread, fn, nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String(kp.PublicKey)},
		{starlark.String("hosts"), starlark.String(host)},
		{starlark.String("port"), starlark.MakeInt(port)},
		{starlark.String("user"), starlark.String("pi")},
		{starlark.String("password"), starlark.String("secret")},
		{starlark.String("host_key_check"), starlark.False},
	})
	if err != nil {
		t.Fatalf("ssh.copy_id error: %v", err)
	}

	list := res.(*starlark.List)
	if list.Len() != 1 {
		t.Fatalf("expected 1 result, got %d", list.Len())
	}
	item := list.Index(0).(starlark.HasAttrs)
	okVal, _ := item.Attr("ok")
	if okVal != starlark.True {
		t.Errorf("expected ok=True")
	}
}

func TestSSHConfigStructuredAuthAndJump(t *testing.T) {
	mod := New()
	thread := &starlark.Thread{Name: "test-structured-config"}

	authDict := starlark.NewDict(4)
	authDict.SetKey(starlark.String("user"), starlark.String("pi"))
	authDict.SetKey(starlark.String("key"), starlark.String("~/.ssh/id_ed25519"))
	authDict.SetKey(starlark.String("use_agent"), starlark.True)
	authDict.SetKey(starlark.String("prompt"), starlark.True)

	jumpDict := starlark.NewDict(3)
	jumpDict.SetKey(starlark.String("host"), starlark.String("jump.corp.net"))
	jumpDict.SetKey(starlark.String("user"), starlark.String("vladimir"))
	jumpDict.SetKey(starlark.String("port"), starlark.MakeInt(2222))

	val, err := mod.sshConfig(thread, starlark.NewBuiltin("ssh.config", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("10.0.0.5")},
		{starlark.String("auth"), authDict},
		{starlark.String("jump"), jumpDict},
	})
	if err != nil {
		t.Fatal(err)
	}

	client := val.(*SSHClient)
	if client.user != "pi" {
		t.Errorf("user = %q, want 'pi'", client.user)
	}
	if client.keyFile != "~/.ssh/id_ed25519" {
		t.Errorf("keyFile = %q, want '~/.ssh/id_ed25519'", client.keyFile)
	}
	if !client.useAgent {
		t.Errorf("useAgent = %v, want true", client.useAgent)
	}
	if !client.prompt {
		t.Errorf("prompt = %v, want true", client.prompt)
	}
	if client.jumpHost != "jump.corp.net" {
		t.Errorf("jumpHost = %q, want 'jump.corp.net'", client.jumpHost)
	}
	if client.jumpUser != "vladimir" {
		t.Errorf("jumpUser = %q, want 'vladimir'", client.jumpUser)
	}
	if client.jumpPort != 2222 {
		t.Errorf("jumpPort = %d, want 2222", client.jumpPort)
	}

	// Verify client.auth dict attribute
	authVal, _ := client.Attr("auth")
	if authVal == nil {
		t.Fatal("expected client.auth attribute")
	}
	userVal, _, _ := authVal.(*starlark.Dict).Get(starlark.String("user"))
	if userVal != starlark.String("pi") {
		t.Errorf("auth.user = %v, want 'pi'", userVal)
	}

	// Verify client.jump dict attribute
	jumpVal, _ := client.Attr("jump")
	if jumpVal == nil {
		t.Fatal("expected client.jump attribute")
	}
	hostVal, _, _ := jumpVal.(*starlark.Dict).Get(starlark.String("host"))
	if hostVal != starlark.String("jump.corp.net") {
		t.Errorf("jump.host = %v, want 'jump.corp.net'", hostVal)
	}
}

func TestSSHUseAgentWithoutSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	c := &SSHClient{
		hosts:    []string{"127.0.0.1"},
		port:     22,
		user:     "pi",
		useAgent: true,
	}

	_, err := c.buildSSHConfig()
	if err == nil {
		t.Error("expected error when use_agent=True and SSH_AUTH_SOCK is empty")
	}
}

func TestSSHEncryptedKeyWithPassphrase(t *testing.T) {
	// Generate an encrypted key
	kp, err := GenerateKeyPairWithOptions(KeyGenOptions{
		Type:       "ed25519",
		Passphrase: "my-secret-pass",
		Comment:    "encrypted-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	tmpFile := filepath.Join(t.TempDir(), "id_ed25519_enc")
	if err := os.WriteFile(tmpFile, []byte(kp.PrivateKey), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Build SSHClient with correct passphrase
	c := &SSHClient{
		hosts:         []string{"127.0.0.1"},
		port:          22,
		user:          "pi",
		keyFile:       tmpFile,
		keyPassphrase: "my-secret-pass",
	}
	cfg, err := c.buildSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error parsing encrypted key with passphrase: %v", err)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(cfg.Auth))
	}

	// 2. Build SSHClient without passphrase and prompt=false (default: should return clear error)
	cNoPass := &SSHClient{
		hosts:   []string{"127.0.0.1"},
		port:    22,
		user:    "pi",
		keyFile: tmpFile,
	}
	_, err = cNoPass.buildSSHConfig()
	if err == nil {
		t.Error("expected error parsing encrypted key without passphrase")
	} else if !strings.Contains(err.Error(), "passphrase protected") {
		t.Errorf("error = %q, want 'passphrase protected'", err.Error())
	}

	// 3. Build SSHClient with prompt=true in non-terminal (should report non-terminal error)
	cAskPass := &SSHClient{
		hosts:   []string{"127.0.0.1"},
		port:    22,
		user:    "pi",
		keyFile: tmpFile,
		prompt:  true,
	}
	_, err = cAskPass.buildSSHConfig()
	if err == nil {
		t.Error("expected error when prompt=True in non-terminal")
	} else if !strings.Contains(err.Error(), "standard input is not a terminal") {
		t.Errorf("error = %q, want 'standard input is not a terminal'", err.Error())
	}
}

func TestSSHEncryptedKeyWithUseAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix socket test skipped on windows")
	}
	// Create dummy unix socket with short path for Darwin limit
	sockFile := fmt.Sprintf("ag_%d.sock", time.Now().UnixNano())
	sockPath := filepath.Join(os.TempDir(), sockFile)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		l.Close()
		os.Remove(sockPath)
	}()
	t.Setenv("SSH_AUTH_SOCK", sockPath)

	kp, err := GenerateKeyPairWithOptions(KeyGenOptions{
		Type:       "ed25519",
		Passphrase: "my-secret-pass",
		Comment:    "encrypted-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	tmpFile := filepath.Join(t.TempDir(), "id_ed25519_enc")
	if err := os.WriteFile(tmpFile, []byte(kp.PrivateKey), 0600); err != nil {
		t.Fatal(err)
	}

	// Client specifies use_agent=True, encrypted key on disk, prompt=False
	c := &SSHClient{
		hosts:    []string{"127.0.0.1"},
		port:     22,
		user:     "pi",
		keyFile:  tmpFile,
		useAgent: true,
		prompt:   false,
	}

	cfg, err := c.buildSSHConfig()
	if err != nil {
		t.Fatalf("expected buildSSHConfig to succeed delegating to agent, got error: %v", err)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("expected 1 auth method (agent callback), got %d", len(cfg.Auth))
	}
}

func TestSSHOneShotRejectsFleetAndJump(t *testing.T) {
	mod := New()
	thread := &starlark.Thread{Name: "test-oneshot-rejections"}

	// 1. ssh.exec rejects fleet
	_, err := mod.sshExec(thread, starlark.NewBuiltin("ssh.exec", nil), starlark.Tuple{starlark.String("uptime")}, []starlark.Tuple{
		{starlark.String("fleet"), starlark.None},
	})
	if err == nil || !strings.Contains(err.Error(), "'fleet' parameter is not supported") {
		t.Errorf("expected error rejecting fleet on ssh.exec, got %v", err)
	}

	// 2. ssh.exec rejects jump
	_, err = mod.sshExec(thread, starlark.NewBuiltin("ssh.exec", nil), starlark.Tuple{starlark.String("uptime")}, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("10.0.0.1")},
		{starlark.String("jump_host"), starlark.String("bastion")},
	})
	if err == nil || !strings.Contains(err.Error(), "bastion/jump host routing is not supported") {
		t.Errorf("expected error rejecting jump on ssh.exec, got %v", err)
	}

	// 3. ssh.copy_id rejects fleet
	_, err = mod.sshCopyId(thread, starlark.NewBuiltin("ssh.copy_id", nil), nil, []starlark.Tuple{
		{starlark.String("fleet"), starlark.None},
	})
	if err == nil || !strings.Contains(err.Error(), "'fleet' parameter is not supported") {
		t.Errorf("expected error rejecting fleet on ssh.copy_id, got %v", err)
	}

	// 4. ssh.copy_id rejects jump
	_, err = mod.sshCopyId(thread, starlark.NewBuiltin("ssh.copy_id", nil), nil, []starlark.Tuple{
		{starlark.String("hosts"), starlark.String("10.0.0.1")},
		{starlark.String("jump_host"), starlark.String("bastion")},
	})
	if err == nil || !strings.Contains(err.Error(), "bastion/jump host routing is not supported") {
		t.Errorf("expected error rejecting jump on ssh.copy_id, got %v", err)
	}
}
