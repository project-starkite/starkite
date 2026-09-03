package ssh

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var errKeyAcceptedProbe = errors.New("key_check: public key accepted by remote host")

// KeyCheckResult represents the outcome of probing a remote server for public key authorization.
type KeyCheckResult struct {
	Host        string
	User        string
	Port        int
	Accepted    bool
	KeyType     string
	Fingerprint string
	Ok          bool
	Error       string
}

func newSSHKeyCheckResult(r *KeyCheckResult) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHKeyCheckResult"), starlark.StringDict{
		"host":        starlark.String(r.Host),
		"user":        starlark.String(r.User),
		"port":        starlark.MakeInt(r.Port),
		"accepted":    starlark.Bool(r.Accepted),
		"key_type":    starlark.String(r.KeyType),
		"fingerprint": starlark.String(r.Fingerprint),
		"ok":          starlark.Bool(r.Ok),
		"error":       starlark.String(r.Error),
	})
}

// probeSigner is a sentinel ssh.Signer that intercepts SSH_MSG_USERAUTH_PK_OK.
type probeSigner struct {
	pub      gossh.PublicKey
	accepted *bool
}

func (s *probeSigner) PublicKey() gossh.PublicKey {
	return s.pub
}

func (s *probeSigner) Sign(rand io.Reader, data []byte) (*gossh.Signature, error) {
	// If the server invokes Sign, it has accepted the public key and is asking for a signature.
	*s.accepted = true
	return nil, errKeyAcceptedProbe
}

// resolveHostKeyCallback resolves the host key callback for known_hosts checking.
func resolveHostKeyCallback(hostKeyCheck bool, knownHostsPath string) (gossh.HostKeyCallback, error) {
	if !hostKeyCheck {
		return gossh.InsecureIgnoreHostKey(), nil
	}
	if knownHostsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home dir: %w", err)
		}
		knownHostsPath = filepath.Join(home, ".ssh", "known_hosts")
	} else {
		expanded, err := expandPath(knownHostsPath)
		if err != nil {
			return nil, err
		}
		knownHostsPath = expanded
	}
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("known_hosts file not found: %s (set host_key_check=False to disable)", knownHostsPath)
	}
	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse known_hosts: %w", err)
	}
	return callback, nil
}

// checkSingleHostKey probes a single host to determine if it accepts the given public key.
func checkSingleHostKey(targetAddr string, host string, user string, port int, pubKey gossh.PublicKey, timeout time.Duration, hostKeyCheck bool, knownHostsPath string, tunnel net.Conn) *KeyCheckResult {
	res := &KeyCheckResult{
		Host:        host,
		User:        user,
		Port:        port,
		KeyType:     pubKey.Type(),
		Fingerprint: gossh.FingerprintSHA256(pubKey),
	}

	var accepted bool
	signer := &probeSigner{
		pub:      pubKey,
		accepted: &accepted,
	}

	hkCallback, err := resolveHostKeyCallback(hostKeyCheck, knownHostsPath)
	if err != nil {
		res.Ok = false
		res.Error = err.Error()
		return res
	}

	config := &gossh.ClientConfig{
		User:            user,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: hkCallback,
		Timeout:         timeout,
	}

	var conn net.Conn
	if tunnel != nil {
		conn = tunnel
	} else {
		c, err := net.DialTimeout("tcp", targetAddr, timeout)
		if err != nil {
			res.Ok = false
			res.Error = fmt.Sprintf("connect to %s: %v", targetAddr, err)
			return res
		}
		conn = c
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	var timedOut atomic.Bool
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() {
			timedOut.Store(true)
			_ = conn.Close()
		})
		defer timer.Stop()
	}

	clientConn, _, _, err := gossh.NewClientConn(conn, targetAddr, config)
	if clientConn != nil {
		_ = clientConn.Close()
	}

	if accepted {
		res.Accepted = true
		res.Ok = true
		return res
	}

	if timedOut.Load() {
		res.Ok = false
		res.Error = fmt.Sprintf("connection to %s timed out after %v", targetAddr, timeout)
		return res
	}

	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unable to authenticate") || strings.Contains(errStr, "auth failed") || strings.Contains(errStr, "no supported methods remain") {
			res.Accepted = false
			res.Ok = true
			return res
		}
		res.Ok = false
		res.Error = errStr
		return res
	}

	res.Accepted = false
	res.Ok = true
	return res
}

