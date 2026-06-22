---
title: "Embedding"
description: "libkite — embed the starkite runtime in a Go program"
weight: 30
---

# Embedding

Sometimes the script is not the program — it is a part of one. You have an HTTP service, an agent loop, or a CLI tool written in Go, and you want users (or operators, or an LLM) to supply the bodies of certain actions as `.star` scripts without rebuilding the binary. `libkite` is the embeddable Starlark runtime that powers starkite, and it exists for exactly that: your Go host keeps the control flow, while scripts fill in the work. Modules, permissions, signal handling, and cancellation all stay under your control.

This page is the reference for the public Go API. For runtime semantics, see [Language](../fundamentals/language.md), [Permission](../fundamentals/security/permission.md), and [Modules](../fundamentals/modules.md).

## Installation

Pull the library into your module with `go get`:

```bash
go get github.com/project-starkite/starkite/libkite
```

## Minimal example

The shortest useful program builds a registry of modules, constructs a runtime, and hands it a script to run. Everything else on this page elaborates one of those three steps:

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

Two details carry the example. The script reaches `os.hostname()` and `json.encode(...)` as globals because the registry came from `loader.NewDefaultRegistry`, which populates the base module set — a bare runtime would have neither. And `NewTrusted` runs the script with every operation allowed, which is fine for code you wrote and dangerous for code you did not; the next sections show how to dial that down.

## Constructors

Pick a constructor by how much you trust the script, since each one sets a different starting permission posture:

| Constructor | Permission default |
|---|---|
| `libkite.New(cfg)` | as configured (`Permissions` nil → all allowed) |
| `libkite.NewTrusted(cfg, opts...)` | allow-all |
| `libkite.NewSandboxed(cfg, opts...)` | deny-all |

All three take a `*Config` plus optional `ConfigOption` functions, and either argument may be `nil`. That gives you three equivalent ways to spell the same setup — choose whichever reads best at the call site:

```go
// Config struct only
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})

// Options only
rt, _ := libkite.NewTrusted(nil, libkite.WithRegistry(registry))

// Both
rt, _ := libkite.NewTrusted(cfg, libkite.WithDebug(true))
```

## Registry construction

A runtime can only call the modules its registry holds, so the registry is where you decide what a script is allowed to reach for. The constructors do not assume one for you — `libkite.New(nil)` starts with an empty registry — so reach for a builder that matches the capability set you want:

| Builder | Modules |
|---|---|
| `libkite.NewRegistry(nil)` | empty |
| `loader.NewDefaultRegistry(nil)` | base (27 modules) |
| `cloudloader.NewCloudRegistry(nil)` | base + `k8s` |
| `ailoader.NewAIRegistry(nil)` | base + `ai` + `mcp` |

### Composing module sets (strict mode)

When you assemble a registry from independent sources — the base modules plus a domain-specific bundle — two of them may claim the same name. By default the registry resolves that silently: the second module to register wins and replaces the first, which is convenient until it hides a collision you would rather know about.

To make those collisions loud, opt into strict mode before registering. Strict mode enforces that module names, top-level export keys, and global aliases are unique across the whole registry:

```go
r := libkite.NewRegistry(nil)
r.SetStrict(true)
loader.RegisterAll(r)        // base modules
mybundle.RegisterAll(r)      // additional modules
```

With strict mode on, a conflict surfaces at the earliest point it can:

- `Register` panics on a duplicate `Name()` — so the failure lands at startup, not in the middle of a script run.
- `LoadAll` returns an error on a duplicate top-level export key or a duplicate global alias.

The all-in-one `kite` binary turns strict mode on to keep the base, cloud, and ai edition namespaces disjoint. Lean editions, which only ever load one bundle, leave it off.

## `Config` struct

Most of what a runtime needs comes from its `Config`. Each field tunes one aspect of how scripts run — which modules exist, what they may do, what globals they see, where output goes:

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

If you would rather not build the struct literal, every `Config` field has a matching `With*` option you can pass after the config argument:

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

Once you hold a runtime, several methods drive it, and they differ mainly in granularity — run a whole script, evaluate a single expression, or call one named function:

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

Every `Execute*`, `Eval`, `Call`, and `CallFn` takes a `context.Context` first, and cancelling that context propagates into the Starlark thread — so a deadline on the host call bounds the script:

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

That deadline does not reach everywhere, though. A blocking call inside a module implementation — `http.url(...).get(timeout=...)`, `ssh.connect(timeout=...)` — honors its own `timeout` kwarg, not the outer `ctx`. To guarantee a script cannot hang past a bound, set both: a `context.WithTimeout` on the runtime call *and* an explicit timeout on any module call that may block.

## Calling Starlark functions from Go

Running whole scripts is one mode; the other is treating `libkite` as a function-execution engine, where the host calls individual Starlark functions on demand. That is the shape you want when scripts define tools for an agent loop, an HTTP handler, or a custom CLI. The pattern has two halves — define the functions once, then call them by name:

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

`ExecuteRepl` is what makes this work: it keeps `check_url` bound after the script returns, so a later `rt.Call` can find it. The value that comes back is a `starlark.Value`, and `startype` bridges it to a Go type in both directions, mapping along these lines:

| Go type | Starlark value |
|---|---|
| `string` | `starlark.String` |
| `int`, `int64` | `starlark.Int` |
| `float64` | `starlark.Float` |
| `bool` | `starlark.Bool` |
| `[]any` | `*starlark.List` |
| `map[string]any` | `*starlark.Dict` |

### Common pattern: Go host, Starlark tools

Put those pieces together and an agent loop falls out naturally. Load the tool definitions once, then let the model choose which one to invoke and feed the result back as the next message:

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

