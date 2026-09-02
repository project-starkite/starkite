package ssh

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// resolvePublicKey resolves and validates the public key string from an argument,
// ssh-agent (when useAgent=True), or default local files.
func resolvePublicKey(keyArg string, useAgent bool) (string, error) {
	if keyArg != "" {
		targetPath := keyArg
		if strings.HasPrefix(targetPath, "~") {
			home, err := os.UserHomeDir()
			if err == nil {
				if targetPath == "~" {
					targetPath = home
				} else if strings.HasPrefix(targetPath, "~/") {
					targetPath = filepath.Join(home, targetPath[2:])
				}
			}
		}

		// If it's a file on disk, read it
		if data, err := os.ReadFile(targetPath); err == nil {
			pubKey, comment, _, _, err := ssh.ParseAuthorizedKey(data)
			if err != nil {
				return "", fmt.Errorf("ssh.copy_id: failed to parse public key file %q: %w", targetPath, err)
			}
			pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pubKey)))
			if comment != "" {
				pubStr = fmt.Sprintf("%s %s", pubStr, comment)
			}
			return pubStr, nil
		}

		// Otherwise parse as raw public key string
		pubKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(keyArg))
		if err != nil {
			return "", fmt.Errorf("ssh.copy_id: invalid public key format: %w", err)
		}
		pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pubKey)))
		if comment != "" {
			pubStr = fmt.Sprintf("%s %s", pubStr, comment)
		}
		return pubStr, nil
	}

	// If keyArg is empty and useAgent is True, query ssh-agent
	if useAgent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return "", fmt.Errorf("ssh.copy_id: use_agent=True but SSH_AUTH_SOCK is not set")
		}
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return "", fmt.Errorf("ssh.copy_id: failed to connect to SSH_AUTH_SOCK %q: %w", socket, err)
		}
		defer conn.Close()
		agentClient := agent.NewClient(conn)
		keys, err := agentClient.List()
		if err != nil {
			return "", fmt.Errorf("ssh.copy_id: failed to list ssh-agent keys: %w", err)
		}
		if len(keys) == 0 {
			return "", fmt.Errorf("ssh.copy_id: use_agent=True but no identities found in ssh-agent")
		}
		pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(keys[0])))
		if keys[0].Comment != "" {
			pubStr = fmt.Sprintf("%s %s", pubStr, keys[0].Comment)
		}
		return pubStr, nil
	}

	// Fallback to standard local public key files
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []string{
			filepath.Join(home, ".ssh", "id_ed25519.pub"),
			filepath.Join(home, ".ssh", "id_rsa.pub"),
			filepath.Join(home, ".ssh", "id_ecdsa.pub"),
		}
		for _, c := range candidates {
			if data, err := os.ReadFile(c); err == nil {
				if pubKey, comment, _, _, err := ssh.ParseAuthorizedKey(data); err == nil {
					pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pubKey)))
					if comment != "" {
						pubStr = fmt.Sprintf("%s %s", pubStr, comment)
					}
					return pubStr, nil
				}
			}
		}
	}

	return "", fmt.Errorf("ssh.copy_id: no public key provided and no default key found in ~/.ssh/ (use use_agent=True to load from ssh-agent)")
}

// promptPasswordIfNeeded prompts the operator in the terminal if password is required.
func promptPasswordIfNeeded(c *SSHClient, prompt bool) (string, error) {
	if c.password != "" && !prompt {
		return c.password, nil
	}

	if (c.keyFile != "" || (c.useAgent && os.Getenv("SSH_AUTH_SOCK") != "")) && !prompt {
		return "", nil
	}

	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		hostSummary := strings.Join(c.hosts, ", ")
		if len(c.hosts) > 3 {
			hostSummary = fmt.Sprintf("%s (+%d more)", strings.Join(c.hosts[:3], ", "), len(c.hosts)-3)
		}
		fmt.Fprintf(os.Stderr, "[ssh.copy_id] Enter password for %s@%s: ", c.user, hostSummary)
		passBytes, err := term.ReadPassword(stdinFd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		return string(passBytes), nil
	}

	if prompt {
		return "", fmt.Errorf("ssh.copy_id: prompt=True specified but standard input is not a terminal")
	}

	if c.password == "" && c.keyFile == "" && !c.useAgent {
		return "", fmt.Errorf("ssh.copy_id: remote host password required for %s (pass password or set prompt=True)", c.hosts)
	}

	return "", nil
}

