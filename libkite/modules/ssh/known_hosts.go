package ssh

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var knownHostsMu sync.Mutex

// KnownHostEntry represents an individual record within a known_hosts file.
type KnownHostEntry struct {
	Marker      string
	Host        string
	Type        string
	PublicKey   string
	Fingerprint string
	LineNumber  int
	Hashed      bool
	Comment     string
	RawLine     string
	KeyBlob     string
}

// PubKeyBase64 returns the base64-encoded raw key payload.
func (e *KnownHostEntry) PubKeyBase64() string {
	if e.KeyBlob != "" {
		return e.KeyBlob
	}
	parts := strings.Fields(e.PublicKey)
	if len(parts) >= 2 {
		return parts[1]
	}
	return e.PublicKey
}

func newSSHKnownHostEntry(e *KnownHostEntry) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHKnownHostEntry"), starlark.StringDict{
		"host":        starlark.String(e.Host),
		"type":        starlark.String(e.Type),
		"public_key":  starlark.String(e.PublicKey),
		"fingerprint": starlark.String(e.Fingerprint),
		"line_number": starlark.MakeInt(e.LineNumber),
		"hashed":      starlark.Bool(e.Hashed),
		"comment":     starlark.String(e.Comment),
		"line":        starlark.String(e.RawLine),
	})
}

// parseKnownHostsLine parses a single raw line into a KnownHostEntry.
// Returns nil, nil if the line is empty or a comment.
func parseKnownHostsLine(line string, lineNum int) (*KnownHostEntry, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil, nil
	}

	raw := trimmed
	var marker string
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil, nil
	}

	idx := 0
	if fields[0] == "@cert-authority" || fields[0] == "@revoked" {
		marker = fields[0]
		idx++
	}

	if len(fields) < idx+3 {
		return nil, fmt.Errorf("malformed known_hosts line %d: insufficient fields", lineNum)
	}

	hostPattern := fields[idx]
	_ = fields[idx+1] // keyType (e.g. "ssh-ed25519")
	keyBlob := fields[idx+2]
	comment := ""
	if len(fields) > idx+3 {
		comment = strings.Join(fields[idx+3:], " ")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(keyBlob)
	if err != nil {
		return nil, fmt.Errorf("malformed base64 key at line %d: %w", lineNum, err)
	}

	pubKey, err := gossh.ParsePublicKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("malformed public key at line %d: %w", lineNum, err)
	}

	isHashed := strings.HasPrefix(hostPattern, "|1|")

	return &KnownHostEntry{
		Marker:      marker,
		Host:        hostPattern,
		Type:        pubKey.Type(),
		PublicKey:   strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pubKey))),
		Fingerprint: gossh.FingerprintSHA256(pubKey),
		LineNumber:  lineNum,
		Hashed:      isHashed,
		Comment:     comment,
		RawLine:     raw,
		KeyBlob:     keyBlob,
	}, nil
}

// matchHashedHost verifies if target matches an OpenSSH hashed hostname pattern (|1|<salt>|<hash>).
func matchHashedHost(pattern, target string) bool {
	if !strings.HasPrefix(pattern, "|1|") {
		return false
	}
	parts := strings.Split(pattern, "|")
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expectedHash, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}

	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(target))
	actualHash := mac.Sum(nil)
	return hmac.Equal(actualHash, expectedHash)
}

// matchHostPattern returns true if the hostPattern matches target host/port.
func matchHostPattern(pattern, host string, port int) bool {
	targetAddr := host
	if port > 0 && port != 22 {
		targetAddr = fmt.Sprintf("[%s]:%d", host, port)
	}
	normTarget := knownhosts.Normalize(targetAddr)
	rawHost := host

	if strings.HasPrefix(pattern, "|1|") {
		return matchHashedHost(pattern, normTarget) || matchHashedHost(pattern, rawHost)
	}

	patterns := strings.SplitSeq(pattern, ",")
	for p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == host || p == normTarget || p == rawHost {
			return true
		}
		normP := knownhosts.Normalize(p)
		if normP == normTarget || normP == rawHost {
			return true
		}
		if strings.ContainsAny(p, "*?") {
			if ok, _ := filepath.Match(p, host); ok {
				return true
			}
			if ok, _ := filepath.Match(p, normTarget); ok {
				return true
			}
		}
	}
	return false
}

