package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"

	"github.com/project-starkite/starkite/libkite"
)

// agentSocketSigner wraps an ssh.PublicKey from ssh-agent and opens/closes
// the SSH_AUTH_SOCK unix socket per Sign call so no socket descriptor leaks.
type agentSocketSigner struct {
	socket string
	pub    ssh.PublicKey
}

func (s *agentSocketSigner) PublicKey() ssh.PublicKey {
	return s.pub
}

func (s *agentSocketSigner) Sign(rand io.Reader, data []byte) (*ssh.Signature, error) {
	c, err := net.Dial("unix", s.socket)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return agent.NewClient(c).Sign(s.pub, data)
}

func (s *agentSocketSigner) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	c, err := net.Dial("unix", s.socket)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var flags agent.SignatureFlags
	switch algorithm {
	case ssh.KeyAlgoRSASHA256:
		flags = agent.SignatureFlagRsaSha256
	case ssh.KeyAlgoRSASHA512:
		flags = agent.SignatureFlagRsaSha512
	}
	return agent.NewClient(c).SignWithFlags(s.pub, data, flags)
}

func newAgentAuthMethod(socket string) (ssh.AuthMethod, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("ssh: failed to connect to SSH_AUTH_SOCK %q: %w", socket, err)
	}
	_ = conn.Close()

	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		c, err := net.Dial("unix", socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		signers, err := agent.NewClient(c).Signers()
		if err != nil {
			return nil, err
		}
		wrapped := make([]ssh.Signer, len(signers))
		for i, s := range signers {
			wrapped[i] = &agentSocketSigner{socket: socket, pub: s.PublicKey()}
		}
		return wrapped, nil
	}), nil
}

// managedSSHConn wraps an ssh.Conn so that closing the *ssh.Client
// deterministically stops its keepalive goroutine and closes any underlying
// bastion jump client.
type managedSSHConn struct {
	ssh.Conn
	jumpClient    *ssh.Client
	keepaliveStop chan struct{}
	closeOnce     sync.Once
}

func (m *managedSSHConn) Close() error {
	err := m.Conn.Close()
	m.closeOnce.Do(func() {
		if m.keepaliveStop != nil {
			close(m.keepaliveStop)
		}
		if m.jumpClient != nil {
			_ = m.jumpClient.Close()
		}
	})
	return err
}

// expandPath expands leading ~ or ~/ or ~\ to the user's home directory.
func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory for path %q: %w", path, err)
		}
		if path == "~" {
			return home, nil
		}
		if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
			return filepath.Join(home, path[2:]), nil
		}
	}
	return path, nil
}

// parsePrivateKeyWithPrompt parses a private key from raw bytes.
// If the key is passphrase-protected and no passphrase was supplied:
// - if useAgent=true, returns (nil, "", nil) delegating authentication to ssh-agent.
// - if prompt=true and stdin is a TTY, prompts the operator in the terminal.
// - if prompt=true and stdin is NOT a TTY, returns an error.
// - if prompt=false, returns an actionable error.
func parsePrivateKeyWithPrompt(keyPath string, keyBytes []byte, passphrase string, prompt bool, useAgent bool) (ssh.Signer, string, error) {
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
		if err != nil {
			return nil, passphrase, fmt.Errorf("failed to parse private key %q with passphrase: %w", keyPath, err)
		}
		return signer, passphrase, nil
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err == nil {
		return signer, "", nil
	}

	if _, isMissing := err.(*ssh.PassphraseMissingError); isMissing {
		if useAgent {
			return nil, "", nil
		}
		if prompt {
			stdinFd := int(os.Stdin.Fd())
			if term.IsTerminal(stdinFd) {
				fmt.Fprintf(os.Stderr, "Enter passphrase for key %q: ", keyPath)
				passBytes, readErr := term.ReadPassword(stdinFd)
				fmt.Fprintln(os.Stderr)
				if readErr != nil {
					return nil, "", fmt.Errorf("failed to read key passphrase: %w", readErr)
				}
				enteredPass := string(passBytes)
				signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, passBytes)
				if err != nil {
					return nil, "", fmt.Errorf("failed to decrypt private key %q: %w", keyPath, err)
				}
				return signer, enteredPass, nil
			}
			return nil, "", fmt.Errorf("private key %q: prompt=True specified but standard input is not a terminal", keyPath)
		}
		return nil, "", fmt.Errorf("private key %q is passphrase protected (supply passphrase, set prompt=True, or use use_agent=True)", keyPath)
	}

	return nil, "", fmt.Errorf("failed to parse private key %q: %w", keyPath, err)
}

