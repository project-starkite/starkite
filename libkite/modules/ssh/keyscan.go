package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKey represents an SSH host public key scanned from a remote server.
type HostKey struct {
	Host        string
	Port        int
	Type        string
	PublicKey   string
	Fingerprint string
	Line        string
	HashedLine  string
}

func newSSHHostKey(hk *HostKey) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHHostKey"), starlark.StringDict{
		"host":        starlark.String(hk.Host),
		"port":        starlark.MakeInt(hk.Port),
		"type":        starlark.String(hk.Type),
		"public_key":  starlark.String(hk.PublicKey),
		"fingerprint": starlark.String(hk.Fingerprint),
		"line":        starlark.String(hk.Line),
		"hashed_line": starlark.String(hk.HashedLine),
	})
}

// resolveKeyAlgorithms converts human-friendly algorithm names to SSH key algorithm identifiers.
func resolveKeyAlgorithms(algos []string) ([][]string, error) {
	if len(algos) == 0 {
		return [][]string{nil}, nil // nil means default negotiation
	}

	var groups [][]string
	for _, a := range algos {
		name := strings.ToLower(strings.TrimSpace(a))
		switch name {
		case "ed25519", "ssh-ed25519":
			groups = append(groups, []string{gossh.KeyAlgoED25519})
		case "rsa", "ssh-rsa":
			groups = append(groups, []string{"rsa-sha2-512", "rsa-sha2-256", gossh.KeyAlgoRSA})
		case "ecdsa":
			groups = append(groups, []string{gossh.KeyAlgoECDSA256, gossh.KeyAlgoECDSA384, gossh.KeyAlgoECDSA521})
		case "ecdsa-256", "ecdsa-p256", "ecdsa-sha2-nistp256":
			groups = append(groups, []string{gossh.KeyAlgoECDSA256})
		case "ecdsa-384", "ecdsa-p384", "ecdsa-sha2-nistp384":
			groups = append(groups, []string{gossh.KeyAlgoECDSA384})
		case "ecdsa-521", "ecdsa-p521", "ecdsa-sha2-nistp521":
			groups = append(groups, []string{gossh.KeyAlgoECDSA521})
		default:
			return nil, fmt.Errorf("ssh.keyscan: unsupported key algorithm %q (supported: ed25519, rsa, ecdsa)", a)
		}
	}
	return groups, nil
}

// parseHostTarget parses "host" or "host:port", returning the hostname and port.
func parseHostTarget(target string, defaultPort int) (string, int) {
	if defaultPort <= 0 {
		defaultPort = 22
	}
	h, pStr, err := net.SplitHostPort(target)
	if err == nil {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			return h, p
		}
		return h, defaultPort
	}
	return target, defaultPort
}

// scanSingleHostKey performs Diffie-Hellman key exchange and extracts the server's public host key.
func scanSingleHostKey(targetAddr string, host string, port int, algos []string, timeout time.Duration, tunnel net.Conn) (*HostKey, error) {
	var capturedKey gossh.PublicKey
	errCaptured := errors.New("keyscan: host key captured")

	config := &gossh.ClientConfig{
		User: "probe",
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error {
			capturedKey = key
			return errCaptured
		},
		Timeout: timeout,
	}
	if len(algos) > 0 {
		config.HostKeyAlgorithms = algos
	}

	var conn net.Conn
	var err error
	if tunnel != nil {
		conn = tunnel
	} else {
		conn, err = net.DialTimeout("tcp", targetAddr, timeout)
		if err != nil {
			return nil, fmt.Errorf("connect to %s: %w", targetAddr, err)
		}
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	clientConn, _, _, _ := gossh.NewClientConn(conn, targetAddr, config)
	if clientConn != nil {
		_ = clientConn.Close()
	}

	if capturedKey == nil {
		return nil, fmt.Errorf("failed to retrieve host key from %s", targetAddr)
	}

	normAddr := knownhosts.Normalize(targetAddr)
	unhashedLine := knownhosts.Line([]string{normAddr}, capturedKey)
	hashedLine := knownhosts.Line([]string{knownhosts.HashHostname(normAddr)}, capturedKey)
	pubKeyStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(capturedKey)))

	return &HostKey{
		Host:        host,
		Port:        port,
		Type:        capturedKey.Type(),
		PublicKey:   pubKeyStr,
		Fingerprint: gossh.FingerprintSHA256(capturedKey),
		Line:        unhashedLine,
		HashedLine:  hashedLine,
	}, nil
}