// buildCopyIdCommand generates a robust, idempotent shell script for key installation.
func buildCopyIdCommand(pubKey string, asUser string) string {
	encodedKey := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(pubKey)))

	homeSetup := ""
	chownSetup := ""
	if asUser != "" {
		homeSetup = fmt.Sprintf(`TARGET_HOME=$(eval echo ~%s)`, asUser)
		chownSetup = fmt.Sprintf(`chown -R %s:%s "${TARGET_SSH}" 2>/dev/null || true`, asUser, asUser)
	}

	return fmt.Sprintf(`sh -c '
KEY=$(echo "%s" | base64 --decode)
TARGET_HOME="${HOME}"
%s
TARGET_SSH="${TARGET_HOME}/.ssh"
TARGET_AUTH="${TARGET_SSH}/authorized_keys"
mkdir -p "${TARGET_SSH}" && chmod 700 "${TARGET_SSH}"
touch "${TARGET_AUTH}" && chmod 600 "${TARGET_AUTH}"
if ! grep -qxF "${KEY}" "${TARGET_AUTH}" 2>/dev/null; then
    printf "%%s\n" "${KEY}" >> "${TARGET_AUTH}"
fi
chmod 600 "${TARGET_AUTH}"
chmod 700 "${TARGET_SSH}"
%s
if command -v restorecon >/dev/null 2>&1; then
    restorecon -R "${TARGET_SSH}" 2>/dev/null || true
fi
'`, encodedKey, homeSetup, chownSetup)
}

