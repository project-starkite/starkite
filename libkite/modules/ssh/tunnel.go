package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// TunnelDialer establishes in-memory TCP connections through an SSH bastion.
// It implements standard ContextDialer signatures compatible with net/http transports,
// custom network dialers, and client connection pools.
type TunnelDialer struct {
	mu            sync.Mutex
	bastionClient *gossh.Client
	jumpConfig    JumpConfig
	targetAddr    string
	timeout       time.Duration
	closed        bool
}

// NewTunnelDialer creates a new TunnelDialer targeting an explicit host:port address or dynamic target.
// If targetAddr is non-empty, it must contain an explicit host and port; otherwise SplitHostPort errors out.
// If targetAddr is empty, DialContext uses the address passed to it dynamically.
func NewTunnelDialer(jumpCfg JumpConfig, targetAddr string, timeout time.Duration) (*TunnelDialer, error) {
	if jumpCfg.Host == "" {
		return nil, fmt.Errorf("tunnel dialer: jump host is required")
	}

	if targetAddr != "" {
		host, port, err := net.SplitHostPort(targetAddr)
		if err != nil {
			return nil, fmt.Errorf("tunnel dialer: invalid target address %q (must be host:port): %w", targetAddr, err)
		}
		if host == "" || port == "" {
			return nil, fmt.Errorf("tunnel dialer: target address %q must specify both host and port", targetAddr)
		}
		targetAddr = net.JoinHostPort(host, port)
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &TunnelDialer{
		jumpConfig: jumpCfg,
		targetAddr: targetAddr,
		timeout:    timeout,
	}, nil
}

// DialContext establishes a direct-tcpip channel through the bastion to the target address.
func (d *TunnelDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, fmt.Errorf("tunnel dialer: dialer is closed")
	}

	target := d.targetAddr
	if target == "" {
		target = address
	}
	if target == "" {
		return nil, fmt.Errorf("tunnel dialer: target address is required")
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("tunnel dialer: invalid dial target %q (must be host:port): %w", target, err)
	}
	target = net.JoinHostPort(host, port)

	if d.bastionClient == nil {
		client, err := DialBastion(ctx, &d.jumpConfig, d.timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to dial SSH bastion %s: %w", d.jumpConfig.Host, err)
		}
		d.bastionClient = client
	}

	conn, err := d.bastionClient.DialContext(ctx, "tcp", target)
	if err != nil {
		// Attempt one reconnect if the SSH connection was dropped
		_ = d.bastionClient.Close()
		d.bastionClient = nil

		client, dialErr := DialBastion(ctx, &d.jumpConfig, d.timeout)
		if dialErr != nil {
			return nil, fmt.Errorf("bastion tunnel dial failed after reconnect attempt: %w", err)
		}
		d.bastionClient = client
		return d.bastionClient.DialContext(ctx, "tcp", target)
	}

	return conn, nil
}

// Dial establishes a connection through the bastion to the target address using the background context.
func (d *TunnelDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// Close terminates the underlying bastion client connection and prevents subsequent dials.
func (d *TunnelDialer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.closed = true
	if d.bastionClient != nil {
		err := d.bastionClient.Close()
		d.bastionClient = nil
		return err
	}
	return nil
}

// TargetAddr returns the configured fixed target address, or empty string if dynamic.
func (d *TunnelDialer) TargetAddr() string {
	return d.targetAddr
}

// BuildJumpSSHConfig builds an *ssh.ClientConfig for connecting to a bastion host.
func BuildJumpSSHConfig(jump JumpConfig, timeout time.Duration) (*gossh.ClientConfig, error) {
	user := jump.User
	if user == "" {
		user = defaultUser()
	}

	config := &gossh.ClientConfig{
		User:    user,
		Timeout: timeout,
	}

	hostKeyCallback, err := resolveHostKeyCallback(jump.HostKeyCheck, jump.KnownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("bastion host key setup: %w", err)
	}
	config.HostKeyCallback = hostKeyCallback

	var authMethods []gossh.AuthMethod
	if jump.UseAgent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket != "" {
			conn, err := net.Dial("unix", socket)
			if err == nil {
				agentClient := agent.NewClient(conn)
				authMethods = append(authMethods, gossh.PublicKeysCallback(agentClient.Signers))
			}
		}
	}
	if jump.Key != "" {
		keyPath, err := expandPath(jump.Key)
		if err != nil {
			return nil, err
		}
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read bastion private key: %w", err)
		}
		signer, _, err := parsePrivateKeyWithPrompt(keyPath, key, jump.Passphrase, jump.Prompt, jump.UseAgent)
		if err != nil {
			return nil, fmt.Errorf("bastion host key: %w", err)
		}
		if signer != nil {
			authMethods = append(authMethods, gossh.PublicKeys(signer))
		}
	}
	if jump.Password != "" {
		authMethods = append(authMethods, gossh.Password(jump.Password))
	}

	if jump.Key == "" && jump.Password == "" && len(authMethods) == 0 {
		home, err := os.UserHomeDir()
		if err == nil {
			for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
				candidate := filepath.Join(home, ".ssh", name)
				if keyBytes, err := os.ReadFile(candidate); err == nil {
					if signer, _, err := parsePrivateKeyWithPrompt(candidate, keyBytes, jump.Passphrase, jump.Prompt, jump.UseAgent); err == nil && signer != nil {
						authMethods = append(authMethods, gossh.PublicKeys(signer))
						break
					}
				}
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method configured for bastion host %s", jump.Host)
	}

	config.Auth = authMethods
	return config, nil
}

// DialBastion establishes an authenticated SSH client connection to a bastion host.
func DialBastion(ctx context.Context, jump *JumpConfig, timeout time.Duration) (*gossh.Client, error) {
	if jump == nil || jump.Host == "" {
		return nil, fmt.Errorf("bastion host is required")
	}

	jumpPort := jump.Port
	if jumpPort <= 0 {
		jumpPort = 22
	}
	jumpAddr := jump.Host
	if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
		jumpAddr = fmt.Sprintf("%s:%d", jumpAddr, jumpPort)
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	config, err := BuildJumpSSHConfig(*jump, timeout)
	if err != nil {
		return nil, err
	}

	var d net.Dialer
	d.Timeout = timeout
	conn, err := d.DialContext(ctx, "tcp", jumpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to bastion %s: %w", jumpAddr, err)
	}

	clientConn, chans, reqs, err := gossh.NewClientConn(conn, jumpAddr, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to authenticate with bastion %s: %w", jumpAddr, err)
	}

	return gossh.NewClient(clientConn, chans, reqs), nil
}