// acquireFileLock creates a file lock with timeout.
func acquireFileLock(lockPath string, timeout time.Duration) (*os.File, error) {
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to acquire known_hosts lock: %w", err)
		}
		// Stale lock check
		if fi, err := os.Stat(lockPath); err == nil {
			if time.Since(fi.ModTime()) > 30*time.Second {
				_ = os.Remove(lockPath)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out acquiring known_hosts lock %s", lockPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// releaseFileLock releases the file lock.
func releaseFileLock(f *os.File, lockPath string) {
	if f != nil {
		_ = f.Close()
	}
	_ = os.Remove(lockPath)
}

// findKnownHosts reads a known_hosts file and returns all entries matching the specified host and port.
func findKnownHosts(filePath, host string, port int) ([]*KnownHostEntry, error) {
	if host == "" {
		return nil, fmt.Errorf("ssh.find_known_hosts: 'host' is required")
	}
	if port <= 0 {
		port = 22
	}
	if filePath == "" {
		filePath = "~/.ssh/known_hosts"
	}
	expandedPath, err := expandPath(filePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(expandedPath)
	if os.IsNotExist(err) {
		return []*KnownHostEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read known_hosts %q: %w", expandedPath, err)
	}

	lines := strings.Split(string(content), "\n")
	var matches []*KnownHostEntry

	for i, l := range lines {
		entry, err := parseKnownHostsLine(l, i+1)
		if err != nil || entry == nil {
			continue
		}
		if matchHostPattern(entry.Host, host, port) {
			if entry.Hashed {
				entry.Host = host
			}
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

// removeKnownHost prunes all matching host entries from a known_hosts file.
func removeKnownHost(filePath, host string, port int, dryRun bool) (int, error) {
	if host == "" {
		return 0, fmt.Errorf("ssh.remove_known_host: 'host' is required")
	}
	if port <= 0 {
		port = 22
	}
	if filePath == "" {
		filePath = "~/.ssh/known_hosts"
	}
	expandedPath, err := expandPath(filePath)
	if err != nil {
		return 0, err
	}

	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	content, err := os.ReadFile(expandedPath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read known_hosts %q: %w", expandedPath, err)
	}

	targetAddr := host
	if port != 22 {
		targetAddr = fmt.Sprintf("[%s]:%d", host, port)
	}
	normTarget := knownhosts.Normalize(targetAddr)
	rawHost := host

	lines := strings.Split(string(content), "\n")
	var newLines []string
	removedCount := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if i < len(lines)-1 {
				newLines = append(newLines, line)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}

		entry, err := parseKnownHostsLine(trimmed, i+1)
		if err != nil || entry == nil {
			newLines = append(newLines, line)
			continue
		}

		if entry.Hashed {
			if matchHashedHost(entry.Host, normTarget) || matchHashedHost(entry.Host, rawHost) {
				removedCount++
				continue // Drop matched hashed line
			}
			newLines = append(newLines, line)
			continue
		}

		// Plaintext line
		patterns := strings.Split(entry.Host, ",")
		var remaining []string
		lineRemoved := 0

		for _, p := range patterns {
			pTrim := strings.TrimSpace(p)
			isMatch := (pTrim == host || pTrim == normTarget || pTrim == rawHost ||
				knownhosts.Normalize(pTrim) == normTarget || knownhosts.Normalize(pTrim) == rawHost)
			if !isMatch && strings.ContainsAny(pTrim, "*?") {
				if ok, _ := filepath.Match(pTrim, host); ok {
					isMatch = true
				}
				if ok, _ := filepath.Match(pTrim, normTarget); ok {
					isMatch = true
				}
			}
			if isMatch {
				lineRemoved++
			} else {
				remaining = append(remaining, pTrim)
			}
		}

		if lineRemoved > 0 {
			removedCount += lineRemoved
			if len(remaining) > 0 {
				reconstructed := strings.Join(remaining, ",")
				var prefix string
				if entry.Marker != "" {
					prefix = entry.Marker + " "
				}
				newLine := prefix + reconstructed + " " + entry.Type + " " + entry.PubKeyBase64()
				if entry.Comment != "" {
					newLine += " " + entry.Comment
				}
				newLines = append(newLines, newLine)
			}
			// When len(remaining) == 0, whole line is pruned
		} else {
			newLines = append(newLines, line)
		}
	}

	if removedCount > 0 && !dryRun {
		lockPath := expandedPath + ".lock"
		lockFile, err := acquireFileLock(lockPath, 5*time.Second)
		if err != nil {
			return 0, err
		}
		defer releaseFileLock(lockFile, lockPath)

		dir := filepath.Dir(expandedPath)
		tempFile, err := os.CreateTemp(dir, ".known_hosts.*.tmp")
		if err != nil {
			return 0, fmt.Errorf("failed to create temp file: %w", err)
		}
		tempPath := tempFile.Name()

		output := strings.Join(newLines, "\n")
		if len(output) > 0 && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}

		if _, err := tempFile.WriteString(output); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return 0, fmt.Errorf("failed to write updated known_hosts: %w", err)
		}
		if err := tempFile.Close(); err != nil {
			_ = os.Remove(tempPath)
			return 0, err
		}

		if fi, err := os.Stat(expandedPath); err == nil {
			_ = os.Chmod(tempPath, fi.Mode())
		} else {
			_ = os.Chmod(tempPath, 0644)
		}

		if err := os.Rename(tempPath, expandedPath); err != nil {
			_ = os.Remove(tempPath)
			return 0, fmt.Errorf("failed to atomically replace known_hosts: %w", err)
		}
	}

	return removedCount, nil
}

// addKnownHost appends a host key entry to known_hosts.
func addKnownHost(filePath, host, keyStr string, hash bool, port int, comment string, dryRun bool) (*KnownHostEntry, error) {
	if host == "" {
		return nil, fmt.Errorf("ssh.add_known_host: 'host' is required")
	}
	if keyStr == "" {
		return nil, fmt.Errorf("ssh.add_known_host: 'key' is required")
	}
	if port <= 0 {
		port = 22
	}
	if filePath == "" {
		filePath = "~/.ssh/known_hosts"
	}
	expandedPath, err := expandPath(filePath)
	if err != nil {
		return nil, err
	}

	var pubKey gossh.PublicKey
	var parsedComment string
	trimmedKey := strings.TrimSpace(keyStr)

	// Try parsing standard authorized_keys format
	k, c, _, _, err := gossh.ParseAuthorizedKey([]byte(trimmedKey))
	if err == nil {
		pubKey = k
		parsedComment = c
	} else {
		// Try parsing raw base64 key
		keyBytes, err2 := base64.StdEncoding.DecodeString(trimmedKey)
		if err2 == nil {
			k2, err3 := gossh.ParsePublicKey(keyBytes)
			if err3 == nil {
				pubKey = k2
			}
		}
	}

	if pubKey == nil {
		return nil, fmt.Errorf("ssh.add_known_host: failed to parse public key: %w", err)
	}

	if comment == "" && parsedComment != "" {
		comment = parsedComment
	}

	targetAddr := host
	if port != 22 {
		targetAddr = fmt.Sprintf("[%s]:%d", host, port)
	}
	normAddr := knownhosts.Normalize(targetAddr)

	var hostPattern string
	if hash {
		hostPattern = knownhosts.HashHostname(normAddr)
	} else {
		hostPattern = normAddr
	}

	line := knownhosts.Line([]string{hostPattern}, pubKey)
	if comment != "" {
		line = line + " " + comment
	}

	entry, err := parseKnownHostsLine(line, 1)
	if err != nil {
		return nil, err
	}
	if hash {
		entry.Host = host
	}

	if dryRun {
		return entry, nil
	}

	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(expandedPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory for known_hosts: %w", err)
	}

	lockPath := expandedPath + ".lock"
	lockFile, err := acquireFileLock(lockPath, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lockFile, lockPath)

	existing, _ := os.ReadFile(expandedPath)
	if len(existing) > 0 {
		if bytes.Contains(existing, []byte(line)) {
			return entry, nil
		}
	}

	f, err := os.OpenFile(expandedPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open known_hosts: %w", err)
	}
	defer f.Close()

	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + line + "\n"); err != nil {
		return nil, fmt.Errorf("failed to write to known_hosts: %w", err)
	}

	return entry, nil
}

// sshFindKnownHosts implements `ssh.find_known_hosts(host, path=None, port=22)`.
func (m *Module) sshFindKnownHosts(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host string `name:"host" position:"0"`
		Path string `name:"path"`
		Port int    `name:"port"`
	}
	p.Port = 22
	p.Path = "~/.ssh/known_hosts"

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "ssh", "connect", "find_known_hosts", p.Host); err != nil {
		return nil, err
	}

	entries, err := findKnownHosts(p.Path, p.Host, p.Port)
	if err != nil {
		return nil, err
	}

	elems := make([]starlark.Value, len(entries))
	for i, e := range entries {
		elems[i] = newSSHKnownHostEntry(e)
	}
	return starlark.NewList(elems), nil
}

