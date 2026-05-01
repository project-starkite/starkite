---
title: "Embedding Libkite"
description: "Using libkite as a library to add Starlark scripting to your Go application"
weight: 7
---

Libkite is the embeddable Starlark runtime that powers starkite. You can use it as a library to add scriptable automation to any Go application.

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/project-starkite/starkite/libkite"
    "github.com/project-starkite/starkite/libkite/loader"
)

func main() {
    // Create a registry with all 27 built-in modules
    registry := loader.NewDefaultRegistry(nil)

    // Create a trusted runtime (all operations allowed)
    rt, err := libkite.NewTrusted(&libkite.Config{
        Registry: registry,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer rt.Close()

    // Execute a script
    if err := rt.Execute(context.Background(), `
        printf("Hello from %s\n", os.hostname())
        data = json.encode({"status": "ok", "count": 42})
        print(data)
    `); err != nil {
        log.Fatal(err)
    }
}
```

## Installation

```bash
go get github.com/project-starkite/starkite/libkite
```

## Creating a Runtime

### With all modules (recommended)

Import the loader package to get all 27 built-in modules:

```go
import (
    "github.com/project-starkite/starkite/libkite"
    "github.com/project-starkite/starkite/libkite/loader"
)

registry := loader.NewDefaultRegistry(nil)
rt, err := libkite.New(&libkite.Config{
    Registry: registry,
})
```

### With trust/sandbox permissions

```go
// Trusted — all operations allowed (for CLI tools, internal scripts)
rt, err := libkite.NewTrusted(&libkite.Config{Registry: registry})

// Sandboxed — only safe operations (for untrusted user scripts)
rt, err := libkite.NewSandboxed(&libkite.Config{Registry: registry})
```

Both accept a `*Config` struct and optional `ConfigOption` functions:

```go
// Config struct only
rt, err := libkite.NewTrusted(&libkite.Config{
    Registry: registry,
})

// Options only
rt, err := libkite.NewTrusted(nil,
    libkite.WithRegistry(registry),
)

// Mix both
rt, err := libkite.NewTrusted(cfg, libkite.WithDebug(true))
```

Timeouts are set by the caller via `context.WithTimeout` and passed into `rt.Execute(ctx, code)` — see [Cancellation via context](#cancellation-via-context).

### Bare runtime (no modules)

If you only need the Starlark engine without built-in modules:

```go
rt, err := libkite.New(nil)
```

This creates a runtime with no modules. Register your own via a custom registry.

### Composing multiple module sets (strict mode)

When you compose module sets that come from independent sources — for example, base modules plus a domain-specific bundle — `Registry` silently overwrites collisions by default: a second module with the same name replaces the first, and a second module that exports the same top-level symbol or global alias wins.

If your composition needs the **invariant that module names, top-level export keys, and global aliases are unique across the whole registry**, opt into strict mode:

```go
r := libkite.NewRegistry(nil)
r.SetStrict(true)
loader.RegisterAll(r)         // base modules
mybundle.RegisterAll(r)       // your additional modules
```

In strict mode:

- `Register` **panics** if you register two modules with the same `Name()` — caught at startup, not at script runtime.
- `LoadAll` returns an error if two modules export the same top-level key or register the same global alias.

This is how the all-edition (`kite`) enforces edition-namespace disjointness across base + cloud + ai. The lean editions leave strict mode off.

## Configuration

The `Config` struct controls all runtime behavior:

```go
config := &libkite.Config{
    // Module registry (nil = empty)
    Registry: registry,

    // Permission policy (nil = allow all)
    Permissions: libkite.SandboxedPermissions(),

    // Global variables injected into every script
    Globals: map[string]interface{}{
        "app_name":    "mytool",
        "app_version": "1.0.0",
    },

    // Redirect print output
    Print: func(thread *starlark.Thread, msg string) {
        logger.Info(msg)
    },

    // Script execution settings
    ScriptPath: "config.star",
    WorkDir:    "/app",

    // Modes
    Debug:  false,
    DryRun: false,
}
```

### Functional Options

All config fields have corresponding `With*` options:

| Option | Description |
|--------|-------------|
| `WithRegistry(r)` | Set module registry |
| `WithPermissions(p)` | Set permission policy |
| `WithTrusted()` | Allow all operations |
| `WithSandboxed()` | Restrict to safe operations |
| `WithGlobals(g)` | Inject global variables |
| `WithPrint(fn)` | Override print function |
| `WithScriptPath(p)` | Set script path for error messages |
| `WithWorkDir(d)` | Set working directory |
| `WithDebug(b)` | Enable debug logging |
| `WithDryRun(b)` | Enable dry-run mode |
| `WithVarStore(vs)` | Set variable store |

## Cancellation via context

Every `Execute*` method and every Go-callable runtime primitive takes a `context.Context` as the first argument. The context is wired to the Starlark thread's cancel flag: when `ctx.Done()` fires, the interpreter observes it at the next safe point and returns an error.

### Timeouts

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := rt.Execute(ctx, script); err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        log.Println("script hit timeout")
    }
    return err
}
```

### Parent cancellation

Pass a parent context (e.g., from an HTTP request) so the script cancels whenever the upstream operation is canceled:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    // r.Context() is canceled when the client disconnects
    if err := rt.Execute(r.Context(), userScript); err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
}
```

### Two-level timeout pitfall

Go-side blocking calls inside libkite modules (e.g., `http.url(...).get(timeout="30s")`, `ssh.connect(timeout="...")`) honor their own kwargs, not the outer `ctx`. If you want guaranteed cancellation, set both: a `context.WithTimeout` on the Runtime call *and* explicit timeouts on module calls that might block.

### Skipping cancellation

Pass `context.Background()` when no cancellation is desired:

```go
rt.Execute(context.Background(), script)
```

## Signal handling

Libkite registers OS signal handlers when a Runtime is created. When a SIGINT/SIGTERM/SIGHUP arrives:

1. If the script registered a handler via `on_signal("SIGINT", fn)`, that callable runs first.
2. Any `defer(fn)` cleanups run in LIFO order.
3. For `SIGINT` / `SIGTERM`, the process exits (`os.Exit(ExitInterrupt)` / `ExitTerminate`).

### From a Starlark script

```python
def on_interrupt(sig):
    print("received", sig, "— shutting down cleanly")

on_signal("SIGINT", on_interrupt)

for host in hosts:
    run_long_task(host)
```

### From Go (register a host-side handler instead)

The same registration surface is available on the Go side, useful when a Go host wants to handle signals without running Starlark:

```go
rt.RegisterSignalHandler("SIGINT", myStarlarkHandler)
rt.HasSignalHandler("SIGINT")       // → true
rt.UnregisterSignalHandler("SIGINT")
```

Note: `on_signal` is a top-level Starlark global, not a method on any module. It's registered alongside `fail`, `exit`, `defer`, and `Result`.

## Calling Starlark functions from Go

Beyond running whole scripts, `Runtime` exposes four methods that let a Go host invoke Starlark from outside a script file. This is the primary API for embedding libkite as a **tool execution engine** — a pattern where Go owns the outer control flow (e.g., an LLM agent loop, an HTTP handler) and Starlark defines the bodies of actions.

### `Runtime.Call(ctx, name, args, kwargs)`

Invoke a top-level callable registered in the runtime's globals. `args` is `[]any`, `kwargs` is `map[string]any`. Either can be `nil`. Returns `starlark.Value`.

```go
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})
defer rt.Close()

