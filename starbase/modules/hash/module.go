// Package hash provides cryptographic hash functions for starkite.
package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "hash"

// Module implements cryptographic hash functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "hash provides cryptographic hash functions: file, text, bytes"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"file":  starlark.NewBuiltin("hash.file", m.fileFactory),
			"text":  starlark.NewBuiltin("hash.text", m.textFactory),
			"bytes": starlark.NewBuiltin("hash.bytes", m.bytesFactory),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// fileFactory creates a HashFile anchored at a path.
// Usage: hash.file(path)
func (m *Module) fileFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &HashFile{path: p.Path, thread: thread, config: m.config}, nil
}

// textFactory creates a Source from a string.
// Usage: hash.text(s)
func (m *Module) textFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &Source{data: []byte(p.S), thread: thread, config: m.config}, nil
}

// bytesFactory creates a Source from bytes or string.
// Usage: hash.bytes(data)
func (m *Module) bytesFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Data starlark.Value `name:"data" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	var data []byte
	switch v := p.Data.(type) {
	case starlark.Bytes:
		data = []byte(string(v))
	case starlark.String:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("hash.bytes: argument must be bytes or string, got %s", p.Data.Type())
	}
	return &Source{data: data, thread: thread, config: m.config}, nil
}

// computeHash computes the hex-encoded hash of data using the named algorithm.
func computeHash(algo string, data []byte) (string, error) {
	switch algo {
	case "md5":
		h := md5.Sum(data)
		return hex.EncodeToString(h[:]), nil
	case "sha1":
		h := sha1.Sum(data)
		return hex.EncodeToString(h[:]), nil
	case "sha256":
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:]), nil
	case "sha512":
		h := sha512.Sum512(data)
		return hex.EncodeToString(h[:]), nil
	default:
		return "", fmt.Errorf("unknown hash algorithm: %s", algo)
	}
}

// hashHexLen returns the hex string length for a given algorithm.
func hashHexLen(algo string) int {
	switch algo {
	case "md5":
		return 32
	case "sha1":
		return 40
	case "sha256":
		return 64
	case "sha512":
		return 128
	default:
		return 0
	}
}
