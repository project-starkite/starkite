package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

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
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("ssh: failed to connect to SSH_AUTH_SOCK %q: %w", socket, err)
		}
		agentClient := agent.NewClient(conn)
		authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
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

		signer, usedPass, err := parsePrivateKeyWithPrompt(keyPath, key, c.keyPassphrase, c.prompt, c.useAgent)
		if err != nil {
			return nil, err
		}
		if c.keyPassphrase == "" && usedPass != "" {
			c.keyPassphrase = usedPass
		}
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

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	return client, nil
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
			conn, err := net.Dial("unix", socket)
			if err == nil {
				agentClient := agent.NewClient(conn)
				authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
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
		jumpPass := c.jumpKeyPassphrase
		if jumpPass == "" {
			jumpPass = c.keyPassphrase
		}
		signer, usedPass, err := parsePrivateKeyWithPrompt(keyPath, key, jumpPass, c.jumpPrompt, c.jumpUseAgent)
		if err != nil {
			return nil, fmt.Errorf("jump host key: %w", err)
		}
		if c.jumpKeyPassphrase == "" && usedPass != "" {
			c.jumpKeyPassphrase = usedPass
		}
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

	jumpClient, err := ssh.Dial("tcp", jumpAddr, jumpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to jump host %s: %w", jumpAddr, err)
	}

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

	var tunnel net.Conn
	select {
	case <-time.After(c.timeout):
		jumpClient.Close()
		return nil, fmt.Errorf("connection to %s via %s timed out after %v", targetAddr, jumpAddr, c.timeout)
	case res := <-dialDone:
		if res.err != nil {
			jumpClient.Close()
			return nil, fmt.Errorf("failed to tunnel from %s to %s: %w", jumpAddr, targetAddr, res.err)
		}
		tunnel = res.tunnel
	}

	// Create an SSH connection over the tunnel
	var timedOut atomic.Bool
	if c.timeout > 0 {
		timer := time.AfterFunc(c.timeout, func() {
			timedOut.Store(true)
			_ = tunnel.Close()
		})
		defer timer.Stop()
	}

	conn, chans, reqs, err := ssh.NewClientConn(tunnel, targetAddr, targetConfig)
	if err != nil {
		tunnel.Close()
		jumpClient.Close()
		if timedOut.Load() {
			return nil, fmt.Errorf("connection to %s via %s timed out after %v", targetAddr, jumpAddr, c.timeout)
		}
		return nil, fmt.Errorf("failed to establish SSH over tunnel to %s: %w", targetAddr, err)
	}

	return ssh.NewClient(conn, chans, reqs), nil
}

// dialHostWithRetry wraps dialHost with exponential backoff retries.
func (c *SSHClient) dialHostWithRetry(host string) (*ssh.Client, error) {
	var lastErr error
	delay := time.Second

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		client, err := c.dialHost(host)
		if err == nil {
			c.startKeepalive(client)
			return client, nil
		}
		lastErr = err

		if attempt < c.maxRetries {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// startKeepalive sends periodic keepalive requests on the connection.
// Keepalive runs post-connection via global requests, not via ssh.ClientConfig
// (which only covers handshake/auth). This matches OpenSSH's keepalive behavior.
// It closes the client after keepAliveMax consecutive failures.
func (c *SSHClient) startKeepalive(client *ssh.Client) {
	if c.keepAliveInterval <= 0 {
		return
	}

	go func() {
		failures := 0
		ticker := time.NewTicker(c.keepAliveInterval)
		defer ticker.Stop()

		for range ticker.C {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				failures++
				if failures >= c.keepAliveMax {
					client.Close()
					return
				}
			} else {
				failures = 0
			}
		}
	}()
}