// Define a tool via ExecuteRepl, then call it from Go.
_ = rt.ExecuteRepl(context.Background(), `
def check_url(url):
    r = http.url(url).get(timeout="5s")
    return {"status": r.status_code, "ok": r.status_code < 400}
`)

val, err := rt.Call(context.Background(), "check_url",
    nil,                                          // no positional args
    map[string]any{"url": "https://example.com"}, // kwargs
)
if err != nil {
    return err
}

// Convert to Go via startype
var out map[string]any
_ = startype.Starlark(val).ToGoValue(&out)
fmt.Printf("status=%v ok=%v\n", out["status"], out["ok"])
```

Values convert from Go to Starlark via `startype`:

| Go type | Starlark value |
|---------|----------------|
| `string` | `starlark.String` |
| `int`, `int64` | `starlark.Int` |
| `float64` | `starlark.Float` |
| `bool` | `starlark.Bool` |
| `[]any` | `*starlark.List` |
| `map[string]any` | `*starlark.Dict` |

### `Runtime.CallFn(ctx, fn, args, kwargs)`

When the caller already holds a `starlark.Callable` (e.g., from `GetGlobalVal`), skip the name lookup:

```go
fnVal, ok := rt.GetGlobalVal("check_url")
if !ok {
    return errors.New("check_url not defined")
}
fn := fnVal.(starlark.Callable)

val, err := rt.CallFn(context.Background(), fn,
    nil,
    map[string]any{"url": "https://example.com"},
)
```

### `Runtime.Eval(ctx, src)`

Evaluate a Starlark **expression** (not a statement) against the runtime's predeclared symbols plus any globals from prior `Execute`/`ExecuteRepl` calls:

```go
v, err := rt.Eval(context.Background(), `1 + 2 * 3`)
// v is starlark.Int(7)

