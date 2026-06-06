---
title: "Embedding"
description: "libkite — embed the starkite runtime in a Go program"
weight: 30
---

# Embedding

`libkite` is the embeddable Starlark runtime that powers starkite. A Go program imports it as a library to add scriptable automation: the host owns the control flow (HTTP handler, agent loop, CLI tool, …) while `.star` scripts define the bodies of actions. Modules, permissions, signal handling, and cancellation are all available to the host.

This page is the reference for the public Go API. For runtime semantics, see [Language](../fundamentals/language.md), [Permission](../fundamentals/security/permission.md), and [Modules](../fundamentals/modules.md).

## Installation

```bash
go get github.com/project-starkite/starkite/libkite
```

## Minimal example

```go
package main

import (
    "context"
    "log"

    "github.com/project-starkite/starkite/libkite"
    "github.com/project-starkite/starkite/libkite/loader"
)

func main() {
    registry := loader.NewDefaultRegistry(nil)

    rt, err := libkite.NewTrusted(&libkite.Config{
        Registry: registry,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer rt.Close()

    err = rt.Execute(context.Background(), `
        printf("Hello from %s\n", os.hostname())
        data = json.encode({"status": "ok"})
        print(data)
    `)
    if err != nil {
        log.Fatal(err)
    }
}
```

## Constructors

| Constructor | Permission default |
|---|---|
| `libkite.New(cfg)` | as configured (`Permissions` nil → all allowed) |
| `libkite.NewTrusted(cfg, opts...)` | allow-all |
| `libkite.NewSandboxed(cfg, opts...)` | deny-all |

All three accept a `*Config` plus optional `ConfigOption` functions. Either may be `nil`.

```go
// Config struct only
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})

// Options only
rt, _ := libkite.NewTrusted(nil, libkite.WithRegistry(registry))

// Both
rt, _ := libkite.NewTrusted(cfg, libkite.WithDebug(true))
```

## Registry construction

| Builder | Modules |
|---|---|
| `libkite.NewRegistry(nil)` | empty |
| `loader.NewDefaultRegistry(nil)` | base (27 modules) |
| `cloudloader.NewCloudRegistry(nil)` | base + `k8s` |
| `ailoader.NewAIRegistry(nil)` | base + `genai` + `mcp` |

### Composing module sets (strict mode)

When composing module sets from independent sources — base modules plus a domain-specific bundle — the registry silently overwrites collisions by default: a second module with the same name replaces the first.

To enforce that module names, top-level export keys, and global aliases are unique across the whole registry, opt into strict mode:

```go
r := libkite.NewRegistry(nil)
r.SetStrict(true)
loader.RegisterAll(r)        // base modules
mybundle.RegisterAll(r)      // additional modules
```

In strict mode:

- `Register` panics on duplicate `Name()` — caught at startup, not at script runtime.
- `LoadAll` returns an error on duplicate top-level export keys or duplicate global aliases.

The all-in-one `kite` binary uses strict mode to enforce edition-namespace disjointness across base + cloud + ai. Lean editions leave strict mode off.

## `Config` struct

```go
type Config struct {
    Registry    *Registry            // module registry (nil = empty)
    Permissions *PermissionConfig    // permission policy (nil = all allowed)
    Globals     map[string]interface{} // global variables injected into every script
    Print       func(*starlark.Thread, string) // override print output
    ScriptPath  string               // script path for error messages
    WorkDir     string               // working directory
    Debug       bool
    DryRun      bool
}
```

### Functional options

Every `Config` field has a corresponding `With*` option:

| Option | Sets |
|---|---|
| `WithRegistry(r)` | `Registry` |
| `WithPermissions(p)` | `Permissions` |
| `WithTrusted()` | `Permissions = AllowAllPermissions()` |
| `WithSandboxed()` | `Permissions = DenyAllPermissions()` |
| `WithGlobals(g)` | `Globals` |
| `WithPrint(fn)` | `Print` |
| `WithScriptPath(p)` | `ScriptPath` |
| `WithWorkDir(d)` | `WorkDir` |
| `WithDebug(b)` | `Debug` |
| `WithDryRun(b)` | `DryRun` |
| `WithVarStore(vs)` | variable store |

