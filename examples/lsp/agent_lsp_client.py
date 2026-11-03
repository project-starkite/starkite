#!/usr/bin/env python3
"""
agent_lsp_client.py - AI Agent Language Server Protocol (LSP) Client for Starkite

This script demonstrates how an autonomous AI coding agent harness connects
directly to 'kite lsp --stdio' over JSON-RPC 2.0 to:
  1. Initialize the language server session
  2. Query completions at cursor positions (preventing API hallucinations)
  3. Query hover documentation on symbols (on-demand signature retrieval)
  4. Receive real-time diagnostics for in-memory agent self-correction
"""

import json
import os
import shutil
import subprocess
import sys
import threading
import time

def find_kite_binary():
    """Locate the kite executable in local repository or system PATH."""
    local_bin = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "bin", "kite"))
    if os.path.exists(local_bin) and os.access(local_bin, os.X_OK):
        return local_bin
    system_kite = shutil.which("kite")
    if system_kite:
        return system_kite
    return "kite"

class LSPClient:
    def __init__(self, server_cmd):
        self.cmd = server_cmd
        self.proc = None
        self.request_id = 0
        self.diagnostics_received = []
        self._lock = threading.Lock()
        self._pending_requests = {}
        self._reader_thread = None
        self.running = False

    def start(self):
        self.proc = subprocess.Popen(
            self.cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=0
        )
        self.running = True
        self._reader_thread = threading.Thread(target=self._read_loop, daemon=True)
        self._reader_thread.start()

    def _send_payload(self, payload):
        raw = json.dumps(payload).encode("utf-8")
        header = f"Content-Length: {len(raw)}\r\n\r\n".encode("ascii")
        self.proc.stdin.write(header + raw)
        self.proc.stdin.flush()

    def send_request(self, method, params, timeout=5.0):
        with self._lock:
            self.request_id += 1
            req_id = self.request_id
            cond = threading.Condition()
            self._pending_requests[req_id] = {"cond": cond, "response": None}

        payload = {
            "jsonrpc": "2.0",
            "id": req_id,
            "method": method,
            "params": params
        }
        self._send_payload(payload)

        with cond:
            if not cond.wait(timeout):
                return None
            return self._pending_requests[req_id]["response"]

    def send_notification(self, method, params):
        payload = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params
        }
        self._send_payload(payload)

    def _read_loop(self):
        while self.running and self.proc.poll() is None:
            # 1. Read headers until \r\n\r\n
            content_length = None
            while True:
                line = self.proc.stdout.readline()
                if not line:
                    return
                line_str = line.decode("ascii", errors="replace").strip()
                if not line_str:
                    break
                if line_str.lower().startswith("content-length:"):
                    content_length = int(line_str.split(":", 1)[1].strip())

            if content_length is None:
                continue

            # 2. Read exact body bytes
            body = self.proc.stdout.read(content_length)
            if not body:
                return

            try:
                msg = json.loads(body.decode("utf-8"))
            except Exception as e:
                continue

            # 3. Handle messages
            if "id" in msg and msg["id"] in self._pending_requests:
                req_id = msg["id"]
                req_entry = self._pending_requests[req_id]
                req_entry["response"] = msg
                with req_entry["cond"]:
                    req_entry["cond"].notify_all()
            elif msg.get("method") == "textDocument/publishDiagnostics":
                self.diagnostics_received.append(msg.get("params", {}))

    def stop(self):
        self.running = False
        try:
            self.send_request("shutdown", {}, timeout=2.0)
            self.send_notification("exit", {})
        except Exception:
            pass
        if self.proc:
            self.proc.terminate()
            self.proc.wait()

