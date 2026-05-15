---
title: "Run with restricted permissions"
description: "Bound a script's privileged operations with --permissions"
weight: 6
---

# Run with restricted permissions

By default `kite` runs in **trust mode** — a script can perform any operation the host user can perform. For scripts that haven't been audited locally, the `--permissions=<profile>` flag activates the permission engine: every privileged module call (filesystem write, network connect, command exec, Kubernetes apply, LLM generate) is matched against a rule set before it runs. Operations not explicitly allowed are denied.

Three built-in profiles cover the common cases. Custom profiles — authored in `~/.starkite/security.yaml`, a file path, or passed inline on the command line — cover everything else.

## With a built-in profile

```bash
kite run examples/core/hello.star --permissions=strict
```

`strict` permits filesystem reads/writes inside `$CWD` and nothing else. The hello script's `os.exec("uname -s")` is not allowed under `strict`, so the run fails with a permission error:

```
Error: permission denied: os.exec is not allowed
```

The three built-ins are:

| Profile | Behavior |
|---|---|
| `allow-all` | every operation allowed (the default trust-mode behavior) |
| `strict` | filesystem reads/writes inside `$CWD` only; everything else denied |
| `deny-all` | every privileged operation denied |

## With a profile from `~/.starkite/security.yaml`

```yaml
# ~/.starkite/security.yaml
permissions:
  ci-deploy:
    rules:
      - allow: fs.read($CWD/**)
      - allow: os.exec(kubectl,helm)
      - allow: k8s.read; k8s.write
```

```bash
kite run deploy.star --permissions=ci-deploy
```

## With inline rules

```bash
kite run deploy.star --permissions='allow:fs.read($CWD/**); allow:os.exec(make)'
```

## With script frontmatter

```python
# permissions: ci-deploy
#!/usr/bin/env kite

# … script body …
```

The CLI flag wins over frontmatter when both are present.

## See also

- [Permissions](../../concepts/permission.md) — full rule grammar, every category, $CWD/$HOME expansion, all four resolution paths
- [Authoring permission profiles](../../guides/authoring-permission-profiles.md) — designing a `security.yaml` for a team