// saveHostKeys idempotently appends scanned host keys to a known_hosts file.
func saveHostKeys(filePath string, keys []*HostKey, hash bool) error {
	if filePath == "" {
		filePath = "~/.ssh/known_hosts"
	}
	expandedPath, err := expandPath(filePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(expandedPath), 0700); err != nil {
		return fmt.Errorf("failed to create directory for known_hosts: %w", err)
	}

	var existingContent []byte
	if data, err := os.ReadFile(expandedPath); err == nil {
		existingContent = data
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read known_hosts file %q: %w", expandedPath, err)
	}

	var toAppend []string
	for _, k := range keys {
		lineToAdd := k.Line
		if hash {
			lineToAdd = k.HashedLine
		}

		if bytes.Contains(existingContent, []byte(lineToAdd)) {
			continue
		}

		// Conflict check for unhashed lines
		if !hash {
			normAddr := knownhosts.Normalize(fmt.Sprintf("%s:%d", k.Host, k.Port))
			lines := strings.SplitSeq(string(existingContent), "\n")
			for l := range lines {
				l = strings.TrimSpace(l)
				if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "@") || strings.HasPrefix(l, "|") {
					continue
				}
				parts := strings.Fields(l)
				if len(parts) >= 3 {
					hostsPart := parts[0]
					typePart := parts[1]
					keyPart := parts[2]

					for h := range strings.SplitSeq(hostsPart, ",") {
						if h == normAddr && typePart == k.Type && !strings.Contains(k.PublicKey, keyPart) {
							return fmt.Errorf("host key conflict for %s (%s): existing key in %s differs from scanned key", k.Host, k.Type, expandedPath)
						}
					}
				}
			}
		}

		toAppend = append(toAppend, lineToAdd)
	}

	if len(toAppend) == 0 {
		return nil
	}

	f, err := os.OpenFile(expandedPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts %q: %w", expandedPath, err)
	}
	defer f.Close()

	if len(existingContent) > 0 && !bytes.HasSuffix(existingContent, []byte("\n")) {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	for _, l := range toAppend {
		if _, err := f.WriteString(l + "\n"); err != nil {
			return fmt.Errorf("failed to write to known_hosts: %w", err)
		}
	}

	return nil
}

// dialJumpClient connects to a bastion jump host and returns an authenticated client.
func dialJumpClient(jump jumpConfig, timeout time.Duration) (*gossh.Client, error) {
	jumpPort := jump.Port
	if jumpPort <= 0 {
		jumpPort = 22
	}
	jumpAddr := jump.Host
	if _, _, err := net.SplitHostPort(jumpAddr); err != nil {
		jumpAddr = fmt.Sprintf("%s:%d", jumpAddr, jumpPort)
	}

	ephemeralClient := &SSHClient{
		timeout:           timeout,
		hostKeyCheck:      false, // host key of jump is not strictly enforced in probe mode unless configured
		jumpHost:          jump.Host,
		jumpPort:          jumpPort,
		jumpUser:          jump.User,
		jumpKeyFile:       jump.Key,
		jumpKeyPassphrase: jump.Passphrase,
		jumpPassword:      jump.Password,
		jumpUseAgent:      jump.UseAgent,
		jumpPrompt:        jump.Prompt,
	}

	config, err := ephemeralClient.buildJumpSSHConfig()
	if err != nil {
		return nil, err
	}

	jClient, err := gossh.Dial("tcp", jumpAddr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to jump host %s: %w", jumpAddr, err)
	}
	return jClient, nil
}

