package manager

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

// receiptFile is the install receipt written for every installed module. It is
// a binary, manager-owned record of where and how the module was installed,
// kept separate from the author-authored mod.yaml. The binary format and the
// leading dot signal that it is a managed artifact, not a file to hand-edit.
const receiptFile = ".mod.receipt"

// receiptMagic prefixes the receipt so the file is unambiguously a starkite
// module receipt and a format version can be detected.
var receiptMagic = []byte("SKMR\x01")

// Provenance captures where and when a module was installed from.
type Provenance struct {
	Namespace string
	Name      string
	// Source is the resolved clone URL or local path.
	Source string
	// Version is the resolved commit or tag.
	Version string
	// InstalledFrom is the original source string the user supplied.
	InstalledFrom string
}

// WriteProvenance writes the install receipt into a module directory.
func WriteProvenance(modulePath string, p *Provenance) error {
	var buf bytes.Buffer
	buf.Write(receiptMagic)
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modulePath, receiptFile), buf.Bytes(), 0o644)
}

// ReadProvenance reads the install receipt from a module directory. Returns
// (nil, nil) when no receipt is present.
func ReadProvenance(modulePath string) (*Provenance, error) {
	data, err := os.ReadFile(filepath.Join(modulePath, receiptFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !bytes.HasPrefix(data, receiptMagic) {
		return nil, fmt.Errorf("invalid module receipt: bad magic")
	}
	var p Provenance
	if err := gob.NewDecoder(bytes.NewReader(data[len(receiptMagic):])).Decode(&p); err != nil {
		return nil, fmt.Errorf("invalid module receipt: %w", err)
	}
	return &p, nil
}
