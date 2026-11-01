// Starkite language client for VS Code.
const { workspace, window, commands } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const config = workspace.getConfiguration("starkite");
  const command = config.get("serverPath") || "kitelsp";

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

  // Command: Restart Server
  context.subscriptions.push(
    commands.registerCommand("starkite.restartServer", async () => {
      if (!client) {
        return;
      }
      await client.stop();
      await client.start();
      window.showInformationMessage("Starkite Language Server restarted.");
    })
  );

  // Command: Run Current Script
  context.subscriptions.push(
    commands.registerCommand("starkite.runFile", () => {
      const editor = window.activeTextEditor;
      if (!editor) {
        return;
      }
      const filePath = editor.document.fileName;
      const terminal = window.createTerminal("Starkite Runner");
      terminal.show();
      terminal.sendText(`kite run "${filePath}"`);
    })
  );

  // Command: Validate File
  context.subscriptions.push(
    commands.registerCommand("starkite.validateFile", () => {
      const editor = window.activeTextEditor;
      if (!editor) {
        return;
      }
      const filePath = editor.document.fileName;
      const terminal = window.createTerminal("Starkite Validator");
      terminal.show();
      terminal.sendText(`kite validate --lint "${filePath}"`);
    })
  );

  context.subscriptions.push({ dispose: () => client && client.stop() });
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
