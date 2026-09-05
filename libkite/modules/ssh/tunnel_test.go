package ssh

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

// startEchoServer starts an in-process TCP echo server on 127.0.0.1.
func startEchoServer(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo server: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	return ln, ln.Addr().String()
}

func TestTunnelDialer_TargetValidation(t *testing.T) {
	jump := JumpConfig{
		Host: "127.0.0.1",
		Port: 2222,
	}

	// Valid target addresses
	validTargets := []string{
		"127.0.0.1:8080",
		"k8s-controller:6443",
		"[::1]:8080",
		"api.internal.net:443",
		"", // dynamic target allowed
	}
	for _, target := range validTargets {
		d, err := NewTunnelDialer(jump, target, 5*time.Second)
		if err != nil {
			t.Errorf("NewTunnelDialer with target %q failed: %v", target, err)
		}
		if d == nil {
			t.Errorf("NewTunnelDialer with target %q returned nil", target)
		}
	}

	// Invalid target addresses (must strictly fail via SplitHostPort, no implicit port fallback)
	invalidTargets := []string{
		"k8s-controller",
		"localhost",
		"127.0.0.1",
		":8080",
		"host:",
	}
	for _, target := range invalidTargets {
		_, err := NewTunnelDialer(jump, target, 5*time.Second)
		if err == nil {
			t.Errorf("NewTunnelDialer with target %q should have failed, but succeeded", target)
		}
	}

	// Missing jump host
	_, err := NewTunnelDialer(JumpConfig{}, "127.0.0.1:8080", 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "jump host is required") {
		t.Errorf("expected jump host is required error, got: %v", err)
	}
}

func TestTunnelDialer_FixedTargetDialing(t *testing.T) {
	echoLn, echoAddr := startEchoServer(t)
	defer echoLn.Close()

	ts, err := NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.AddPassword("bastionuser", "bastionpass")
	if err := ts.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer ts.Close()

	jump := JumpConfig{
		Host:         "127.0.0.1",
		Port:         ts.Port(),
		User:         "bastionuser",
		Password:     "bastionpass",
		HostKeyCheck: false,
	}

	dialer, err := NewTunnelDialer(jump, echoAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("NewTunnelDialer failed: %v", err)
	}
	defer dialer.Close()

	if dialer.TargetAddr() != echoAddr {
		t.Errorf("TargetAddr() = %q, want %q", dialer.TargetAddr(), echoAddr)
	}

	// Dial via DialContext
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", "")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	// Send message through tunnel and verify echo response
	msg := "hello over direct-tcpip\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write to tunneled conn: %v", err)
	}

	reader := bufio.NewReader(conn)
	reply, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read from tunneled conn: %v", err)
	}
	if reply != msg {
		t.Fatalf("got echo %q, want %q", reply, msg)
	}

	// Dial via Dial (without context)
	conn2, err := dialer.Dial("tcp", "")
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn2.Close()

	msg2 := "second message\n"
	if _, err := conn2.Write([]byte(msg2)); err != nil {
		t.Fatalf("failed to write to tunneled conn2: %v", err)
	}
	reader2 := bufio.NewReader(conn2)
	reply2, err := reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read from tunneled conn2: %v", err)
	}
	if reply2 != msg2 {
		t.Fatalf("got echo %q, want %q", reply2, msg2)
	}

	// Close dialer and ensure subsequent dials fail
	if err := dialer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	_, err = dialer.DialContext(context.Background(), "tcp", "")
	if err == nil || !strings.Contains(err.Error(), "dialer is closed") {
		t.Fatalf("expected dialer is closed error, got: %v", err)
	}
}