// runKeyCheck executes the public key acceptance check across target hosts.
func runKeyCheck(hosts []string, defaultPort int, user string, pubKey gossh.PublicKey, timeout time.Duration, hostKeyCheck bool, knownHostsPath string, jump *jumpConfig) ([]*KeyCheckResult, error) {
	var jClient *gossh.Client
	if jump != nil && jump.Host != "" {
		jc, err := dialJumpClient(*jump, timeout)
		if err != nil {
			return nil, err
		}
		jClient = jc
		defer jClient.Close()
	}

	var results []*KeyCheckResult
	for _, target := range hosts {
		h, p := parseHostTarget(target, defaultPort)
		targetAddr := fmt.Sprintf("%s:%d", h, p)

		var tunnel net.Conn
		if jClient != nil {
			type dialResult struct {
				tunnel net.Conn
				err    error
			}
			dialDone := make(chan dialResult, 1)
			go func() {
				t, err := jClient.Dial("tcp", targetAddr)
				dialDone <- dialResult{tunnel: t, err: err}
			}()

			select {
			case <-time.After(timeout):
				results = append(results, &KeyCheckResult{
					Host:        h,
					User:        user,
					Port:        p,
					KeyType:     pubKey.Type(),
					Fingerprint: gossh.FingerprintSHA256(pubKey),
					Ok:          false,
					Error:       fmt.Sprintf("tunnel connection to %s timed out after %v", targetAddr, timeout),
				})
				continue
			case res := <-dialDone:
				if res.err != nil {
					results = append(results, &KeyCheckResult{
						Host:        h,
						User:        user,
						Port:        p,
						KeyType:     pubKey.Type(),
						Fingerprint: gossh.FingerprintSHA256(pubKey),
						Ok:          false,
						Error:       fmt.Sprintf("failed to tunnel through jump host: %v", res.err),
					})
					continue
				}
				tunnel = res.tunnel
			}
		}

		res := checkSingleHostKey(targetAddr, h, user, p, pubKey, timeout, hostKeyCheck, knownHostsPath, tunnel)
		results = append(results, res)
	}

	return results, nil
}

// sshCheckAuthorizedKey implements module-level `ssh.check_authorized_key(key=..., hosts=..., user=...)`.
func (m *Module) sshCheckAuthorizedKey(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawHosts starlark.Value
	var rawKey starlark.Value
	var jumpDict *starlark.Dict

	var p struct {
		Key          string `name:"key"`
		Hosts        string `name:"hosts"`
		User         string `name:"user"`
		Port         int    `name:"port"`
		Timeout      string `name:"timeout"`
		HostKeyCheck bool   `name:"host_key_check"`
	}

	p.Port = 22
	p.Timeout = "5s"
	p.HostKeyCheck = true

	var leftoverKwargs []starlark.Tuple
	for _, kv := range kwargs {
		k, ok := starlark.AsString(kv[0])
		if !ok {
			continue
		}
		switch k {
		case "key":
			rawKey = kv[1]
		case "hosts":
			rawHosts = kv[1]
		case "jump":
			if d, ok := kv[1].(*starlark.Dict); ok {
				jumpDict = d
			} else {
				return nil, fmt.Errorf("ssh.check_authorized_key: 'jump' must be a dict, got %s", kv[1].Type())
			}
		default:
			leftoverKwargs = append(leftoverKwargs, kv)
		}
	}

	if len(args) > 0 {
		rawKey = args[0]
		args = args[1:]
	}

	if err := startype.Args(args, leftoverKwargs).Go(&p); err != nil {
		return nil, err
	}

	keyArg := p.Key
	if rawKey != nil {
		if s, ok := starlark.AsString(rawKey); ok {
			keyArg = s
		} else {
			return nil, fmt.Errorf("ssh.check_authorized_key: 'key' must be a string, got %s", rawKey.Type())
		}
	}

	pubKeyStr, err := resolvePublicKey(keyArg, false)
	if err != nil {
		return nil, fmt.Errorf("ssh.check_authorized_key: %w", err)
	}

	pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return nil, fmt.Errorf("ssh.check_authorized_key: failed to parse public key: %w", err)
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
			return nil, fmt.Errorf("ssh.check_authorized_key: 'hosts' must be a string or list of strings, got %s", rawHosts.Type())
		}
	} else if p.Hosts != "" {
		hosts = append(hosts, p.Hosts)
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.check_authorized_key: 'hosts' is required and cannot be empty")
	}

	user := p.User
	if user == "" {
		user = defaultUser()
	}

	timeout := 5 * time.Second
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh.check_authorized_key: invalid timeout %q: %w", p.Timeout, err)
		}
		timeout = d
	}

	var jc *jumpConfig
	if jumpDict != nil {
		j, err := parseJumpDict(jumpDict, authConfig{})
		if err != nil {
			return nil, fmt.Errorf("ssh.check_authorized_key: %w", err)
		}
		jc = &j
	}

	checkResults, err := runKeyCheck(hosts, p.Port, user, pubKey, timeout, p.HostKeyCheck, "", jc)
	if err != nil {
		return nil, fmt.Errorf("ssh.check_authorized_key: %w", err)
	}

	items := make([]starlark.Value, len(checkResults))
	for i, r := range checkResults {
		items[i] = newSSHKeyCheckResult(r)
	}
	return starlark.NewList(items), nil
}