## Execution methods

| Method | Purpose |
|---|---|
| `rt.Execute(ctx, src)` | Run a script. `src` is a string of Starlark source. |
| `rt.ExecuteRepl(ctx, src)` | Run a script and retain its top-level bindings across calls. |
| `rt.ExecuteTests(ctx, src)` | Run every `def test_*` and return per-test results. |
| `rt.ExecuteTestsWithConfig(ctx, src, cfg)` | As above, with name filter and verbose flag. |
| `rt.Eval(ctx, expr)` | Evaluate a Starlark *expression* (not a statement). |
| `rt.Call(ctx, name, args, kwargs)` | Call a defined function by name. |
| `rt.CallFn(ctx, fn, args, kwargs)` | Call a `starlark.Callable` directly. |
| `rt.GetGlobalVal(name)` | Look up a top-level binding by name. |
| `rt.NewThread(name)` | Create a `starlark.Thread` pre-configured with the runtime's permissions and print. |
| `rt.PrintVariables()` | Dump the configured `VarStore` to stdout. Debug helper. |
| `rt.Registry()` | Access the module registry. |
| `rt.Permissions()` | Access the active permission checker. |
| `rt.Close()` | Release resources. Call before the runtime goes out of scope. |

### Cancellation via context

Every `Execute*`, `Eval`, `Call`, and `CallFn` takes a `context.Context` as the first argument. Cancellation propagates to the Starlark thread:

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

Blocking calls inside module implementations (`http.url(...).get(timeout=...)`, `ssh.connect(timeout=...)`) honor their own kwargs, not the outer `ctx`. For guaranteed cancellation, set both: a `context.WithTimeout` on the runtime call *and* explicit timeouts on module calls that may block.

## Calling Starlark functions from Go

When the host wants to invoke individual Starlark functions instead of running whole scripts — the pattern that embeds `libkite` as a tool execution engine for agent loops, HTTP handlers, or custom CLIs:

```go
// Define a tool in REPL mode so its top-level bindings persist.
_ = rt.ExecuteRepl(context.Background(), `
def check_url(url):
    r = http.url(url).get(timeout="5s")
    return {"status": r.status_code, "ok": r.status_code < 400}
`)

// Call it from Go.
val, err := rt.Call(context.Background(), "check_url",
    nil,                                           // positional args
    map[string]any{"url": "https://example.com"},  // kwargs
)

// Convert to Go via startype.
var out map[string]any
_ = startype.Starlark(val).ToGoValue(&out)
```

`startype` handles Go ↔ Starlark conversion in both directions:

| Go type | Starlark value |
|---|---|
| `string` | `starlark.String` |
| `int`, `int64` | `starlark.Int` |
| `float64` | `starlark.Float` |
| `bool` | `starlark.Bool` |
| `[]any` | `*starlark.List` |
| `map[string]any` | `*starlark.Dict` |

### Common pattern: Go host, Starlark tools

```go
_ = rt.ExecuteRepl(context.Background(), toolsSource)

for {
    resp, _ := llmClient.Chat(ctx, messages, toolSchemas)
    if resp.ToolCall == nil {
        break
    }
    result, _ := rt.Call(ctx, resp.ToolCall.Name, nil, resp.ToolCall.Args)
    messages = append(messages, resultMessage(result))
}
```

## Permissions

| Helper | Effect |
|---|---|
| `libkite.DenyAllPermissions()` | compute, print, and log only; no fs, network, or exec |
| `libkite.AllowFSPermissions()` | read any file; write/delete within `$CWD`; `os.env`, `io.prompt` |
| `libkite.AllowNetPermissions()` | adds `http.client` and all `ssh` |
| `libkite.AllowLocalPermissions()` | adds `http.server`, `os.exec` under `$CWD`, `ai.generate`, `k8s.read`/`write`/`config`, `mcp.client`/`server` |
| `libkite.AllowAllPermissions()` | every operation allowed, including unrestricted `os.exec`, `k8s.exec`, and `os.process` |
| `&libkite.PermissionConfig{Allow: …, Deny: …, Default: …}` | custom rules |

