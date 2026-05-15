---
title: "Run inside a sandbox"
description: "OS-level isolation with the gVisor sandbox profile (Linux)"
weight: 7
---

# Run inside a sandbox

OS-level isolation for untrusted scripts. The `--sandbox[=<profile>]` flag runs the script inside a [gVisor](https://gvisor.dev) user-space kernel — the script sees only the directories the profile mounts (no `$HOME`, no host credentials, no `/etc/passwd`), and its outbound network can be cut to loopback. The host filesystem outside the mounted paths is invisible from inside the sandbox, even under a runtime compromise.

The sandbox composes cleanly with `--permissions` — independent layers, one at the kernel level via gVisor, the other at the Starlark API level inside the process. Linux-only.

Two built-in profiles:

| Profile | Filesystem | Network |
|---|---|---|
| `default` (bare `--sandbox`) | `$CWD` rw, `/tmp` tmpfs, read-only `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` | Host network — full reachability |
| `strict` | `$CWD` rw, `/tmp` tmpfs only | Loopback only — no outbound |

## Default profile

**Source:** [`examples/sandbox/default-http-fetch.star`](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/default-http-fetch.star)

```python
#!/usr/bin/env kite

resp = http.url("https://example.com").get()
print("status: %d" % resp.status_code)
print("body length: %d bytes" % len(resp.get_text()))
```

```bash
kite run examples/sandbox/default-http-fetch.star --sandbox
```

The script reaches `example.com` over HTTPS; the curated `/etc/ssl/certs` mount provides CA roots. `$HOME` and `/etc/passwd` are invisible.

## Strict profile

**Source:** [`examples/sandbox/strict-compute.star`](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/strict-compute.star)

```python
#!/usr/bin/env kite

# (1) Compute over project files inside $CWD.
input_path = path("./input.json")
input_path.write_text('{"items": [1, 2, 3, 4, 5]}')

doc = json.decode(input_path.read_text())
total = sum(doc["items"])
output_path = path("./output.json")
output_path.write_text(json.encode({"sum": total}))
print("wrote sum=%d to ./output.json" % total)

input_path.remove()
output_path.remove()

# (2) Loopback works inside the strict sandbox; outbound does not.
def echo(req):
    return {"status": 200, "body": req.body or "empty"}

srv = http.server()
srv.handle("/echo", echo)
srv.start(port=0)

resp = http.url("http://127.0.0.1:%d/echo" % srv.port()).post(body="ping")
print("loopback echo: status=%d body=%s" % (resp.status_code, resp.get_text()))
srv.shutdown()
```

```bash
kite run examples/sandbox/strict-compute.star --sandbox=strict
```

Outbound to non-loopback addresses fails — uncomment the trailing `try_get` block in the source to observe the error.

## Shebang scripts

For scripts launched via shebang, set the env var:

```bash
STARKITE_SECURITY_SANDBOX=strict ./my-script.star
```

The CLI flag still wins when both are present.

## Compose with permissions

The two layers compose cleanly. Sandbox bounds *what the process can see*; permissions bound *what the script may invoke*. For untrusted scripts, use both:

```bash
kite run untrusted.star --sandbox=strict --permissions=strict
```

## See also

- [Sandbox](../../concepts/sandbox.md) — full profile schema, custom profiles, AppArmor setup on Ubuntu 24.04+
- [Permissions](../../concepts/permission.md) — the orthogonal layer