The Go host owns the loop and the conversation; the Starlark side owns what each tool actually does. You can edit `toolsSource` without touching the host, which is the whole point of embedding the runtime rather than hard-coding the tools.

## Permissions

A registry decides what modules exist; permissions decide what a script may do with them. Start from a built-in helper that names a coherent capability tier, and reach for a custom config only when none of the tiers fit:

| Helper | Effect |
|---|---|
| `libkite.DenyAllPermissions()` | compute, print, and log only; no fs, network, or exec |
| `libkite.AllowFSPermissions()` | read any file; write/delete within `$CWD`; `os.env`, `io.prompt` |
| `libkite.AllowNetPermissions()` | adds `http.client` and all `ssh` |
| `libkite.AllowLocalPermissions()` | adds `http.server`, `os.exec` under `$CWD`, `ai.generate`, `k8s.read`/`write`/`config`, `mcp.client`/`server` |
| `libkite.AllowAllPermissions()` | every operation allowed, including unrestricted `os.exec`, `k8s.exec`, and `os.process` |
| `&libkite.PermissionConfig{Allow: …, Deny: …, Default: …}` | custom rules |

When the tiers are too coarse, write the policy out as rules. This config, for instance, lets a script read its own config tree, use any `json` function, and reach one API host — and nothing else, because the default is deny:

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

For the full rule grammar, see [Permission](../fundamentals/security/permission.md).

## Custom modules

The built-in registries cover the common ground, but eventually a script needs a capability only your host can supply — a domain API, an internal service. You add it by implementing the `Module` interface and registering it. The interface is small; `Load` is where the actual builtins come from:

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

After the `Register` call, scripts run by this runtime can call `mymod.hello()` alongside every base module. Register it on the same registry you pass to the constructor — a module registered after the runtime is built will not be seen.

## Capturing output

By default a script's `print` goes to stdout, which is rarely what you want when the runtime is buried inside a service. Supply a `Print` function and the runtime routes every `print` through it, so you can redirect output into a buffer, a log, or a response stream:

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

Here every line a script prints lands in `out` instead of the terminal, ready to return to a caller or fold into structured logs.

## Signal handling

A long-running script may need to clean up when the process is interrupted, and `libkite` wires that up for you. Creating a runtime installs OS signal handlers, and on `SIGINT` / `SIGTERM` / `SIGHUP` they run a fixed sequence:

1. A script-registered handler via `on_signal("SIGINT", fn)` runs first.
2. Any `defer(fn)` cleanups run in LIFO order.
3. For `SIGINT` / `SIGTERM`, the process exits with `ExitInterrupt` / `ExitTerminate`.

You can register, query, and remove handlers from the Go side as well, which is useful when the host — not the script — owns the cleanup logic:

```go
rt.RegisterSignalHandler("SIGINT", myStarlarkHandler)
rt.HasSignalHandler("SIGINT")     // → true
rt.UnregisterSignalHandler("SIGINT")
```

On the script side, `on_signal` is a top-level Starlark global, alongside `fail`, `exit`, `defer`, and `Result`.

## Adding Kubernetes support

When scripts need to talk to a cluster, swap the base registry for the cloud one. `cloudloader.NewCloudRegistry` gives you the base modules plus `k8s`:

```go
import (
    "github.com/project-starkite/starkite/libkite"
    cloudloader "github.com/project-starkite/starkite/cloudkite/loader"
)

registry := cloudloader.NewCloudRegistry(nil)   // base + k8s
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})
```

That capability is not free: it pulls in `k8s.io/client-go` and its dependency tree, adding roughly 37 MB to the binary. Reach for it only when scripts actually use `k8s`.

## Adding AI/MCP support

The AI registry follows the same shape, bundling `ai` and `mcp` on top of the base set so scripts can call models and MCP servers:

```go
import (
    "github.com/project-starkite/starkite/libkite"
    ailoader "github.com/project-starkite/starkite/aikite/loader"
)

registry := ailoader.NewAIRegistry(nil)         // base + ai + mcp
rt, _ := libkite.NewTrusted(&libkite.Config{Registry: registry})
```

## Running tests

A host can run a script's own `test_*` functions through the runtime. `ExecuteTests` runs them all and hands back one result per test, so you decide what to do with the failures:

```go
results, err := rt.ExecuteTests(context.Background(), code)
for _, r := range results {
    if !r.Passed {
        fmt.Printf("FAIL: %s — %v\n", r.Name, r.Error)
    }
}
```

To run a subset or see per-assertion detail, pass a `TestConfig`. The filter keeps only tests whose name contains the substring; verbose surfaces each assertion as it runs:

```go
cfg := libkite.TestConfig{Filter: "integration", Verbose: true}
results, _ := rt.ExecuteTestsWithConfig(ctx, code, cfg)
```

One subtlety to expect in the results: `exit(code)` inside a test function counts as a visible test failure, with the result's `Error` wrapping `*libkite.ExitError{Code: code}`. A top-level `exit(code)` in the test script is different — it returns `*libkite.ExitError` straight from `ExecuteTestsWithConfig`.

## Dependency footprint

Every module you bundle is code compiled into your binary, so the registry you choose sets a floor on its size. The progression makes the trade-off concrete — more capability, more megabytes:

| Registry | Modules | Binary size impact |
|---|---|---|
| `libkite.New(nil)` (no registry) | none | ~5 MB |
| `loader.NewDefaultRegistry(nil)` | 27 base | ~26 MB |
| `cloudloader.NewCloudRegistry(nil)` | 27 + k8s | ~63 MB |
| `ailoader.NewAIRegistry(nil)` | 27 + ai + mcp | ~92 MB |
