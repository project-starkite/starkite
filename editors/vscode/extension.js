// Starkite language client for VS Code.
//
// The client is deliberately thin: it starts `kite lsp` and hands the buffer
// over. Everything an editor shows — diagnostics, completion, hover — comes
// from the server, so a feature added there needs no change here.

const { workspace, window } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const config = workspace.getConfiguration("starkite");
  const command = config.get("serverPath") || "kite";

  const serverOptions = {
    run: { command, args: ["lsp"], transport: TransportKind.stdio },
    debug: { command, args: ["lsp"], transport: TransportKind.stdio },
  };

  const clientOptions = {
    documentSelector: [{ scheme: "file", language: "starkite" }],
    synchronize: {
      // The server resolves load() targets through these, so a change to
      // either can change what go-to-definition finds.
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
    window.showErrorMessage(
      `Starkite: could not start "${command} lsp". Build kite with the lsp tag: ` +
        `go build -tags lsp ./kite. (${err.message})`
    );
  });

  context.subscriptions.push({ dispose: () => client && client.stop() });
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