// copyId installs a public key onto the target fleet hosts.
func (c *SSHClient) copyId(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Key      string `name:"key" position:"0"`
		AsUser   string `name:"as_user"`
		Sudo     bool   `name:"sudo"`
		Prompt   bool   `name:"prompt"`
		KeyCheck bool   `name:"key_check"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	// 1. Resolve Public Key
	pubKey, err := resolvePublicKey(p.Key, c.useAgent)
	if err != nil {
		return nil, err
	}

	if c.dryRun {
		results := make([]starlark.Value, len(c.hosts))
		for i, host := range c.hosts {
			results[i] = newSSHResult(host, "copy_id", fmt.Sprintf("[DRY RUN] Would install public key on %s: %s", host, pubKey), "", 0, true, true)
		}
		return starlark.NewList(results), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	asUser := p.AsUser
	if asUser == "" {
		asUser = c.defaultAsUser
	}
	sudo := p.Sudo || c.defaultSudo

	// 2. Pre-flight Key Check (if key_check is enabled)
	alreadyAccepted := make(map[string]bool)
	var pendingHosts []string

	if p.KeyCheck {
		parsedPub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err == nil {
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

			probeUser := c.user
			if asUser != "" {
				probeUser = asUser
			}

			checkResults, _ := runKeyCheck(c.hosts, c.port, probeUser, parsedPub, c.timeout, c.hostKeyCheck, c.knownHostsFile, jc)
			for _, cr := range checkResults {
				if cr.Accepted {
					alreadyAccepted[cr.Host] = true
				} else {
					pendingHosts = append(pendingHosts, cr.Host)
				}
			}
		} else {
			pendingHosts = append(pendingHosts, c.hosts...)
		}
	} else {
		pendingHosts = append(pendingHosts, c.hosts...)
	}

	// If all target hosts already accept the key, bypass prompts and remote install entirely
	if len(pendingHosts) == 0 {
		results := make([]starlark.Value, len(c.hosts))
		for i, host := range c.hosts {
			results[i] = newSSHResult(host, "copy_id", "[ALREADY INSTALLED] Public key already accepted by remote host", "", 0, true, false)
		}
		return starlark.NewList(results), nil
	}

	// 3. Handle interactive password prompt if necessary (only for pending hosts)
	origHosts := c.hosts
	c.hosts = pendingHosts
	defer func() { c.hosts = origHosts }()

	prompt := p.Prompt || c.prompt
	promptedPassword, err := promptPasswordIfNeeded(c, prompt)
	if err != nil {
		return nil, err
	}
	if promptedPassword != "" {
		c.password = promptedPassword
	}

	// 4. Generate idempotent remote shell command
	installCmd := buildCopyIdCommand(pubKey, asUser)
	if sudo {
		installCmd = "sudo " + installCmd
	}

	// 5. Dispatch across pending hosts using configured execution strategy
	var installResults starlark.Value
	if c.execPolicy == "concurrent" {
		installResults, err = c.execConcurrent(installCmd, c.execMaxWorkers)
	} else {
		installResults, err = c.execLinear(installCmd)
	}
	if err != nil {
		return nil, err
	}

	// 6. Merge results if some hosts were already accepted
	if len(alreadyAccepted) > 0 {
		installedList, ok := installResults.(*starlark.List)
		if ok {
			installedMap := make(map[string]starlark.Value)
			for i := 0; i < installedList.Len(); i++ {
				item := installedList.Index(i)
				if s, ok := item.(*starlarkstruct.Struct); ok {
					if hVal, err := s.Attr("host"); err == nil {
						if hStr, ok := starlark.AsString(hVal); ok {
							installedMap[hStr] = item
						}
					}
				}
			}

			merged := make([]starlark.Value, len(origHosts))
			for i, host := range origHosts {
				if alreadyAccepted[host] {
					merged[i] = newSSHResult(host, "copy_id", "[ALREADY INSTALLED] Public key already accepted by remote host", "", 0, true, false)
				} else if r, ok := installedMap[host]; ok {
					merged[i] = r
				}
			}
			return starlark.NewList(merged), nil
		}
	}

	return installResults, nil
}

// sshCopyId executes copy_id at the module level across a host list.
// Usage: ssh.copy_id(hosts=["192.168.1.10"], user="pi", password="...")
func (m *Module) sshCopyId(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawKey starlark.Value
	var rawHosts starlark.Value

	if len(args) > 0 {
		rawKey = args[0]
	}

	var p struct {
		Key          string `name:"key"`
		User         string `name:"user"`
		Password     string `name:"password"`
		Passphrase   string `name:"passphrase"`
		UseAgent     bool   `name:"use_agent"`
		Prompt       bool   `name:"prompt"`
		AsUser       string `name:"as_user"`
		Sudo         bool   `name:"sudo"`
		Port         int    `name:"port"`
		Timeout      string `name:"timeout"`
		DryRun       bool   `name:"dry_run"`
		HostKeyCheck bool   `name:"host_key_check"`
		KeyCheck     bool   `name:"key_check"`
	}
	p.HostKeyCheck = true

	var remainingKwargs []starlark.Tuple
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "key":
			rawKey = kv[1]
		case "hosts":
			rawHosts = kv[1]
		case "fleet":
			return nil, fmt.Errorf("ssh.copy_id: 'fleet' parameter is not supported in module-scope functions; use ssh.config(fleet=...) instead")
		case "jump", "jump_host", "jump_user", "jump_key", "jump_password", "jump_key_passphrase", "jump_port":
			return nil, fmt.Errorf("ssh.copy_id: bastion/jump host routing is not supported in module-scope functions; use ssh.config(jump={...}) instead")
		default:
			remainingKwargs = append(remainingKwargs, kv)
		}
	}

	if err := startype.Args(nil, remainingKwargs).Go(&p); err != nil {
		return nil, err
	}

	if rawHosts == nil || rawHosts == starlark.None {
		return nil, fmt.Errorf("ssh.copy_id: missing required parameter 'hosts'")
	}

	var hosts []string
	switch h := rawHosts.(type) {
	case *starlark.List:
		for i := 0; i < h.Len(); i++ {
			if s, ok := starlark.AsString(h.Index(i)); ok {
				hosts = append(hosts, s)
			}
		}
	case starlark.Tuple:
		for _, elem := range h {
			if s, ok := starlark.AsString(elem); ok {
				hosts = append(hosts, s)
			}
		}
	case starlark.String:
		hosts = append(hosts, string(h))
	default:
		return nil, fmt.Errorf("ssh.copy_id: 'hosts' must be a string or list of strings, got %s", rawHosts.Type())
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.copy_id: 'hosts' cannot be empty")
	}

	user := p.User
	if user == "" {
		user = defaultUser()
	}
	port := p.Port
	if port <= 0 {
		port = 22
	}
	timeout := 30 * time.Second
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh.copy_id: invalid timeout %q: %w", p.Timeout, err)
		}
		timeout = d
	}

	client := &SSHClient{
		thread:            thread,
		dryRun:            p.DryRun || (m.config != nil && m.config.DryRun),
		debug:             m.config != nil && m.config.Debug,
		hosts:             hosts,
		user:              user,
		password:          p.Password,
		keyPassphrase:     p.Passphrase,
		useAgent:          p.UseAgent,
		prompt:            p.Prompt,
		port:              port,
		timeout:           timeout,
		maxRetries:        3,
		execPolicy:        "concurrent",
		hostKeyCheck:      p.HostKeyCheck,
		keepAliveInterval: 30 * time.Second,
		keepAliveMax:      3,
		defaultSudo:       p.Sudo,
		defaultAsUser:     p.AsUser,
	}

	keyStr := p.Key
	if rawKey != nil {
		if s, ok := starlark.AsString(rawKey); ok {
			keyStr = s
		}
	}

	callKwargs := []starlark.Tuple{
		{starlark.String("key_check"), starlark.Bool(p.KeyCheck)},
	}
	if p.Prompt {
		callKwargs = append(callKwargs, starlark.Tuple{starlark.String("prompt"), starlark.Bool(true)})
	}

	return client.copyId(thread, fn, starlark.Tuple{starlark.String(keyStr)}, callKwargs)
}
