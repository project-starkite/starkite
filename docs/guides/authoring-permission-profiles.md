---
title: "Authoring permission profiles"
description: "Designing a security.yaml for your team"
weight: 20
---

# Authoring permission profiles

A permission profile is a named bundle of rules. Profiles live in `~/.starkite/security.yaml`, in a file referenced via `--permissions=path/to/file.yaml#name`, or inline in a script's frontmatter. Start from `deny-all` and add only the operations the script requires.

## Skeleton

```yaml
# ~/.starkite/security.yaml
permissions:
  ci-deploy:
    rules:
      - allow: fs.read($CWD/**)
      - allow: os.exec(kubectl,helm)
      - allow: k8s.read; k8s.write
      - allow: http.client(api.production.example.com)
```

## See also

- [Permissions](../concepts/permission.md) — full rule grammar and built-in profiles
- [CI integration](ci-integration.md) — wiring a profile into a CI job
