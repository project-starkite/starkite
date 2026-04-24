// Package loader provides module registration for starbase.
// This package exists to avoid import cycles - the starbase package
// defines the interface, while this package imports all module implementations.
package loader

import (
	"github.com/project-starkite/starkite/starbase"
	"github.com/project-starkite/starkite/starbase/modules/base64"
	"github.com/project-starkite/starkite/starbase/modules/csv"
	fmtmod "github.com/project-starkite/starkite/starbase/modules/fmt"
	"github.com/project-starkite/starkite/starbase/modules/fs"
	"github.com/project-starkite/starkite/starbase/modules/gzip"
	"github.com/project-starkite/starkite/starbase/modules/hash"
	"github.com/project-starkite/starkite/starbase/modules/http"
	"github.com/project-starkite/starkite/starbase/modules/inventory"
	iomod "github.com/project-starkite/starkite/starbase/modules/io"
	"github.com/project-starkite/starkite/starbase/modules/json"
	"github.com/project-starkite/starkite/starbase/modules/log"
	osmod "github.com/project-starkite/starkite/starbase/modules/os"
	"github.com/project-starkite/starkite/starbase/modules/concur"
	"github.com/project-starkite/starkite/starbase/modules/regexp"
	"github.com/project-starkite/starkite/starbase/modules/retry"
	"github.com/project-starkite/starkite/starbase/modules/runtime"
	"github.com/project-starkite/starkite/starbase/modules/ssh"
	"github.com/project-starkite/starkite/starbase/modules/strings"
	"github.com/project-starkite/starkite/starbase/modules/table"
	"github.com/project-starkite/starkite/starbase/modules/template"
	"github.com/project-starkite/starkite/starbase/modules/test"
	"github.com/project-starkite/starkite/starbase/modules/time"
	"github.com/project-starkite/starkite/starbase/modules/uuid"
	"github.com/project-starkite/starkite/starbase/modules/vars"
	"github.com/project-starkite/starkite/starbase/modules/yaml"
	"github.com/project-starkite/starkite/starbase/modules/zip"
)

// RegisterAll registers all built-in modules with the given registry.
func RegisterAll(r *starbase.Registry) {
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
func NewDefaultRegistry(config *starbase.ModuleConfig) *starbase.Registry {
	r := starbase.NewRegistry(config)
	RegisterAll(r)
	return r
}