// buildSSHConfig creates an *ssh.ClientConfig from the SSHClient's settings.
func (c *SSHClient) buildSSHConfig() (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:    c.user,
		Timeout: c.timeout,
	}

	// Host key verification
	hostKeyCallback, err := c.hostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("host key setup: %w", err)
	}
	config.HostKeyCallback = hostKeyCallback

	// Authentication methods
	var authMethods []ssh.AuthMethod

	// SSH agent auth
	if c.useAgent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, fmt.Errorf("ssh: use_agent=True but SSH_AUTH_SOCK is not set")
		}
		authMethod, err := newAgentAuthMethod(socket)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, authMethod)
	}

	// Key-based auth
	if c.keyFile != "" {
		keyPath, err := expandPath(c.keyFile)
		if err != nil {
			return nil, err
		}
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		c.mu.Lock()
		signer, usedPass, err := parsePrivateKeyWithPrompt(keyPath, key, c.keyPassphrase, c.prompt, c.useAgent)
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		if c.keyPassphrase == "" && usedPass != "" {
			c.keyPassphrase = usedPass
		}
		c.mu.Unlock()
		if signer != nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	// Password auth
	if c.password != "" {
		authMethods = append(authMethods, ssh.Password(c.password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method configured")
	}

	config.Auth = authMethods
	return config, nil
}

// hostKeyCallback returns the appropriate host key callback based on config.
func (c *SSHClient) hostKeyCallback() (ssh.HostKeyCallback, error) {
	return resolveHostKeyCallback(c.hostKeyCheck, c.knownHostsFile)
}

// dialHost connects to a host, routing through a jump host if configured.
func (c *SSHClient) dialHost(host string) (*ssh.Client, error) {
	config, err := c.buildSSHConfig()
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", host, c.port)

	if c.jumpHost != "" {
		return c.dialViaJump(addr, config)
	}

	ctx := libkite.ThreadContext(c.thread)
	dialer := net.Dialer{Timeout: c.timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	var timedOut atomic.Bool
	if c.timeout > 0 {
		timer := time.AfterFunc(c.timeout, func() {
			timedOut.Store(true)
			_ = rawConn.Close()
		})
		defer timer.Stop()
	}

	conn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, config)
	if err != nil {
		_ = rawConn.Close()
		if timedOut.Load() {
			return nil, fmt.Errorf("connection to %s timed out after %v", addr, c.timeout)
		}
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	managed := &managedSSHConn{
		Conn:          conn,
		keepaliveStop: make(chan struct{}),
	}
	return ssh.NewClient(managed, chans, reqs), nil
}

// buildJumpSSHConfig creates an *ssh.ClientConfig for connecting to the jump host.
func (c *SSHClient) buildJumpSSHConfig() (*ssh.ClientConfig, error) {
	user := c.jumpUser
	if user == "" {
		user = c.user
	}
	keyFile := c.jumpKeyFile
	if keyFile == "" {
		keyFile = c.keyFile
	}
	password := c.jumpPassword
	if password == "" {
		password = c.password
	}

	config := &ssh.ClientConfig{
		User:    user,
		Timeout: c.timeout,
	}

	hostKeyCheck := c.hostKeyCheck
	knownHostsPath := c.knownHostsFile
	if c.jumpHost != "" {
		if !c.jumpHostKeyCheck {
			hostKeyCheck = false
		}
		if c.jumpKnownHostsFile != "" {
			knownHostsPath = c.jumpKnownHostsFile
		}
	}
	hostKeyCallback, err := resolveHostKeyCallback(hostKeyCheck, knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("jump host key setup: %w", err)
	}
	config.HostKeyCallback = hostKeyCallback

	var authMethods []ssh.AuthMethod
	if c.jumpUseAgent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket != "" {
			if authMethod, err := newAgentAuthMethod(socket); err == nil {
				authMethods = append(authMethods, authMethod)
			}
		}
	}
	if keyFile != "" {
		keyPath, err := expandPath(keyFile)
		if err != nil {
			return nil, err
		}
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read jump private key: %w", err)
		}
		c.mu.Lock()
		jumpPass := c.jumpKeyPassphrase
		if jumpPass == "" {
			jumpPass = c.keyPassphrase
		}
		signer, usedPass, err := parsePrivateKeyWithPrompt(keyPath, key, jumpPass, c.jumpPrompt, c.jumpUseAgent)
		if err != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("jump host key: %w", err)
		}
		if c.jumpKeyPassphrase == "" && usedPass != "" {
			c.jumpKeyPassphrase = usedPass
		}
		c.mu.Unlock()
		if signer != nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method configured for jump host")
	}

	config.Auth = authMethods
	return config, nil
}

