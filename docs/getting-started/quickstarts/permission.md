---
title: "Run with restricted permissions"
description: "Bound a script's privileged operations with --permissions"
weight: 6
---

# Run with restricted permissions

By default `kite` runs in **trust mode** — scripts can do anything the host user can do. The `--permissions=<profile>` flag flips the default to deny-all and requires explicit allow rules.

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

- [Permissions](../../fundamentals/security/permissions.md) — full rule grammar, every category, $CWD/$HOME expansion, all four resolution paths
- [Authoring permission profiles](../../guides/authoring-permission-profiles.md) — designing a `security.yaml` for a team
