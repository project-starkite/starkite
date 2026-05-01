// Package loader provides module registration for libkite.
// This package exists to avoid import cycles - the libkite package
// defines the interface, while this package imports all module implementations.
package loader

import (
	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/modules/base64"
	"github.com/project-starkite/starkite/libkite/modules/csv"
	fmtmod "github.com/project-starkite/starkite/libkite/modules/fmt"
	"github.com/project-starkite/starkite/libkite/modules/fs"
	"github.com/project-starkite/starkite/libkite/modules/gzip"
	"github.com/project-starkite/starkite/libkite/modules/hash"
	"github.com/project-starkite/starkite/libkite/modules/http"
	"github.com/project-starkite/starkite/libkite/modules/inventory"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
	"github.com/project-starkite/starkite/libkite/modules/json"
	"github.com/project-starkite/starkite/libkite/modules/log"
	osmod "github.com/project-starkite/starkite/libkite/modules/os"
	"github.com/project-starkite/starkite/libkite/modules/concur"
	"github.com/project-starkite/starkite/libkite/modules/regexp"
	"github.com/project-starkite/starkite/libkite/modules/retry"
	"github.com/project-starkite/starkite/libkite/modules/runtime"
	"github.com/project-starkite/starkite/libkite/modules/ssh"
	"github.com/project-starkite/starkite/libkite/modules/strings"
	"github.com/project-starkite/starkite/libkite/modules/table"
	"github.com/project-starkite/starkite/libkite/modules/template"
	"github.com/project-starkite/starkite/libkite/modules/test"
	"github.com/project-starkite/starkite/libkite/modules/time"
	"github.com/project-starkite/starkite/libkite/modules/uuid"
	"github.com/project-starkite/starkite/libkite/modules/vars"
	"github.com/project-starkite/starkite/libkite/modules/yaml"
	"github.com/project-starkite/starkite/libkite/modules/zip"
)

// RegisterAll registers all built-in modules with the given registry.
func RegisterAll(r *libkite.Registry) {
	// Core modules with global aliases
	r.Register(osmod.New())    // os.* + global aliases (env, exec, etc.)
	r.Register(fs.New())       // fs.* + global aliases (read_file, exists, etc.)
	r.Register(fmtmod.New())   // fmt.* + global aliases (printf, sprintf, errorf)
	r.Register(runtime.New())  // runtime.* (platform, arch, cpu_count, uname)
	r.Register(iomod.New())    // io.* (confirm, prompt)
	r.Register(test.New())     // test.* + global aliases (skip, fail)
	r.Register(vars.New())     // vars.* + global alias (var)

	// Stateless utility modules
	r.Register(strings.New())
	r.Register(regexp.New())
	r.Register(json.New())
	r.Register(yaml.New())
	r.Register(csv.New())
	r.Register(time.New())
	r.Register(base64.New())
	r.Register(hash.New())
	r.Register(uuid.New())
	r.Register(template.New())
	r.Register(log.New())
	r.Register(concur.New())
	r.Register(retry.New())
	r.Register(table.New())
	r.Register(gzip.New())
	r.Register(zip.New())

	// Provider modules (stateful)
	r.Register(ssh.New())
	r.Register(http.New())
	r.Register(inventory.New())

	// Note: WASM plugins are registered separately by core/cloud editions
	// via the wasm package (github.com/project-starkite/starkite/wasm)
}

// NewDefaultRegistry creates a new registry with all built-in modules registered.
func NewDefaultRegistry(config *libkite.ModuleConfig) *libkite.Registry {
	r := libkite.NewRegistry(config)
	RegisterAll(r)
	return r
}
