#!/usr/bin/env kite
# defense-in-depth.star — demonstrates Starkite's default zero-trust execution.
#
# Two independent layers are active by default under 'deny-all':
#   Starlark call gate: blocks gated module calls (exec, file I/O, network)
#                       at the Starlark API level.
#   OS kernel sandbox:  confines the process view (no host $HOME, no outbound
#                       network sockets, no subprocess execution) at the OS level.
#
# A breach in either layer is contained by the other.
#
# Run:
#   kite run ./defense-in-depth.star

# Pure compute is allowed under both layers.
data = {"items": [1, 2, 3], "label": "demo"}
serialized = json.encode(data)
print("encoded: %s" % serialized)

decoded = json.decode(serialized)
assert(decoded["label"] == "demo", "round-trip mismatch")
print("decoded: label=%s items=%d" % (decoded["label"], len(decoded["items"])))

# These would fail under the default deny-all profile (API-level block):
# os.exec("echo blocked")               # permission denied: os.exec not allowed
# http.url("https://example.com").get() # permission denied: http.client not allowed

# These would also fail under OS kernel sandbox alone (kernel-level block):
# read_text("/etc/passwd")              # file not found (not mounted)
# http.url("https://1.1.1.1").get()     # network unreachable
