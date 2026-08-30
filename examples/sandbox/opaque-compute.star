#!/usr/bin/env kite
# opaque-compute.star — pure compute over project files inside the opaque
# sandbox profile. Completely offline: no outbound network, no DNS, no TLS roots.
#
# Demonstrates opaque profile properties:
#   1. $CWD is writable — the script reads input.json, transforms it,
#      writes output.json next to itself.
#   2. Completely offline — network connections are blocked at the OS layer.
#
# Run:
#   kite ./opaque-compute.star --sandbox-opaque --permissions=allow-fs
#   STARKITE_SANDBOX_PROFILE=opaque ./opaque-compute.star --permissions=allow-fs

# (1) Compute over project files.
def transform_items(values):
    total = 0
    for v in values:
        total += v
    return total

input_path = path("./input.json")
input_path.write_text('{"items": [1, 2, 3, 4, 5]}')

doc = json.decode(input_path.read_text())
total = transform_items(doc["items"])
output_path = path("./output.json")
output_path.write_text(json.encode({"sum": total}))
print("wrote sum=%d to ./output.json" % total)

input_path.remove()
output_path.remove()

# (2) Verify network isolation (outbound blocked)
result = http.url("https://example.com").try_get(timeout="1s")
assert(not result.ok, "outbound requests must fail in opaque profile")
print("network isolated: outbound blocked as expected")