func TestTunnelDialer_DynamicDialing(t *testing.T) {
	echo1Ln, echo1Addr := startEchoServer(t)
	defer echo1Ln.Close()

	echo2Ln, echo2Addr := startEchoServer(t)
	defer echo2Ln.Close()

	ts, err := NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.AddPassword("bastionuser", "bastionpass")
	if err := ts.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer ts.Close()

	jump := JumpConfig{
		Host:         "127.0.0.1",
		Port:         ts.Port(),
		User:         "bastionuser",
		Password:     "bastionpass",
		HostKeyCheck: false,
	}

	// Dynamic dialer with empty targetAddr
	dialer, err := NewTunnelDialer(jump, "", 5*time.Second)
	if err != nil {
		t.Fatalf("NewTunnelDialer failed: %v", err)
	}
	defer dialer.Close()

	// 1. Dial server 1
	conn1, err := dialer.DialContext(context.Background(), "tcp", echo1Addr)
	if err != nil {
		t.Fatalf("DialContext to echo1 failed: %v", err)
	}
	defer conn1.Close()

	msg1 := "message to echo 1\n"
	if _, err := conn1.Write([]byte(msg1)); err != nil {
		t.Fatalf("write to conn1 failed: %v", err)
	}
	reply1, err := bufio.NewReader(conn1).ReadString('\n')
	if err != nil || reply1 != msg1 {
		t.Fatalf("reply1 = %q, want %q (err: %v)", reply1, msg1, err)
	}

	// 2. Dial server 2
	conn2, err := dialer.DialContext(context.Background(), "tcp", echo2Addr)
	if err != nil {
		t.Fatalf("DialContext to echo2 failed: %v", err)
	}
	defer conn2.Close()

	msg2 := "message to echo 2\n"
	if _, err := conn2.Write([]byte(msg2)); err != nil {
		t.Fatalf("write to conn2 failed: %v", err)
	}
	reply2, err := bufio.NewReader(conn2).ReadString('\n')
	if err != nil || reply2 != msg2 {
		t.Fatalf("reply2 = %q, want %q (err: %v)", reply2, msg2, err)
	}

	// 3. Dial invalid target missing port
	_, err = dialer.DialContext(context.Background(), "tcp", "invalid-target-no-port")
	if err == nil || !strings.Contains(err.Error(), "invalid dial target") {
		t.Fatalf("expected invalid dial target error, got: %v", err)
	}

	// 4. Dial empty target
	_, err = dialer.DialContext(context.Background(), "tcp", "")
	if err == nil || !strings.Contains(err.Error(), "target address is required") {
		t.Fatalf("expected target address is required error, got: %v", err)
	}
}

func TestTunnelDialer_UnreachableTarget(t *testing.T) {
	ts, err := NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.AddPassword("user", "pass")
	if err := ts.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer ts.Close()

	// Pick an unused local port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	closedPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	jump := JumpConfig{
		Host:         "127.0.0.1",
		Port:         ts.Port(),
		User:         "user",
		Password:     "pass",
		HostKeyCheck: false,
	}

	dialer, err := NewTunnelDialer(jump, fmt.Sprintf("127.0.0.1:%d", closedPort), 2*time.Second)
	if err != nil {
		t.Fatalf("NewTunnelDialer failed: %v", err)
	}
	defer dialer.Close()

	_, err = dialer.DialContext(context.Background(), "tcp", "")
	if err == nil {
		t.Fatal("expected error dialing unreachable target, got nil")
	}
}

func TestTunnelDialer_InvalidBastion(t *testing.T) {
	// Closed port for bastion
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	closedPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	jump := JumpConfig{
		Host:         "127.0.0.1",
		Port:         closedPort,
		User:         "user",
		Password:     "pass",
		HostKeyCheck: false,
	}

	dialer, err := NewTunnelDialer(jump, "127.0.0.1:8080", 2*time.Second)
	if err != nil {
		t.Fatalf("NewTunnelDialer failed: %v", err)
	}
	defer dialer.Close()

	_, err = dialer.DialContext(context.Background(), "tcp", "")
	if err == nil {
		t.Fatal("expected error connecting to closed bastion port, got nil")
	}
}

func TestTunnelDialer_ContextCancellation(t *testing.T) {
	jump := JumpConfig{
		Host:         "127.0.0.1",
		Port:         22,
		User:         "user",
		Password:     "pass",
		HostKeyCheck: false,
	}

	dialer, err := NewTunnelDialer(jump, "127.0.0.1:8080", 5*time.Second)
	if err != nil {
		t.Fatalf("NewTunnelDialer failed: %v", err)
	}
	defer dialer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = dialer.DialContext(ctx, "tcp", "")
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
}

