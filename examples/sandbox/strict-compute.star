#!/usr/bin/env kite
# strict-compute.star — pure compute over project files inside the strict
# sandbox profile. No outbound network, no DNS, no TLS roots.
#
# Demonstrates two strict-profile properties:
#   1. $CWD is writable — the script reads input.json, transforms it,
#      writes output.json next to itself.
#   2. Loopback networking works inside the sandbox — an in-script
#      http.server() and http.url() client round-trip with no host
#      involvement.
#
# Run:
#   kite strict-compute.star --sandbox=strict --permissions=allow-local
#   STARKITE_SECURITY_SANDBOX=strict ./strict-compute.star --permissions=allow-local

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

# (2) In-sandbox loopback as a self-contained test fixture.
def echo_handler(req):
    body = req.body if req.body else "empty"
    return {"status": 200, "body": body}

srv = http.server()
srv.handle("/echo", echo_handler)
srv.start(port=0)

resp = http.url("http://127.0.0.1:%d/echo" % srv.port()).post(body="ping")
print("loopback echo: status=%d body=%s" % (resp.status_code, resp.get_text()))
srv.shutdown()

# Outbound to non-loopback addresses fails. Uncomment to observe:
# r = http.url("https://example.com").try_get(timeout="2s")
# print("outbound ok? %s; error: %s" % (r.ok, r.error))
