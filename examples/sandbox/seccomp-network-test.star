#!/usr/bin/env kite
# seccomp-network-test.star — Tests Seccomp-BPF network sandboxing
# across both isolated (network="none") and allowed (network="host") modes.
#
# This script exercises:
#   1. In-process HTTP socket creation (via http.try_get).
#   2. Child process network socket creation (via os.exec curl).
#   3. Local compute and file operations to ensure the script functions normally.
#
# Usage:
#   # 1. Test in isolated mode (network="none", Seccomp-BPF blocks all network sockets):
#   kite run ./seccomp-network-test.star --sandbox-opaque --permissions=allow-all
#
#   # 2. Test in allowed mode (network="host", Host network access permitted):
#   kite run ./seccomp-network-test.star --sandbox-net --permissions=allow-all

def main():
    print("=== Starkite Sandbox Network Test ===")

    # --- Step 1: In-Process HTTP Socket Test ---
    print("\n[Step 1] In-process network request (http.try_get)...")
    res = http.try_get("https://1.1.1.1", timeout="2s")

    if res.ok:
        print("  -> Result: ALLOWED (HTTP status: %d)" % res.value.status_code)
    else:
        print("  -> Result: BLOCKED")
        print("  -> Error:  %s" % res.error)

    # --- Step 2: Child Process Network Test (curl) ---
    print("\n[Step 2] Child process network request (os.exec curl)...")
    curl_res = os.try_exec("curl", ["--connect-timeout", "2", "-s", "https://1.1.1.1"])

    if curl_res.ok and curl_res.code == 0:
        print("  -> Result: ALLOWED (curl exited 0)")
    else:
        print("  -> Result: BLOCKED (curl exit code: %d, error: %s)" % (curl_res.code, curl_res.error))

    # --- Step 3: Local Compute & File Operations ---
    print("\n[Step 3] Verifying local script compute and file I/O...")
    test_file = path("./.seccomp_local_test.json")
    test_data = {"test": "seccomp-bpf", "timestamp": time.now().unix, "status": "ok"}

    # Write and read back local data
    test_file.write_text(json.encode(test_data))
    read_data = json.decode(test_file.read_text())
    test_file.remove()

    if read_data["status"] != "ok":
        fail("local file I/O verification failed")
    print("  -> Local JSON file operations: SUCCESS")

    # Summary
    if not res.ok:
        print("\n[VERIFICATION] Network is ISOLATED by kernel Seccomp-BPF. Script completed successfully.")
    else:
        print("\n[VERIFICATION] Network is REACHABLE. Script completed successfully.")

main()
