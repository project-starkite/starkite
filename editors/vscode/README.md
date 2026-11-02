# Starkite VS Code Extension

Provides rich language intelligence, static analysis diagnostics, and script execution for Starkite automation scripts (`.star`), powered by the `kite lsp` embedded language server.

## Features

- **Static Analysis & Diagnostics**: Syntax validation, dialect convention checks, unchecked `try_` Result warnings (`L002`), deprecation hints (`L003`), sandbox execution policy guidance (`L004`), and `load()` import verification (`L005`).
- **Context-Aware Autocompletion**: Module namespaces (`fs`, `http`, `os`, `k8s`, `sql`, `ssh`, `concur`), exported methods, and `try_` error-handling variants.
- **Hover Documentation**: Function signatures, parameter descriptions, and return types extracted directly from runtime metadata.
- **Signature Help**: Parameter hints while authoring function calls.
- **Definition Navigation**: Go to definition for `load()` module dependencies.
- **Command Palette Integration**:
  - `Starkite: Restart Language Server`: Re-initializes the language server.
  - `Starkite: Validate Script (with Lints)`: Executes `kite validate --lint` on the active file.
  - `Starkite: Run Current Script in Sandbox`: Launches `kite run` in an integrated terminal under default sandbox containment.

## Requirements

The extension communicates with `kite lsp` over standard I/O.

Ensure `kite` is installed to your `PATH`:

```bash
# Build from repository root
make kite
sudo cp ./bin/kite /usr/local/bin/
```

Or configure `starkite.serverPath` in your VS Code settings to point to your `kite` executable.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `starkite.serverPath` | `"kite"` | Path to the Starkite binary (defaults to `kite`). |
| `starkite.trace.server` | `"off"` | Trace language server JSON-RPC communication (`off`, `messages`, `verbose`). |
