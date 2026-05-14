---
title: "Authoring permission profiles"
description: "Designing a security.yaml for your team"
weight: 20
---

# Authoring permission profiles

A permission profile is a named bundle of rules. Profiles live in `~/.starkite/security.yaml`, in a file you point `--permissions=path/to/file.yaml#name` at, or inline in a script's frontmatter. Authoring a good profile is mostly about deciding what the *minimum* set of allowed operations is — start from `deny-all` and add only what the script demands.

!!! info "Coming soon"
    A full walkthrough with worked examples is in progress.

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

- [Permissions](../fundamentals/security/permissions.md) — full rule grammar and built-in profiles
- [CI integration](ci-integration.md) — wiring a profile into a CI job