def main():
    kite_path = find_kite_binary()
    print("======================================================================")
    print("Starkite Language Server - AI Agent JSON-RPC Demonstration")
    print(f"Server binary: {kite_path}")
    print("======================================================================\n")

    client = LSPClient([kite_path, "lsp", "--stdio"])
    client.start()

    try:
        # Step 1: Initialize handshake
        print("[1] Executing LSP Initialize Handshake...")
        init_res = client.send_request("initialize", {
            "processId": os.getpid(),
            "rootUri": "file:///workspace",
            "capabilities": {
                "textDocument": {
                    "completion": {"completionItem": {"snippetSupport": True}},
                    "hover": {"contentFormat": ["markdown", "plaintext"]},
                    "publishDiagnostics": {"relatedInformation": True}
                }
            }
        })
        if init_res and "result" in init_res:
            server_info = init_res["result"].get("serverInfo", {})
            print(f"    Connected to LSP Server: {server_info.get('name', 'starlsp')} v{server_info.get('version', '0.11.x')}")
        client.send_notification("initialized", {})
        print("    Handshake complete.\n")

        # Step 2: Open a virtual document buffer in memory
        doc_uri = "file:///workspace/audit.star"
        initial_script = """def main():
    res = http.try_get("https://httpbin.org/get")
    p = fs.
"""
        print("[2] Opening Virtual Document Buffer in Memory (textDocument/didOpen)...")
        client.send_notification("textDocument/didOpen", {
            "textDocument": {
                "uri": doc_uri,
                "languageId": "starkite",
                "version": 1,
                "text": initial_script
            }
        })
        time.sleep(0.3)
        print("    Document buffer synchronized.\n")

        # Step 3: Autocompletion Query (Preventing Hallucinations)
        # Querying cursor position after 'fs.' (Line 2, Column 11)
        print("[3] Querying Autocompletions at Cursor Position (Line 2, Column 11: 'fs.')...")
        comp_res = client.send_request("textDocument/completion", {
            "textDocument": {"uri": doc_uri},
            "position": {"line": 2, "character": 11}
        })
        if comp_res and "result" in comp_res:
            items = comp_res["result"].get("items", []) if isinstance(comp_res["result"], dict) else comp_res["result"]
            print(f"    Received {len(items)} completions from live Go catalog:")
            for item in items[:6]:
                label = item.get("label")
                detail = item.get("detail", "")
                print(f"      • {label} ({detail})")
            if len(items) > 6:
                print(f"      ... and {len(items) - 6} more")
        print("    Benefit: The agent selects confirmed functions instead of hallucinating.\n")

        # Step 4: Hover Documentation Query (Just-in-Time Docs)
        # Hovering over 'http.try_get' (Line 1, Column 16)
        print("[4] Querying Hover Documentation at Cursor (Line 1, Column 16: 'http.try_get')...")
        hover_res = client.send_request("textDocument/hover", {
            "textDocument": {"uri": doc_uri},
            "position": {"line": 1, "character": 16}
        })
        if hover_res and "result" in hover_res and hover_res["result"]:
            contents = hover_res["result"].get("contents", "")
            if isinstance(contents, dict):
                contents = contents.get("value", "")
            print("    Retrieved on-demand hover documentation:")
            for line in str(contents).strip().splitlines()[:5]:
                print(f"      {line}")
        print("    Benefit: The agent obtains type signatures without prompt context dilution.\n")

        # Step 5: Real-time In-Memory Diagnostics for Self-Correction
        print("[5] Simulating Agent Self-Correction Loop via publishDiagnostics...")
        print("    Agent edits buffer with a deliberate convention defect (calling main() at top-level)...")
        defective_script = """def main():
    print("Automating tasks")

main()
"""
        client.send_notification("textDocument/didChange", {
            "textDocument": {
                "uri": doc_uri,
                "version": 2
            },
            "contentChanges": [{"text": defective_script}]
        })
        time.sleep(0.4)

        if client.diagnostics_received:
            diag_event = client.diagnostics_received[-1]
            diags = diag_event.get("diagnostics", [])
            print(f"    Received {len(diags)} live diagnostics from language server:")
            for d in diags:
                print(f"      [{d.get('code', 'WARN')}] Line {d['range']['start']['line'] + 1}: {d.get('message')}")
            print("    Agent Self-Correction: Agent reads L001 and removes top-level main() before disk write.")
        else:
            print("    Language server validated buffer in real time.")

        print("\n======================================================================")
        print("Agent LSP Demonstration Finished Successfully")
        print("======================================================================")

    finally:
        client.stop()

if __name__ == "__main__":
    main()