// After defining a function via ExecuteRepl:
v, err = rt.Eval(context.Background(), `check_url("https://example.com")["ok"]`)
```

Statements (like `x = 1`) error — use `ExecuteRepl` for those.

### `Runtime.GetGlobalVal(name)`

Look up a defined global by name. Returns `(starlark.Value, bool)`:

```go
fn, ok := rt.GetGlobalVal("check_url")
if !ok {
    return errors.New("tool not defined")
}
// fn can be type-asserted to starlark.Callable, or passed to CallFn directly.
```

### Common pattern: Go host, Starlark tools

This layering pairs well with agent loops. The Go host owns the LLM client and tool-schema JSON; libkite runs the body of each tool the model calls:

```go
// 1. Register tool bodies
_ = rt.ExecuteRepl(context.Background(), toolsSource)

// 2. Inside the agent loop
for {
    resp, _ := llmClient.Chat(ctx, messages, toolSchemas)
    if resp.ToolCall == nil {
        break
    }
    result, err := rt.Call(ctx, resp.ToolCall.Name, nil, resp.ToolCall.Args)
    if err != nil {
        // handle
    }
    messages = append(messages, resultMessage(result))
}
```

The libkite modules (http, fs, k8s, ssh, …) provide the action surface the model can reach through these tool bodies.

## Custom Modules

Register your own modules alongside the built-ins:

```go
import "github.com/project-starkite/starkite/libkite"

// Implement the Module interface
type MyModule struct{}

func (m *MyModule) Name() libkite.ModuleName { return "mymod" }
func (m *MyModule) Description() string        { return "My custom module" }
func (m *MyModule) Aliases() starlark.StringDict { return nil }
func (m *MyModule) FactoryMethod() string      { return "" }

func (m *MyModule) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
    return starlark.StringDict{
        "hello": starlark.NewBuiltin("mymod.hello", func(thread *starlark.Thread,
            fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
            return starlark.String("Hello from my module!"), nil
        }),
    }, nil
}

// Register it
registry := loader.NewDefaultRegistry(nil)
registry.Register(&MyModule{})

rt, err := libkite.NewTrusted(&libkite.Config{Registry: registry})
```

Scripts can then use `mymod.hello()`.

## Permission Sandboxing

Control what scripts can do:

```go
// Allow everything
config.Permissions = libkite.TrustedPermissions()

// Block dangerous operations (exec, file writes, network)
config.Permissions = libkite.SandboxedPermissions()

// Fine-grained rules
config.Permissions = &libkite.PermissionConfig{
    Allow: []string{
        "fs.read_text(./config/**)",  // read config files only
        "json.*",                      // all JSON operations
        "strings.*",                   // all string operations
        "http.get",                    // HTTP GET only
    },
    Deny: []string{
        "os.exec",                    // no command execution
        "fs.write",                   // no file writes
    },
    Default: libkite.DefaultDeny,    // deny anything not in allow list
}
```

## Capturing Output

Redirect `print()` output:

```go
var output strings.Builder

rt, err := libkite.NewTrusted(&libkite.Config{
    Registry: registry,
    Print: func(thread *starlark.Thread, msg string) {
        output.WriteString(msg)
        output.WriteString("\n")
    },
})

rt.Execute(context.Background(), `print("captured")`)
fmt.Println(output.String()) // "captured\n"
```

## WASM Module Support

WASM plugin support is optional. Import the wasm package to enable it:

```go
import (
    "github.com/project-starkite/starkite/libkite/loader"
    "github.com/project-starkite/starkite/wasm"
)

registry := loader.NewDefaultRegistry(nil)

// Register WASM plugins from ~/.starkite/modules/wasm/
wasm.RegisterPlugins(registry, "")
```

If you don't import the wasm package, your binary is ~2.5MB smaller.

## Adding Cloud Modules (Kubernetes)

Import the cloud loader to add Kubernetes support alongside the base modules:

```go
import (
    "github.com/project-starkite/starkite/libkite"
    cloudloader "github.com/project-starkite/starkite/kitecloud/loader"
)

// NewCloudRegistry registers all 27 base modules + k8s module
registry := cloudloader.NewCloudRegistry(nil)

rt, err := libkite.NewTrusted(&libkite.Config{Registry: registry})
if err != nil {
    log.Fatal(err)
}
defer rt.Close()

