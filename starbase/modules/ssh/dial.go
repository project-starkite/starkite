package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

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

	// Key-based auth
	if c.keyFile != "" {
		key, err := os.ReadFile(c.keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		var signer ssh.Signer
		if c.keyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(c.keyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Password auth
	if c.password != "" {
		authMethods = append(authMethods, ssh.Password(c.password))
	}

	// SSH agent auth
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method configured")
	}

	config.Auth = authMethods
	return config, nil
}

// hostKeyCallback returns the appropriate host key callback based on config.
func (c *SSHClient) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if !c.hostKeyCheck {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	khFile := c.knownHostsFile
	if khFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		khFile = filepath.Join(home, ".ssh", "known_hosts")
	}

	callback, err := knownhosts.New(khFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts %q: %w", khFile, err)
	}
	return callback, nil
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

// dialViaJump connects to a target host through a jump/bastion host.
func (c *SSHClient) dialViaJump(targetAddr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	// Dial the jump host
	jumpAddr := c.jumpHost
	if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
		jumpAddr = fmt.Sprintf("%s:%d", jumpAddr, c.port)
	}

	jumpClient, err := ssh.Dial("tcp", jumpAddr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to jump host %s: %w", jumpAddr, err)
	}

	// Create a tunnel from jump host to target
	tunnel, err := jumpClient.Dial("tcp", targetAddr)
	if err != nil {
		jumpClient.Close()
		return nil, fmt.Errorf("failed to tunnel from %s to %s: %w", jumpAddr, targetAddr, err)
	}

	// Create an SSH connection over the tunnel
	conn, chans, reqs, err := ssh.NewClientConn(tunnel, targetAddr, config)
	if err != nil {
		tunnel.Close()
		jumpClient.Close()
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