func TestParseJumpDict(t *testing.T) {
	// Full jump configuration
	d := starlark.NewDict(10)
	d.SetKey(starlark.String("host"), starlark.String("bastion.corp.net"))
	d.SetKey(starlark.String("port"), starlark.MakeInt(2222))
	d.SetKey(starlark.String("user"), starlark.String("admin"))
	d.SetKey(starlark.String("key"), starlark.String("~/.ssh/id_rsa"))
	d.SetKey(starlark.String("password"), starlark.String("secret"))
	d.SetKey(starlark.String("passphrase"), starlark.String("keypass"))
	d.SetKey(starlark.String("use_agent"), starlark.False)
	d.SetKey(starlark.String("prompt"), starlark.True)
	d.SetKey(starlark.String("host_key_check"), starlark.False)
	d.SetKey(starlark.String("known_hosts_file"), starlark.String("/tmp/known_hosts"))

	cfg, err := ParseJumpDict(d)
	if err != nil {
		t.Fatalf("ParseJumpDict failed: %v", err)
	}

	if cfg.Host != "bastion.corp.net" {
		t.Errorf("Host = %q, want 'bastion.corp.net'", cfg.Host)
	}
	if cfg.Port != 2222 {
		t.Errorf("Port = %d, want 2222", cfg.Port)
	}
	if cfg.User != "admin" {
		t.Errorf("User = %q, want 'admin'", cfg.User)
	}
	if cfg.Key != "~/.ssh/id_rsa" {
		t.Errorf("Key = %q, want '~/.ssh/id_rsa'", cfg.Key)
	}
	if cfg.Password != "secret" {
		t.Errorf("Password = %q, want 'secret'", cfg.Password)
	}
	if cfg.Passphrase != "keypass" {
		t.Errorf("Passphrase = %q, want 'keypass'", cfg.Passphrase)
	}
	if cfg.UseAgent != false {
		t.Errorf("UseAgent = %v, want false", cfg.UseAgent)
	}
	if cfg.Prompt != true {
		t.Errorf("Prompt = %v, want true", cfg.Prompt)
	}
	if cfg.HostKeyCheck != false {
		t.Errorf("HostKeyCheck = %v, want false", cfg.HostKeyCheck)
	}
	if cfg.KnownHostsFile != "/tmp/known_hosts" {
		t.Errorf("KnownHostsFile = %q, want '/tmp/known_hosts'", cfg.KnownHostsFile)
	}

	// Fallback to targetAuth credentials
	targetAuth := AuthConfig{
		User:       "targetuser",
		Key:        "~/.ssh/target_key",
		Passphrase: "targetpassphrase",
		Password:   "targetpass",
		UseAgent:   true,
		Prompt:     true,
	}

	d2 := starlark.NewDict(1)
	d2.SetKey(starlark.String("host"), starlark.String("bastion2.corp.net"))

	cfg2, err := ParseJumpDict(d2, targetAuth)
	if err != nil {
		t.Fatalf("ParseJumpDict with targetAuth failed: %v", err)
	}
	if cfg2.Host != "bastion2.corp.net" {
		t.Errorf("Host = %q, want 'bastion2.corp.net'", cfg2.Host)
	}
	if cfg2.Port != 22 {
		t.Errorf("Port = %d, want 22 (default)", cfg2.Port)
	}
	if cfg2.User != "targetuser" {
		t.Errorf("User = %q, want 'targetuser' (inherited)", cfg2.User)
	}
	if cfg2.Key != "~/.ssh/target_key" {
		t.Errorf("Key = %q, want '~/.ssh/target_key' (inherited)", cfg2.Key)
	}
	if cfg2.Passphrase != "targetpassphrase" {
		t.Errorf("Passphrase = %q, want 'targetpassphrase' (inherited)", cfg2.Passphrase)
	}
	if cfg2.Password != "targetpass" {
		t.Errorf("Password = %q, want 'targetpass' (inherited)", cfg2.Password)
	}
	if !cfg2.UseAgent {
		t.Errorf("UseAgent = %v, want true (inherited)", cfg2.UseAgent)
	}
	if !cfg2.HostKeyCheck {
		t.Errorf("HostKeyCheck = %v, want true (default)", cfg2.HostKeyCheck)
	}

	// Missing host error
	dEmpty := starlark.NewDict(0)
	_, err = ParseJumpDict(dEmpty)
	if err == nil || !strings.Contains(err.Error(), "'host' is required") {
		t.Errorf("expected 'host' is required error, got: %v", err)
	}

	// Unexpected field error
	dInvalid := starlark.NewDict(2)
	dInvalid.SetKey(starlark.String("host"), starlark.String("bastion"))
	dInvalid.SetKey(starlark.String("unsupported_field"), starlark.String("val"))
	_, err = ParseJumpDict(dInvalid)
	if err == nil || !strings.Contains(err.Error(), "unexpected field") {
		t.Errorf("expected unexpected field error, got: %v", err)
	}
}
