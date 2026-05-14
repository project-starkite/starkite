---
title: "CI integration"
description: "Running starkite in GitHub Actions, GitLab CI, and other runners"
weight: 10
---

# CI integration

Starkite is a single binary with no runtime dependencies, suited to CI runners: install the binary, run the script, exit with its status code.

## CI job structure

1. Install or download the `kite` binary on the runner.
2. Pin a permission profile to bound the script's privilege surface.
3. Run `kite <script>.star` with required variables (`--var key=value`, `--var-file=…`).
4. Propagate the exit code.

## Pinning the binary

In GitHub Actions, download a release asset from [GitHub Releases](https://github.com/project-starkite/starkite/releases) into `$RUNNER_TEMP` and add it to `PATH`. Pin the version (`kite-linux-amd64@v0.1.0`, not `@latest`) so CI runs stay reproducible.

## Pinning permissions

Pass `--permissions=<profile>` (or set `STARKITE_PERMISSIONS=<profile>` for shebang scripts) to bound the script's privilege surface. For a CI deploy that needs only `kubectl apply`-equivalent operations, a profile allowing `k8s.write` against a specific namespace and nothing else produces both clarity and an audit trail.

See [Authoring permission profiles](authoring-permission-profiles.md) for the rule grammar.
