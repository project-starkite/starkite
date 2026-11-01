# Starkite VS Code Extension

Provides language support for Starkite automation scripts (`.star`), powered by the `kitelsp` language server.

## Features

- **Real-Time Diagnostics**: Syntax errors, unclosed delimiters, and type mismatches.
- **Context-Aware Autocompletion**: Module namespaces (`fs`, `http`, `os`, `k8s`, `sql`, `ssh`), exported methods, and `try_` variants.
- **Hover Documentation**: Function signatures, parameter descriptions, and return types.
- **Signature Help**: Parameter hints while calling functions.
- **Definition Navigation**: Go to definition for `load()` dependencies.

## Requirements

The extension communicates with `kitelsp` over standard I/O.

Install `kitelsp` to your `PATH`:

```bash
make build-kitelsp
cp ./bin/kitelsp /usr/local/bin/
```

Or configure `starkite.serverPath` in your VS Code settings to point to your `kitelsp` executable. If configured with `kite`, the extension will automatically append `lsp` to launch the embedded server.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `starkite.serverPath` | `"kitelsp"` | Path to the `kitelsp` binary (or `kite` built with `-tags lsp`). |
| `starkite.trace.server` | `"off"` | Trace language server JSON-RPC communication (`off`, `messages`, `verbose`). |