```go
config.Permissions = &libkite.PermissionConfig{
    Allow: []string{
        "fs.read($CWD/config/**)",
        "json.*",
        "http.client(api.example.com)",
    },
    Deny:    []string{"os.exec", "fs.write"},
    Default: libkite.DefaultDeny,
}
```

Rule grammar: see [Permission](../fundamentals/security/permission.md).

## Custom modules

Implement the `Module` interface and register it with the registry:

```go
type MyModule struct{}

func (m *MyModule) Name() libkite.ModuleName     { return "mymod" }
func (m *MyModule) Description() string          { return "My custom module" }
func (m *MyModule) Aliases() starlark.StringDict { return nil }
func (m *MyModule) FactoryMethod() string        { return "" }

func (m *MyModule) Load(*libkite.ModuleConfig) (starlark.StringDict, error) {
    return starlark.StringDict{
        "hello": starlark.NewBuiltin("mymod.hello",
            func(thread *starlark.Thread, fn *starlark.Builtin,
                args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
                return starlark.String("hello from mymod"), nil
            }),
    }, nil
}

registry := loader.NewDefaultRegistry(nil)
registry.Register(&MyModule{})
```

## Capturing output

```go
var out strings.Builder

rt, _ := libkite.NewTrusted(&libkite.Config{
    Registry: registry,
    Print: func(thread *starlark.Thread, msg string) {
        out.WriteString(msg)
        out.WriteString("\n")
    },
})
```

## Signal handling

Libkite registers OS signal handlers when a Runtime is created. On `SIGINT`/`SIGTERM`/`SIGHUP`:

1. A script-registered handler via `on_signal("SIGINT", fn)` runs first.
2. Any `defer(fn)` cleanups run in LIFO order.
3. For `SIGINT` / `SIGTERM`, the process exits with `ExitInterrupt` / `ExitTerminate`.

Host-side handler registration:

```go
rt.RegisterSignalHandler("SIGINT", myStarlarkHandler)
rt.HasSignalHandler("SIGINT")     // → true
rt.UnregisterSignalHandler("SIGINT")
```

`on_signal` is a top-level Starlark global alongside `fail`, `exit`, `defer`, and `Result`.

## Adding Kubernetes support

```go
import (
    "github.com/project-starkite/starkite/libkite"
    cloudloader "github.com/project-starkite/starkite/cloudkite/loader"
)

registry := cloudloader.NewCloudRegistry(nil)   // base + k8s
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})
```

Pulls in `k8s.io/client-go` and Kubernetes dependencies — about 37 MB added to the binary.

## Adding AI/MCP support

```go
import (
    "github.com/project-starkite/starkite/libkite"
    ailoader "github.com/project-starkite/starkite/aikite/loader"
)

registry := ailoader.NewAIRegistry(nil)         // base + genai + mcp
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})
```

## Running tests

```go
results, err := rt.ExecuteTests(context.Background(), code)
for _, r := range results {
    if !r.Passed {
        fmt.Printf("FAIL: %s — %v\n", r.Name, r.Error)
    }
}
```

With a name filter and verbose output:

```go
cfg := libkite.TestConfig{Filter: "integration", Verbose: true}
results, _ := rt.ExecuteTestsWithConfig(ctx, code, cfg)
```

`exit(code)` inside a test function is treated as a visible test failure (the result's `Error` wraps `*libkite.ExitError{Code: code}`). A top-level `exit(code)` in the test script returns `*libkite.ExitError` from `ExecuteTestsWithConfig`.

## Dependency footprint

| Registry | Modules | Binary size impact |
|---|---|---|
| `libkite.New(nil)` (no registry) | none | ~5 MB |
| `loader.NewDefaultRegistry(nil)` | 27 base | ~26 MB |
| `cloudloader.NewCloudRegistry(nil)` | 27 + k8s | ~63 MB |
| `ailoader.NewAIRegistry(nil)` | 27 + genai + mcp | ~92 MB |
