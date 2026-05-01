// Package libkite provides an embeddable Starlark runtime with built-in modules,
// a permission system for sandboxing untrusted scripts, and a complete execution engine.
//
// # Quick Start
//
// For CLI tools that need full access:
//
//	rt, err := libkite.NewTrusted(nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer rt.Close()
//	if err := rt.Execute(code); err != nil {
//		log.Fatal(err)
//	}
//
// For sandboxed execution with limited permissions:
//
//	rt, err := libkite.NewSandboxed(nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer rt.Close()
//	if err := rt.Execute(code); err != nil {
//		log.Fatal(err)
//	}
//
// # Configuration
//
// Use Config for full control:
//
//	config := &libkite.Config{
//		ScriptPath:  "script.star",
//		Timeout:     30 * time.Second,
//		Debug:       true,
//		Permissions: libkite.TrustedPermissions(),
//	}
//	rt, err := libkite.New(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer rt.Close()
//
// # Permissions
//
// The permission system controls which operations scripts can perform:
//
//   - TrustedPermissions() - Allow all operations (default for CLI tools)
//   - SandboxedPermissions() - Allow only safe operations (no I/O)
//   - Custom PermissionConfig - Fine-grained control with allow/deny rules
//
// Permission rules use the format: "module.function" or "module.function(resource)"
//
// Examples:
//
//	"fs.*"                    // Allow all fs operations
//	"os.exec"                 // Allow command execution
//	"fs.read(/tmp/**)"        // Allow reading files in /tmp
//
// # Modules
//
// libkite includes 28+ built-in modules. Use the loader package to register all:
//
//	import "github.com/project-starkite/starkite/libkite/loader"
//
//	registry := loader.NewDefaultRegistry(moduleConfig)
//	config := &libkite.Config{
//		Registry: registry,
//	}
//
// # Testing
//
// Run test_* functions in Starlark scripts:
//
//	results, err := rt.ExecuteTests(code)
//	for _, r := range results {
//		if !r.Passed {
//			log.Printf("Test %s failed: %v", r.Name, r.Error)
//		}
//	}
package libkite

// Version information for libkite.
const (
	Version = "1.0.0"
)
