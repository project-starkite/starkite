package ssh

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func verifyKeyPair(t *testing.T, kp *KeyPair, expectedPrefix string) {
	t.Helper()

	if kp == nil {
		t.Fatal("expected non-nil KeyPair")
	}

	// 1. Verify public key format
	if !strings.HasPrefix(kp.PublicKey, expectedPrefix) {
		t.Errorf("public key = %q, want prefix %q", kp.PublicKey, expectedPrefix)
	}
	if kp.Comment != "" && !strings.Contains(kp.PublicKey, kp.Comment) {
		t.Errorf("public key should contain comment %q: %q", kp.Comment, kp.PublicKey)
	}

	// 2. Parse public key
	pubKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(kp.PublicKey))
	if err != nil {
		t.Fatalf("failed to parse authorized key: %v", err)
	}
	if kp.Comment != "" && comment != kp.Comment {
		t.Errorf("parsed comment = %q, want %q", comment, kp.Comment)
	}

	// 3. Verify fingerprint
	expectedFingerprint := ssh.FingerprintSHA256(pubKey)
	if kp.Fingerprint != expectedFingerprint {
		t.Errorf("fingerprint = %q, want %q", kp.Fingerprint, expectedFingerprint)
	}
	if !strings.HasPrefix(kp.Fingerprint, "SHA256:") {
		t.Errorf("fingerprint %q should start with SHA256:", kp.Fingerprint)
	}

	// 4. Verify private key is valid OpenSSH PEM format
	if !strings.Contains(kp.PrivateKey, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Errorf("private key missing OpenSSH header: %q", kp.PrivateKey)
	}

	// 5. Parse private key and match public key bytes
	signer, err := ssh.ParsePrivateKey([]byte(kp.PrivateKey))
	if err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}
	if !bytes.Equal(signer.PublicKey().Marshal(), pubKey.Marshal()) {
		t.Error("private key signer public key does not match generated public key")
	}
}

func TestGenerateKeyPairEd25519(t *testing.T) {
	tests := []struct {
		name    string
		comment string
	}{
		{"default without comment", ""},
		{"with comment", "user@hostname-test"},
		{"with spaces in comment", "cluster admin key 2026"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kp, err := GenerateKeyPair("ed25519", 0, tc.comment)
			if err != nil {
				t.Fatalf("GenerateKeyPair failed: %v", err)
			}
			if kp.Type != "ed25519" {
				t.Errorf("Type = %q, want ed25519", kp.Type)
			}
			verifyKeyPair(t, kp, "ssh-ed25519 ")
		})
	}
}

func TestGenerateKeyPairRSA(t *testing.T) {
	tests := []struct {
		name string
		bits int
	}{
		{"rsa default (4096)", 0},
		{"rsa 2048", 2048},
		{"rsa 3072", 3072},
		{"rsa 4096", 4096},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kp, err := GenerateKeyPair("rsa", tc.bits, "rsa-test")
			if err != nil {
				t.Fatalf("GenerateKeyPair(rsa, %d) failed: %v", tc.bits, err)
			}
			if kp.Type != "rsa" {
				t.Errorf("Type = %q, want rsa", kp.Type)
			}
			verifyKeyPair(t, kp, "ssh-rsa ")
		})
	}
}

func TestGenerateKeyPairECDSA(t *testing.T) {
	tests := []struct {
		name           string
		bits           int
		expectedPrefix string
	}{
		{"ecdsa default (P-256)", 0, "ecdsa-sha2-nistp256 "},
		{"ecdsa 256", 256, "ecdsa-sha2-nistp256 "},
		{"ecdsa 384", 384, "ecdsa-sha2-nistp384 "},
		{"ecdsa 521", 521, "ecdsa-sha2-nistp521 "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kp, err := GenerateKeyPair("ecdsa", tc.bits, "ecdsa-test")
			if err != nil {
				t.Fatalf("GenerateKeyPair(ecdsa, %d) failed: %v", tc.bits, err)
			}
			if kp.Type != "ecdsa" {
				t.Errorf("Type = %q, want ecdsa", kp.Type)
			}
			verifyKeyPair(t, kp, tc.expectedPrefix)
		})
	}
}

func TestGenerateKeyPairDefaults(t *testing.T) {
	// Empty type should default to ed25519
	kp, err := GenerateKeyPair("", 0, "")
	if err != nil {
		t.Fatalf("GenerateKeyPair with empty type failed: %v", err)
	}
	if kp.Type != "ed25519" {
		t.Errorf("default type = %q, want ed25519", kp.Type)
	}
	verifyKeyPair(t, kp, "ssh-ed25519 ")

	// Mixed case and spaces
	kp2, err := GenerateKeyPair("  Ed25519  ", 0, "")
	if err != nil {
		t.Fatalf("GenerateKeyPair with trimmed mixed case failed: %v", err)
	}
	if kp2.Type != "ed25519" {
		t.Errorf("trimmed type = %q, want ed25519", kp2.Type)
	}
}

func TestGenerateKeyPairErrors(t *testing.T) {
	// 1. Unsupported type
	_, err := GenerateKeyPair("dsa", 0, "")
	if err == nil {
		t.Error("expected error for unsupported key type 'dsa'")
	}

	// 2. Invalid RSA bits
	_, err = GenerateKeyPair("rsa", 1024, "")
	if err == nil {
		t.Error("expected error for insecure RSA 1024 bits")
	}
	_, err = GenerateKeyPair("rsa", 5000, "")
	if err == nil {
		t.Error("expected error for invalid RSA 5000 bits")
	}

	// 3. Invalid ECDSA bits
	_, err = GenerateKeyPair("ecdsa", 512, "")
	if err == nil {
		t.Error("expected error for invalid ECDSA 512 bits")
	}
}