// runKeyscan executes the host key discovery workflow for given targets.
func runKeyscan(hosts []string, defaultPort int, timeout time.Duration, algoGroups [][]string, save bool, path string, hash bool, jump *jumpConfig) ([]*HostKey, error) {
	var jClient *gossh.Client
	if jump != nil && jump.Host != "" {
		jc, err := dialJumpClient(*jump, timeout)
		if err != nil {
			return nil, err
		}
		jClient = jc
		defer jClient.Close()
	}

	var scannedKeys []*HostKey

	for _, target := range hosts {
		h, p := parseHostTarget(target, defaultPort)
		targetAddr := fmt.Sprintf("%s:%d", h, p)

		for _, algos := range algoGroups {
			var tunnel net.Conn
			if jClient != nil {
				t, err := jClient.Dial("tcp", targetAddr)
				if err != nil {
					return nil, fmt.Errorf("failed to tunnel through jump host to %s: %w", targetAddr, err)
				}
				tunnel = t
			}

			hk, err := scanSingleHostKey(targetAddr, h, p, algos, timeout, tunnel)
			if err != nil {
				return nil, err
			}
			scannedKeys = append(scannedKeys, hk)
		}
	}

	if save && len(scannedKeys) > 0 {
		if err := saveHostKeys(path, scannedKeys, hash); err != nil {
			return nil, err
		}
	}

	return scannedKeys, nil
}

// sshKeyscan implements module-level `ssh.keyscan(hosts, ...)`.
func (m *Module) sshKeyscan(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawHosts starlark.Value
	var rawTypes starlark.Value
	var jumpDict *starlark.Dict

	var p struct {
		Hosts   string `name:"hosts"`
		Port    int    `name:"port"`
		Timeout string `name:"timeout"`
		Type    string `name:"type"`
		Save    bool   `name:"save"`
		Path    string `name:"path"`
		Hash    bool   `name:"hash"`
	}

	p.Port = 22
	p.Timeout = "5s"
	p.Path = "~/.ssh/known_hosts"

	var leftoverKwargs []starlark.Tuple
	for _, kv := range kwargs {
		k, ok := starlark.AsString(kv[0])
		if !ok {
			continue
		}
		switch k {
		case "hosts":
			rawHosts = kv[1]
		case "types":
			rawTypes = kv[1]
		case "jump":
			if d, ok := kv[1].(*starlark.Dict); ok {
				jumpDict = d
			} else {
				return nil, fmt.Errorf("ssh.keyscan: 'jump' must be a dict, got %s", kv[1].Type())
			}
		default:
			leftoverKwargs = append(leftoverKwargs, kv)
		}
	}

	if len(args) > 0 {
		rawHosts = args[0]
		args = args[1:]
	}

	if err := startype.Args(args, leftoverKwargs).Go(&p); err != nil {
		return nil, err
	}

	var hosts []string
	if rawHosts != nil {
		switch h := rawHosts.(type) {
		case starlark.Sequence:
			iter := h.Iterate()
			defer iter.Done()
			var elem starlark.Value
			for iter.Next(&elem) {
				if s, ok := starlark.AsString(elem); ok && strings.TrimSpace(s) != "" {
					hosts = append(hosts, strings.TrimSpace(s))
				}
			}
		case starlark.String:
			if s := strings.TrimSpace(string(h)); s != "" {
				hosts = append(hosts, s)
			}
		default:
			return nil, fmt.Errorf("ssh.keyscan: 'hosts' must be a string or list of strings, got %s", rawHosts.Type())
		}
	} else if p.Hosts != "" {
		hosts = append(hosts, p.Hosts)
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.keyscan: 'hosts' is required and cannot be empty")
	}

	var typeList []string
	if p.Type != "" {
		typeList = append(typeList, p.Type)
	}
	if rawTypes != nil {
		if seq, ok := rawTypes.(starlark.Sequence); ok {
			iter := seq.Iterate()
			defer iter.Done()
			var elem starlark.Value
			for iter.Next(&elem) {
				if s, ok := starlark.AsString(elem); ok && strings.TrimSpace(s) != "" {
					typeList = append(typeList, strings.TrimSpace(s))
				}
			}
		} else {
			return nil, fmt.Errorf("ssh.keyscan: 'types' must be a list of strings, got %s", rawTypes.Type())
		}
	}

	algoGroups, err := resolveKeyAlgorithms(typeList)
	if err != nil {
		return nil, err
	}

	timeout := 5 * time.Second
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh.keyscan: invalid timeout %q: %w", p.Timeout, err)
		}
		timeout = d
	}

	var jc *jumpConfig
	if jumpDict != nil {
		j, err := parseJumpDict(jumpDict, authConfig{})
		if err != nil {
			return nil, fmt.Errorf("ssh.keyscan: %w", err)
		}
		jc = &j
	}

	keys, err := runKeyscan(hosts, p.Port, timeout, algoGroups, p.Save, p.Path, p.Hash, jc)
	if err != nil {
		return nil, fmt.Errorf("ssh.keyscan: %w", err)
	}

	items := make([]starlark.Value, 0, len(keys))
	for _, k := range keys {
		items = append(items, newSSHHostKey(k))
	}
	return starlark.NewList(items), nil
}