// sshKeyCheck is a backwards-compatible alias for sshCheckAuthorizedKey.
func (m *Module) sshKeyCheck(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return m.sshCheckAuthorizedKey(thread, fn, args, kwargs)
}

// checkAuthorizedKey implements client-level `client.check_authorized_key(key=...)`.
func (c *SSHClient) checkAuthorizedKey(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawHosts starlark.Value
	var rawKey starlark.Value

	var p struct {
		Key          string `name:"key"`
		Hosts        string `name:"hosts"`
		User         string `name:"user"`
		Port         int    `name:"port"`
		Timeout      string `name:"timeout"`
		HostKeyCheck bool   `name:"host_key_check"`
	}

	p.User = c.user
	p.Port = c.port
	p.Timeout = c.timeout.String()
	p.HostKeyCheck = c.hostKeyCheck

	var leftoverKwargs []starlark.Tuple
	for _, kv := range kwargs {
		k, ok := starlark.AsString(kv[0])
		if !ok {
			continue
		}
		switch k {
		case "key":
			rawKey = kv[1]
		case "hosts":
			rawHosts = kv[1]
		default:
			leftoverKwargs = append(leftoverKwargs, kv)
		}
	}

	if len(args) > 0 {
		rawKey = args[0]
		args = args[1:]
	}

	if err := startype.Args(args, leftoverKwargs).Go(&p); err != nil {
		return nil, err
	}

	keyArg := p.Key
	if rawKey != nil {
		if s, ok := starlark.AsString(rawKey); ok {
			keyArg = s
		}
	}

	pubKeyStr, err := resolvePublicKey(keyArg, c.useAgent)
	if err != nil {
		return nil, fmt.Errorf("ssh.client.check_authorized_key: %w", err)
	}

	pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return nil, fmt.Errorf("ssh.client.check_authorized_key: failed to parse public key: %w", err)
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
		return nil, fmt.Errorf("ssh.client.check_authorized_key: no target hosts configured")
	}

	user := p.User
	if user == "" {
		user = c.user
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

	checkResults, err := runKeyCheck(hosts, p.Port, user, pubKey, timeout, p.HostKeyCheck, c.knownHostsFile, jc)
	if err != nil {
		return nil, fmt.Errorf("ssh.client.check_authorized_key: %w", err)
	}

	items := make([]starlark.Value, len(checkResults))
	for i, r := range checkResults {
		items[i] = newSSHKeyCheckResult(r)
	}
	return starlark.NewList(items), nil
}

// keyCheck is a backwards-compatible alias for checkAuthorizedKey.
func (c *SSHClient) keyCheck(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return c.checkAuthorizedKey(thread, fn, args, kwargs)
}
