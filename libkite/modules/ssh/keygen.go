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
}

// GenerateKeyPair generates an SSH keypair for the specified type and options.
func GenerateKeyPair(keyType string, bits int, comment string) (*KeyPair, error) {
	keyType = strings.ToLower(strings.TrimSpace(keyType))
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
		switch bits {
		case 0, 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("invalid ECDSA key size %d (must be 256, 384, or 521)", bits)
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
	if comment != "" {
		pubStr = fmt.Sprintf("%s %s", pubStr, comment)
	}
	pubStr += "\n"

	// 2. Compute SHA256 Fingerprint
	fingerprint := ssh.FingerprintSHA256(sshPub)

	// 3. Serialize OpenSSH Private Key (PEM format)
	pemBlock, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privPem := string(pem.EncodeToMemory(pemBlock))

	return &KeyPair{
		Type:        keyType,
		PublicKey:   pubStr,
		PrivateKey:  privPem,
		Fingerprint: fingerprint,
		Comment:     comment,
	}, nil
}