// sshRemoveKnownHost implements `ssh.remove_known_host(host, path=None, port=22)`.
func (m *Module) sshRemoveKnownHost(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host string `name:"host" position:"0"`
		Path string `name:"path"`
		Port int    `name:"port"`
	}
	p.Port = 22
	p.Path = "~/.ssh/known_hosts"

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "ssh", "connect", "remove_known_host", p.Host); err != nil {
		return nil, err
	}

	dryRun := m.config != nil && m.config.DryRun
	count, err := removeKnownHost(p.Path, p.Host, p.Port, dryRun)
	if err != nil {
		return nil, err
	}

	return starlark.MakeInt(count), nil
}

// sshAddKnownHost implements `ssh.add_known_host(host, key, path=None, hash=True, port=22, comment="")`.
func (m *Module) sshAddKnownHost(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host    string `name:"host" position:"0"`
		Key     string `name:"key" position:"1"`
		Path    string `name:"path"`
		Hash    bool   `name:"hash"`
		Port    int    `name:"port"`
		Comment string `name:"comment"`
	}
	p.Port = 22
	p.Path = "~/.ssh/known_hosts"
	p.Hash = true

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "ssh", "connect", "add_known_host", p.Host); err != nil {
		return nil, err
	}

	dryRun := m.config != nil && m.config.DryRun
	entry, err := addKnownHost(p.Path, p.Host, p.Key, p.Hash, p.Port, p.Comment, dryRun)
	if err != nil {
		return nil, err
	}

	return newSSHKnownHostEntry(entry), nil
}