// dialViaJump connects to a target host through a jump/bastion host.
func (c *SSHClient) dialViaJump(targetAddr string, targetConfig *ssh.ClientConfig) (*ssh.Client, error) {
	jumpConfig, err := c.buildJumpSSHConfig()
	if err != nil {
		return nil, err
	}

	jumpPort := c.jumpPort
	if jumpPort <= 0 {
		jumpPort = c.port
	}
	if jumpPort <= 0 {
		jumpPort = 22
	}

	jumpAddr := c.jumpHost
	if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
		jumpAddr = fmt.Sprintf("%s:%d", jumpAddr, jumpPort)
	}

	ctx := libkite.ThreadContext(c.thread)
	dialer := net.Dialer{Timeout: c.timeout}
	jumpRawConn, err := dialer.DialContext(ctx, "tcp", jumpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to jump host %s: %w", jumpAddr, err)
	}
	jumpConn, jumpChans, jumpReqs, err := ssh.NewClientConn(jumpRawConn, jumpAddr, jumpConfig)
	if err != nil {
		_ = jumpRawConn.Close()
		return nil, fmt.Errorf("failed to connect to jump host %s: %w", jumpAddr, err)
	}
	jumpClient := ssh.NewClient(jumpConn, jumpChans, jumpReqs)

	// Create a tunnel from jump host to target
	type dialResult struct {
		tunnel net.Conn
		err    error
	}
	dialDone := make(chan dialResult, 1)
	go func() {
		t, err := jumpClient.Dial("tcp", targetAddr)
		dialDone <- dialResult{tunnel: t, err: err}
	}()

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()

	var tunnel net.Conn
	select {
	case <-timer.C:
		_ = jumpClient.Close()
		return nil, fmt.Errorf("connection to %s via %s timed out after %v", targetAddr, jumpAddr, c.timeout)
	case <-ctx.Done():
		_ = jumpClient.Close()
		return nil, fmt.Errorf("connection to %s via %s canceled: %w", targetAddr, jumpAddr, ctx.Err())
	case res := <-dialDone:
		if res.err != nil {
			_ = jumpClient.Close()
			return nil, fmt.Errorf("failed to tunnel from %s to %s: %w", jumpAddr, targetAddr, res.err)
		}
		tunnel = res.tunnel
	}

	// Create an SSH connection over the tunnel
	var timedOut atomic.Bool
	if c.timeout > 0 {
		hsTimer := time.AfterFunc(c.timeout, func() {
			timedOut.Store(true)
			_ = tunnel.Close()
		})
		defer hsTimer.Stop()
	}

	conn, chans, reqs, err := ssh.NewClientConn(tunnel, targetAddr, targetConfig)
	if err != nil {
		_ = tunnel.Close()
		_ = jumpClient.Close()
		if timedOut.Load() {
			return nil, fmt.Errorf("connection to %s via %s timed out after %v", targetAddr, jumpAddr, c.timeout)
		}
		return nil, fmt.Errorf("failed to establish SSH over tunnel to %s: %w", targetAddr, err)
	}

	managed := &managedSSHConn{
		Conn:          conn,
		jumpClient:    jumpClient,
		keepaliveStop: make(chan struct{}),
	}
	return ssh.NewClient(managed, chans, reqs), nil
}

// dialHostWithRetry wraps dialHost with exponential backoff retries.
func (c *SSHClient) dialHostWithRetry(host string) (*ssh.Client, error) {
	var lastErr error
	delay := time.Second
	ctx := libkite.ThreadContext(c.thread)

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, err := c.dialHost(host)
		if err == nil {
			c.startKeepalive(client)
			return client, nil
		}
		lastErr = err

		if attempt < c.maxRetries {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			delay *= 2
		}
	}
	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// startKeepalive sends periodic keepalive requests on the connection.
// Keepalive runs post-connection via global requests, not via ssh.ClientConfig
// (which only covers handshake/auth). This matches OpenSSH's keepalive behavior.
// It closes the client after keepAliveMax consecutive failures, or exits
// immediately when the client connection is closed.
func (c *SSHClient) startKeepalive(client *ssh.Client) {
	if c.keepAliveInterval <= 0 {
		return
	}

	var stopCh <-chan struct{}
	if mc, ok := client.Conn.(*managedSSHConn); ok {
		stopCh = mc.keepaliveStop
	}

	go func() {
		failures := 0
		ticker := time.NewTicker(c.keepAliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					failures++
					if failures >= c.keepAliveMax {
						_ = client.Close()
						return
					}
				} else {
					failures = 0
				}
			}
		}
	}()
}
