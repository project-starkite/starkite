#!/usr/bin/env kite
# defense-in-depth.star — composes the strict sandbox profile with the
# deny-all permissions profile.
#
# Two independent layers:
#   --permissions=deny-all  blocks every gated module call (exec, file I/O,
#                           network) at the API level.
#   --sandbox=opaque        confines the OS view (no /etc/*, no $HOME, no
#                           outbound network) at the kernel level via gVisor.
#
# A breach in either layer is contained by the other.
#
# Run:
#   kite ./defense-in-depth.star --sandbox=opaque --permissions=deny-all

# Pure compute is allowed under both layers.
data = {"items": [1, 2, 3], "label": "demo"}
serialized = json.encode(data)
print("encoded: %s" % serialized)

decoded = json.decode(serialized)
assert(decoded["label"] == "demo", "round-trip mismatch")
print("decoded: label=%s items=%d" % (decoded["label"], len(decoded["items"])))

# These would fail under --permissions=deny-all (API-level block):
# os.exec("echo blocked")               # permission denied: os.exec not allowed
# http.url("https://example.com").get() # permission denied: http.client not allowed

# These would also fail under --sandbox=opaque alone (kernel-level block):
# read_text("/etc/passwd")              # file not found (not mounted)
# http.url("https://1.1.1.1").get()     # network unreachable