// findKnownHosts implements `client.find_known_hosts(host=None, path=None, port=None)`.
func (c *SSHClient) findKnownHosts(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host string `name:"host"`
		Path string `name:"path"`
		Port int    `name:"port"`
	}
	p.Port = c.port
	p.Path = c.knownHostsFile
	if p.Path == "" {
		p.Path = "~/.ssh/known_hosts"
	}

	if len(args) > 0 {
		if s, ok := starlark.AsString(args[0]); ok {
			p.Host = s
		}
		args = args[1:]
	}

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	var hosts []string
	if p.Host != "" {
		hosts = []string{p.Host}
	} else {
		hosts = c.hosts
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.client.find_known_hosts: no target hosts configured")
	}

	var allEntries []*KnownHostEntry
	for _, h := range hosts {
		entries, err := findKnownHosts(p.Path, h, p.Port)
		if err != nil {
			return nil, err
		}
		allEntries = append(allEntries, entries...)
	}

	elems := make([]starlark.Value, len(allEntries))
	for i, e := range allEntries {
		elems[i] = newSSHKnownHostEntry(e)
	}
	return starlark.NewList(elems), nil
}

// removeKnownHost implements `client.remove_known_host(host=None, path=None, port=None)`.
func (c *SSHClient) removeKnownHost(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host string `name:"host"`
		Path string `name:"path"`
		Port int    `name:"port"`
	}
	p.Port = c.port
	p.Path = c.knownHostsFile
	if p.Path == "" {
		p.Path = "~/.ssh/known_hosts"
	}

	if len(args) > 0 {
		if s, ok := starlark.AsString(args[0]); ok {
			p.Host = s
		}
		args = args[1:]
	}

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	var hosts []string
	if p.Host != "" {
		hosts = []string{p.Host}
	} else {
		hosts = c.hosts
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.client.remove_known_host: no target hosts configured")
	}

	total := 0
	for _, h := range hosts {
		count, err := removeKnownHost(p.Path, h, p.Port, c.dryRun)
		if err != nil {
			return nil, err
		}
		total += count
	}

	return starlark.MakeInt(total), nil
}

// addKnownHost implements `client.add_known_host(key, host=None, path=None, hash=True, port=None, comment="")`.
func (c *SSHClient) addKnownHost(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Key     string `name:"key"`
		Host    string `name:"host"`
		Path    string `name:"path"`
		Hash    bool   `name:"hash"`
		Port    int    `name:"port"`
		Comment string `name:"comment"`
	}
	p.Port = c.port
	p.Hash = true
	p.Path = c.knownHostsFile
	if p.Path == "" {
		p.Path = "~/.ssh/known_hosts"
	}

	if len(args) > 0 {
		if s, ok := starlark.AsString(args[0]); ok {
			p.Key = s
		}
		args = args[1:]
	}

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	targetHost := p.Host
	if targetHost == "" && len(c.hosts) > 0 {
		targetHost = c.hosts[0]
	}
	if targetHost == "" {
		return nil, fmt.Errorf("ssh.client.add_known_host: 'host' is required")
	}

	entry, err := addKnownHost(p.Path, targetHost, p.Key, p.Hash, p.Port, p.Comment, c.dryRun)
	if err != nil {
		return nil, err
	}

	return newSSHKnownHostEntry(entry), nil
}