rt.Execute(context.Background(), `
    # Kubernetes operations are now available
    pods = k8s.list("pods", namespace="default")
    for pod in pods:
        printf("%s: %s\n", pod["metadata"]["name"], pod["status"]["phase"])
`)
```

This pulls in `k8s.io/client-go` and related Kubernetes dependencies (~37MB added to binary). Only import the cloud loader if your tool needs Kubernetes.

### Example: Kubernetes Automation Tool

```go
package main

import (
    "log"
    "os"

    "github.com/project-starkite/starkite/libkite"
    cloudloader "github.com/project-starkite/starkite/kitecloud/loader"
)

func main() {
    registry := cloudloader.NewCloudRegistry(nil)
    rt, err := libkite.NewTrusted(&libkite.Config{
        Registry:   registry,
        ScriptPath: os.Args[1],
        Globals: map[string]interface{}{
            "cluster": os.Getenv("CLUSTER_NAME"),
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer rt.Close()

    script, _ := os.ReadFile(os.Args[1])
    if err := rt.Execute(context.Background(), string(script)); err != nil {
        log.Fatal(err)
    }
}
```

Users can then write scripts with full k8s access:

```python
# deploy.star
printf("Deploying to cluster: %s\n", cluster)
k8s.apply({
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {"name": "web", "namespace": "default"},
    "spec": {"replicas": 3, "selector": {"matchLabels": {"app": "web"}},
        "template": {"metadata": {"labels": {"app": "web"}},
            "spec": {"containers": [{"name": "web", "image": "nginx:latest"}]}}},
})
```

### Dependency Trade-offs

| Registry | Modules | Binary Size Impact |
|----------|---------|-------------------|
| `loader.NewDefaultRegistry(nil)` | 27 base modules | ~26MB |
| `loader.NewDefaultRegistry(nil)` + `wasm.RegisterPlugins()` | 27 + WASM | ~29MB |
| `cloudloader.NewCloudRegistry(nil)` | 27 + k8s | ~63MB |
| `libkite.New(nil)` (no registry) | None | ~5MB |

Choose the registry that matches your tool's needs. Most tools only need the base modules.

## Built-in Modules

When using `loader.NewDefaultRegistry()`, scripts get access to 27 modules:

| Category | Modules |
|----------|---------|
| System | `os`, `fs`, `io`, `runtime` |
| Data | `json`, `yaml`, `csv`, `template`, `base64`, `hash`, `gzip`, `zip` |
| Text | `strings`, `regexp`, `fmt` |
| Network | `http` (client + server), `ssh`, `inventory` |
| Execution | `concur`, `retry` |
| Utility | `time`, `uuid`, `log`, `table`, `vars`, `path` |
| Testing | `test` |

## Running Tests

Execute test functions in scripts:

```go
results, err := rt.ExecuteTests(context.Background(), code)
for _, r := range results {
    if !r.Passed {
        fmt.Printf("FAIL: %s — %v\n", r.Name, r.Error)
    }
}
```

With filter + custom config, use `ExecuteTestsWithConfig`:

```go
cfg := libkite.TestConfig{
    Filter:  "integration",  // run only test_* whose names contain this substring
    Verbose: true,
}
results, err := rt.ExecuteTestsWithConfig(ctx, code, cfg)
```

An `exit(code)` inside a test function is treated as a visible test failure (the result's `Error` wraps `*libkite.ExitError{Code: code}`) — not a silent process exit. A top-level `exit(code)` in the test script itself returns `*libkite.ExitError` from `ExecuteTestsWithConfig`.

## Other Runtime methods

For custom embedding scenarios:

| Method | Purpose |
|--------|---------|
| `rt.NewThread(name) *starlark.Thread` | Create a thread pre-configured with the runtime's permissions and print function. Useful when you need to call `starlark.Call` yourself on a callable you hold. |
| `rt.PrintVariables()` | Print all variables from the runtime's configured `VarStore` to stdout. Debug helper. |
| `rt.Registry() *libkite.Registry` | Access the module registry — e.g., to register a module after `New` or inspect what's loaded. |
| `rt.Permissions() *PermissionChecker` | Access the active permission checker — e.g., to run manual checks. |

## Example: Config File Evaluator

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/project-starkite/starkite/libkite"
    "github.com/project-starkite/starkite/libkite/loader"
)

func main() {
    registry := loader.NewDefaultRegistry(nil)
    rt, err := libkite.NewSandboxed(&libkite.Config{
        Registry:   registry,
        ScriptPath: "config.star",
        Globals: map[string]interface{}{
            "env": os.Getenv("APP_ENV"),
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer rt.Close()

    script, _ := os.ReadFile("config.star")
    if err := rt.Execute(context.Background(), string(script)); err != nil {
        log.Fatal(err)
    }
}
```
