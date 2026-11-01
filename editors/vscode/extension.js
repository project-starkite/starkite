// Starkite language client for VS Code.
//
// The client connects to the kitelsp standalone language server (or "kite lsp" fallback)
// over stdio. Everything an editor displays — diagnostics, completion, hover — comes
// from the server.

const { workspace, window } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const config = workspace.getConfiguration("starkite");
  const command = config.get("serverPath") || "kitelsp";

  // If command is "kite" or points to a kite binary, pass ["lsp"] as argument.
  // If command is "kitelsp", execute directly with no arguments.
  const isKiteBinary = /(^|[\\/])kite(\.exe)?$/.test(command);
  const serverArgs = isKiteBinary ? ["lsp"] : [];

  const serverOptions = {
    run: { command, args: serverArgs, transport: TransportKind.stdio },
    debug: { command, args: serverArgs, transport: TransportKind.stdio },
  };

  const clientOptions = {
    documentSelector: [{ scheme: "file", language: "starkite" }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/mod.{yaml,lock}"),
    },
    outputChannelName: "Starkite",
  };

  client = new LanguageClient(
    "starkite",
    "Starkite Language Server",
    serverOptions,
    clientOptions
  );

  client.start().catch((err) => {
    const targetDesc = command + (serverArgs.length ? " " + serverArgs.join(" ") : "");
    window.showErrorMessage(
      `Starkite: could not start "${targetDesc}". Ensure kitelsp is installed in your PATH ` +
        `(make build-kitelsp) or configure starkite.serverPath in settings. (${err.message})`
    );
  });

  context.subscriptions.push({ dispose: () => client && client.stop() });
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
