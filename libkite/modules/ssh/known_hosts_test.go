package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
	gossh "golang.org/x/crypto/ssh"
)

// generateTestHostKey generates a dummy RSA public key for known_hosts testing.
func generateTestHostKey(t *testing.T) (gossh.PublicKey, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey failed: %v", err)
	}
	pub, err := gossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("gossh.NewPublicKey failed: %v", err)
	}
	pubStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub)))
	return pub, pubStr
}

func TestKnownHosts_AddFindRemove(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	pubKey1, pubKeyStr1 := generateTestHostKey(t)
	pubKey2, pubKeyStr2 := generateTestHostKey(t)

	// 1. Add known host (plaintext)
	entry1, err := addKnownHost(knownHostsPath, "server1.example.com", pubKeyStr1, false, 22, "comment-server1", false)
	if err != nil {
		t.Fatalf("addKnownHost server1 failed: %v", err)
	}
	if entry1.Host != "server1.example.com" {
		t.Errorf("entry1.Host = %q, want server1.example.com", entry1.Host)
	}
	if entry1.Comment != "comment-server1" {
		t.Errorf("entry1.Comment = %q, want comment-server1", entry1.Comment)
	}

	// 2. Add known host (hashed)
	entry2, err := addKnownHost(knownHostsPath, "192.168.1.50", pubKeyStr2, true, 22, "", false)
	if err != nil {
		t.Fatalf("addKnownHost server2 (hashed) failed: %v", err)
	}
	if entry2.Host != "192.168.1.50" {
		t.Errorf("entry2.Host = %q, want 192.168.1.50", entry2.Host)
	}
	if !entry2.Hashed {
		t.Errorf("entry2.Hashed = false, want true")
	}

	// 3. Add known host on custom port (hashed)
	_, pubKeyStr3 := generateTestHostKey(t)
	_, err = addKnownHost(knownHostsPath, "edge-node", pubKeyStr3, true, 2222, "edge-node-port-2222", false)
	if err != nil {
		t.Fatalf("addKnownHost edge-node failed: %v", err)
	}

	// 4. Test findKnownHosts (plaintext)
	matches, err := findKnownHosts(knownHostsPath, "server1.example.com", 22)
	if err != nil {
		t.Fatalf("findKnownHosts server1 failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("findKnownHosts server1 count = %d, want 1", len(matches))
	}
	if matches[0].Fingerprint != gossh.FingerprintSHA256(pubKey1) {
		t.Errorf("fingerprint = %s, want %s", matches[0].Fingerprint, gossh.FingerprintSHA256(pubKey1))
	}

	// 5. Test findKnownHosts (hashed)
	matchesHashed, err := findKnownHosts(knownHostsPath, "192.168.1.50", 22)
	if err != nil {
		t.Fatalf("findKnownHosts 192.168.1.50 failed: %v", err)
	}
	if len(matchesHashed) != 1 {
		t.Fatalf("findKnownHosts 192.168.1.50 count = %d, want 1", len(matchesHashed))
	}
	if matchesHashed[0].Fingerprint != gossh.FingerprintSHA256(pubKey2) {
		t.Errorf("fingerprint = %s, want %s", matchesHashed[0].Fingerprint, gossh.FingerprintSHA256(pubKey2))
	}
	if !matchesHashed[0].Hashed {
		t.Errorf("matchesHashed[0].Hashed = false, want true")
	}

	// 6. Test findKnownHosts (custom port)
	matchesPort, err := findKnownHosts(knownHostsPath, "edge-node", 2222)
	if err != nil {
		t.Fatalf("findKnownHosts edge-node:2222 failed: %v", err)
	}
	if len(matchesPort) != 1 {
		t.Fatalf("findKnownHosts edge-node count = %d, want 1", len(matchesPort))
	}

	// 7. Test findKnownHosts (non-matching)
	matchesNone, err := findKnownHosts(knownHostsPath, "nonexistent.host", 22)
	if err != nil {
		t.Fatalf("findKnownHosts nonexistent failed: %v", err)
	}
	if len(matchesNone) != 0 {
		t.Errorf("findKnownHosts nonexistent count = %d, want 0", len(matchesNone))
	}

	// 8. Test removeKnownHost on hashed host
	removed, err := removeKnownHost(knownHostsPath, "192.168.1.50", 22, false)
	if err != nil {
		t.Fatalf("removeKnownHost failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("removeKnownHost 192.168.1.50 removed = %d, want 1", removed)
	}

	// Verify it was removed
	afterRemove, err := findKnownHosts(knownHostsPath, "192.168.1.50", 22)
	if err != nil {
		t.Fatalf("findKnownHosts after remove failed: %v", err)
	}
	if len(afterRemove) != 0 {
		t.Errorf("expected 0 entries after remove, got %d", len(afterRemove))
	}

	// 9. Test removeKnownHost on plaintext host
	removedServer1, err := removeKnownHost(knownHostsPath, "server1.example.com", 22, false)
	if err != nil {
		t.Fatalf("removeKnownHost server1 failed: %v", err)
	}
	if removedServer1 != 1 {
		t.Errorf("removeKnownHost server1 removed = %d, want 1", removedServer1)
	}

	afterRemoveServer1, err := findKnownHosts(knownHostsPath, "server1.example.com", 22)
	if err != nil {
		t.Fatalf("findKnownHosts after remove failed: %v", err)
	}
	if len(afterRemoveServer1) != 0 {
		t.Errorf("expected 0 entries for server1 after remove, got %d", len(afterRemoveServer1))
	}
}

func TestKnownHosts_CommaSeparatedHostsPruning(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	_, pubKeyStr := generateTestHostKey(t)
	// Write a line with multiple comma-separated hosts
	line := fmt.Sprintf("node-a,node-b,10.0.0.5 %s comment\n", pubKeyStr)
	if err := os.WriteFile(knownHostsPath, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	// Query node-b
	entries, err := findKnownHosts(knownHostsPath, "node-b", 22)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for node-b, got %d", len(entries))
	}

	// Remove node-b only
	removed, err := removeKnownHost(knownHostsPath, "node-b", 22, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	// Verify node-b is gone but node-a and 10.0.0.5 remain
	entriesB, _ := findKnownHosts(knownHostsPath, "node-b", 22)
	if len(entriesB) != 0 {
		t.Errorf("expected node-b to be removed, got %d entries", len(entriesB))
	}
	entriesA, _ := findKnownHosts(knownHostsPath, "node-a", 22)
	if len(entriesA) != 1 {
		t.Errorf("expected node-a to remain, got %d entries", len(entriesA))
	}
	entriesIP, _ := findKnownHosts(knownHostsPath, "10.0.0.5", 22)
	if len(entriesIP) != 1 {
		t.Errorf("expected 10.0.0.5 to remain, got %d entries", len(entriesIP))
	}

	// Now remove node-a and 10.0.0.5
	removeKnownHost(knownHostsPath, "node-a", 22, false)
	removeKnownHost(knownHostsPath, "10.0.0.5", 22, false)

	// File should now have 0 entries
	entriesFinal, _ := findKnownHosts(knownHostsPath, "node-a", 22)
	if len(entriesFinal) != 0 {
		t.Errorf("expected 0 entries remaining, got %d", len(entriesFinal))
	}
}

func TestKnownHosts_StarlarkExecution(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	_, pubKeyStr := generateTestHostKey(t)

	rt, err := libkite.NewTrusted(nil)
	if err != nil {
		t.Fatal(err)
	}

	sshMod := New()
	dict, err := sshMod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"ssh": dict["ssh"],
	}
	thread := rt.NewThread("test-known-hosts")

	script := fmt.Sprintf(`
def test_workflow():
    # 1. Add known host
    entry = ssh.add_known_host("my-vm.local", %q, path=%q, hash=True)
    if not entry.hashed:
        return "expected hashed entry"
    if entry.host != "my-vm.local":
        return "expected host my-vm.local"

    # 2. Find known host
    found = ssh.find_known_hosts("my-vm.local", path=%q)
    if len(found) != 1:
        return "expected 1 found entry"
    if not found[0].fingerprint.startswith("SHA256:"):
        return "expected valid fingerprint"

    # 3. Safe try_ variant
    try_res = ssh.try_find_known_hosts("my-vm.local", path=%q)
    if not try_res.ok:
        return "expected try_res.ok to be True"

    # 4. Remove known host
    removed = ssh.remove_known_host("my-vm.local", path=%q)
    if removed != 1:
        return "expected removed count 1"

    # 5. Verify gone
    after = ssh.find_known_hosts("my-vm.local", path=%q)
    if len(after) != 0:
        return "expected 0 entries after removal"

    return "ok"
`, pubKeyStr, knownHostsPath, knownHostsPath, knownHostsPath, knownHostsPath, knownHostsPath)

	globals, err := starlark.ExecFile(thread, "test_kh.star", script, predeclared)
	if err != nil {
		t.Fatalf("ExecFile failed: %v", err)
	}

	res, err := starlark.Call(thread, globals["test_workflow"], nil, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	strVal, ok := starlark.AsString(res)
	if !ok || strVal != "ok" {
		t.Errorf("test_workflow result = %v, want 'ok'", res)
	}
}

func TestKnownHosts_ClientMethods(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	_, pubKeyStr := generateTestHostKey(t)

	rt, err := libkite.NewTrusted(nil)
	if err != nil {
		t.Fatal(err)
	}

	sshMod := New()
	dict, err := sshMod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"ssh": dict["ssh"],
	}
	thread := rt.NewThread("test-client-kh")

	script := fmt.Sprintf(`
def test_client():
    client = ssh.config(
        hosts = ["cluster-node-1"],
        known_hosts_file = %q,
    )

    # 1. Add host key via client
    entry = client.add_known_host(%q, hash=True)
    if not entry.hashed:
        return "expected hashed entry"

    # 2. Find host keys via client
    found = client.find_known_hosts()
    if len(found) != 1:
        return "expected 1 found entry"

    # 3. Remove host key via client
    removed = client.remove_known_host()
    if removed != 1:
        return "expected removed 1"

    # 4. Verify gone
    after = client.find_known_hosts()
    if len(after) != 0:
        return "expected 0 after remove"

    return "ok"
`, knownHostsPath, pubKeyStr)

	globals, err := starlark.ExecFile(thread, "test_client_kh.star", script, predeclared)
	if err != nil {
		t.Fatalf("ExecFile failed: %v", err)
	}

	res, err := starlark.Call(thread, globals["test_client"], nil, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	strVal, ok := starlark.AsString(res)
	if !ok || strVal != "ok" {
		t.Errorf("test_client result = %v, want 'ok'", res)
	}
}
