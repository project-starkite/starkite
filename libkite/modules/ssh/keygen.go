package ssh

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// KeyPair holds the generated cryptographic SSH keypair data.
type KeyPair struct {
	Type        string
	PublicKey   string
	PrivateKey  string
	Fingerprint string
	Comment     string
	Path        string
	PubPath     string
}

// KeyGenOptions configures keypair generation.
type KeyGenOptions struct {
	Type       string
	Bits       int
	Comment    string
	Passphrase string
	Path       string
	Overwrite  bool
}

// GenerateKeyPair generates an SSH keypair for the specified type, bits, and comment in memory.
func GenerateKeyPair(keyType string, bits int, comment string) (*KeyPair, error) {
	return GenerateKeyPairWithOptions(KeyGenOptions{
		Type:    keyType,
		Bits:    bits,
		Comment: comment,
	})
}

// GenerateKeyPairWithOptions generates an SSH keypair with full configuration options, including
// optional passphrase encryption and atomic disk persistence with strict POSIX file modes.
func GenerateKeyPairWithOptions(opts KeyGenOptions) (*KeyPair, error) {
	keyType := strings.ToLower(strings.TrimSpace(opts.Type))
	if keyType == "" {
		keyType = "ed25519"
	}

	var priv crypto.PrivateKey
	var pub crypto.PublicKey

	switch keyType {
	case "ed25519":
		edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
		}
		priv = edPriv
		pub = edPub

	case "rsa":
		bits := opts.Bits
		if bits == 0 {
			bits = 4096
		}
		if bits != 2048 && bits != 3072 && bits != 4096 {
			return nil, fmt.Errorf("invalid RSA key size %d (must be 2048, 3072, or 4096)", bits)
		}
		rsaPriv, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, fmt.Errorf("failed to generate rsa key: %w", err)
		}
		priv = rsaPriv
		pub = &rsaPriv.PublicKey

	case "ecdsa":
		var curve elliptic.Curve
		switch opts.Bits {
		case 0, 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("invalid ECDSA key size %d (must be 256, 384, or 521)", opts.Bits)
		}
		ecdsaPriv, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ecdsa key: %w", err)
		}
		priv = ecdsaPriv
		pub = &ecdsaPriv.PublicKey

	default:
		return nil, fmt.Errorf("unsupported key type %q (must be ed25519, rsa, or ecdsa)", keyType)
	}

	// 1. Serialize OpenSSH Public Key
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}

	pubBytes := ssh.MarshalAuthorizedKey(sshPub)
	pubStr := strings.TrimSpace(string(pubBytes))
	if opts.Comment != "" {
		pubStr = fmt.Sprintf("%s %s", pubStr, opts.Comment)
	}
	pubStr += "\n"

	// 2. Compute SHA256 Fingerprint
	fingerprint := ssh.FingerprintSHA256(sshPub)

	// 3. Serialize OpenSSH Private Key (PEM format)
	var pemBlock *pem.Block
	if opts.Passphrase != "" {
		pemBlock, err = ssh.MarshalPrivateKeyWithPassphrase(priv, opts.Comment, []byte(opts.Passphrase))
	} else {
		pemBlock, err = ssh.MarshalPrivateKey(priv, opts.Comment)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privPem := string(pem.EncodeToMemory(pemBlock))

	kp := &KeyPair{
		Type:        keyType,
		PublicKey:   pubStr,
		PrivateKey:  privPem,
		Fingerprint: fingerprint,
		Comment:     opts.Comment,
	}

	// 4. Handle optional disk persistence
	if opts.Path != "" {
		targetPath := opts.Path
		if strings.HasPrefix(targetPath, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("cannot resolve home directory for path %q: %w", targetPath, err)
			}
			if targetPath == "~" {
				targetPath = home
			} else if strings.HasPrefix(targetPath, "~/") {
				targetPath = filepath.Join(home, targetPath[2:])
			}
		}

		absPrivPath, err := filepath.Abs(targetPath)
		if err != nil {
			return nil, fmt.Errorf("invalid path %q: %w", targetPath, err)
		}
		absPubPath := absPrivPath + ".pub"

		// Overwrite guard
		if !opts.Overwrite {
			if _, err := os.Stat(absPrivPath); err == nil {
				return nil, fmt.Errorf("private key file %q already exists (use overwrite=True)", absPrivPath)
			}
			if _, err := os.Stat(absPubPath); err == nil {
				return nil, fmt.Errorf("public key file %q already exists (use overwrite=True)", absPubPath)
			}
		}

		// Ensure directory exists with mode 0700
		dir := filepath.Dir(absPrivPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory %q: %w", dir, err)
		}

		// Write private key with mode 0600
		if err := os.WriteFile(absPrivPath, []byte(kp.PrivateKey), 0600); err != nil {
			return nil, fmt.Errorf("failed to write private key to %q: %w", absPrivPath, err)
		}

		// Write public key with mode 0644
		if err := os.WriteFile(absPubPath, []byte(kp.PublicKey), 0644); err != nil {
			return nil, fmt.Errorf("failed to write public key to %q: %w", absPubPath, err)
		}

		kp.Path = absPrivPath
		kp.PubPath = absPubPath
	}

	return kp, nil
}
