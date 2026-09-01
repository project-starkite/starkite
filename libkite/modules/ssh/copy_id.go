package ssh

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
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
func promptPasswordIfNeeded(c *SSHClient, askPassword bool) (string, error) {
	if c.password != "" && !askPassword {
		return c.password, nil
	}

	if (c.keyFile != "" || (c.useAgent && os.Getenv("SSH_AUTH_SOCK") != "")) && !askPassword {
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

	if askPassword {
		return "", fmt.Errorf("ssh.copy_id: ask_password=True specified but standard input is not a terminal")
	}

	if c.password == "" && c.keyFile == "" && !c.useAgent {
		return "", fmt.Errorf("ssh.copy_id: no password or key configured, and standard input is not a terminal")
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
		Key         string `name:"key" position:"0"`
		AsUser      string `name:"as_user"`
		Sudo        bool   `name:"sudo"`
		AskPassword bool   `name:"ask_password"`
		UseAgent    bool   `name:"use_agent"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	useAgent := p.UseAgent || c.useAgent

	// 1. Resolve Public Key
	pubKey, err := resolvePublicKey(p.Key, useAgent)
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

	// 2. Handle interactive password prompt if necessary
	promptedPassword, err := promptPasswordIfNeeded(c, p.AskPassword)
	if err != nil {
		return nil, err
	}
	if promptedPassword != "" {
		c.password = promptedPassword
	}

	asUser := p.AsUser
	if asUser == "" {
		asUser = c.defaultAsUser
	}
	sudo := p.Sudo || c.defaultSudo

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	// 3. Generate idempotent remote shell command
	installCmd := buildCopyIdCommand(pubKey, asUser)
	if sudo {
		installCmd = "sudo " + installCmd
	}

	// 4. Dispatch across fleet using configured execution strategy
	if c.execPolicy == "concurrent" {
		return c.execConcurrent(installCmd, c.execMaxWorkers)
	}
	return c.execLinear(installCmd)
}

// sshCopyId executes copy_id at the module level across a fleet or hosts list.
func (m *Module) sshCopyId(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var copyIdArgs starlark.Tuple
	var copyIdKwargs []starlark.Tuple
	var configKwargs []starlark.Tuple

	if len(args) > 0 {
		copyIdArgs = args
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "key":
			if len(copyIdArgs) == 0 {
				copyIdArgs = starlark.Tuple{kv[1]}
			} else {
				copyIdKwargs = append(copyIdKwargs, kv)
			}
		case "as_user", "sudo", "ask_password", "use_agent":
			copyIdKwargs = append(copyIdKwargs, kv)
		default:
			configKwargs = append(configKwargs, kv)
		}
	}

	// Create ephemeral SSHClient
	clientVal, err := m.sshConfig(thread, fn, nil, configKwargs)
	if err != nil {
		return nil, err
	}
	client := clientVal.(*SSHClient)

	return client.copyId(thread, fn, copyIdArgs, copyIdKwargs)
}