// keyscan implements client-level `client.keyscan(...)`.
func (c *SSHClient) keyscan(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawHosts starlark.Value
	var rawTypes starlark.Value

	var p struct {
		Hosts   string `name:"hosts"`
		Port    int    `name:"port"`
		Timeout string `name:"timeout"`
		Type    string `name:"type"`
		Save    bool   `name:"save"`
		Path    string `name:"path"`
		Hash    bool   `name:"hash"`
	}

	p.Port = c.port
	p.Timeout = c.timeout.String()
	p.Path = c.knownHostsFile
	if p.Path == "" {
		p.Path = "~/.ssh/known_hosts"
	}

	var leftoverKwargs []starlark.Tuple
	for _, kv := range kwargs {
		k, ok := starlark.AsString(kv[0])
		if !ok {
			continue
		}
		switch k {
		case "hosts":
			rawHosts = kv[1]
		case "types":
			rawTypes = kv[1]
		default:
			leftoverKwargs = append(leftoverKwargs, kv)
		}
	}

	if len(args) > 0 {
		rawHosts = args[0]
		args = args[1:]
	}

	if err := startype.Args(args, leftoverKwargs).Go(&p); err != nil {
		return nil, err
	}

	var hosts []string
	if rawHosts != nil {
		switch h := rawHosts.(type) {
		case starlark.Sequence:
			iter := h.Iterate()
			defer iter.Done()
			var elem starlark.Value
			for iter.Next(&elem) {
				if s, ok := starlark.AsString(elem); ok && strings.TrimSpace(s) != "" {
					hosts = append(hosts, strings.TrimSpace(s))
				}
			}
		case starlark.String:
			if s := strings.TrimSpace(string(h)); s != "" {
				hosts = append(hosts, s)
			}
		}
	} else if p.Hosts != "" {
		hosts = append(hosts, p.Hosts)
	} else {
		hosts = append(hosts, c.hosts...)
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.client.keyscan: no target hosts configured")
	}

	var typeList []string
	if p.Type != "" {
		typeList = append(typeList, p.Type)
	}
	if rawTypes != nil {
		if seq, ok := rawTypes.(starlark.Sequence); ok {
			iter := seq.Iterate()
			defer iter.Done()
			var elem starlark.Value
			for iter.Next(&elem) {
				if s, ok := starlark.AsString(elem); ok && strings.TrimSpace(s) != "" {
					typeList = append(typeList, strings.TrimSpace(s))
				}
			}
		}
	}

	algoGroups, err := resolveKeyAlgorithms(typeList)
	if err != nil {
		return nil, err
	}

	timeout := c.timeout
	if p.Timeout != "" {
		if d, err := time.ParseDuration(p.Timeout); err == nil {
			timeout = d
		}
	}

	var jc *jumpConfig
	if c.jumpHost != "" {
		jc = &jumpConfig{
			Host:       c.jumpHost,
			Port:       c.jumpPort,
			User:       c.jumpUser,
			Key:        c.jumpKeyFile,
			Passphrase: c.jumpKeyPassphrase,
			Password:   c.jumpPassword,
			UseAgent:   c.jumpUseAgent,
			Prompt:     c.jumpPrompt,
		}
	}

	keys, err := runKeyscan(hosts, p.Port, timeout, algoGroups, p.Save, p.Path, p.Hash, jc)
	if err != nil {
		return nil, fmt.Errorf("ssh.client.keyscan: %w", err)
	}

	items := make([]starlark.Value, 0, len(keys))
	for _, k := range keys {
		items = append(items, newSSHHostKey(k))
	}
	return starlark.NewList(items), nil
}
