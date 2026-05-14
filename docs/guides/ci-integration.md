---
title: "CI integration"
description: "Running starkite in GitHub Actions, GitLab CI, and other runners"
weight: 10
---

# CI integration

Starkite is a single binary with no runtime dependencies, which makes it well-suited to CI: drop the binary on the runner, run the script, exit.

!!! info "Coming soon"
    Detailed walkthroughs for GitHub Actions, GitLab CI, and self-hosted runners are in progress. For now, the building blocks below cover the typical setup.

## The shape

A CI job that runs a starkite script looks like this:

1. Install or download the `kite` binary on the runner.
2. Optionally pin a permission profile so the script can't escape its intended blast radius.
3. Run `kite <script>.star` with whatever variables the job needs (`--var key=value`, `--var-file=…`).
4. Let the exit code propagate.

## Pinning the binary

In GitHub Actions, download a release asset from [GitHub Releases](https://github.com/project-starkite/starkite/releases) into `$RUNNER_TEMP` and add it to `PATH`. Pin the version — `kite-linux-amd64@v0.1.0` not `@latest` — so CI runs are reproducible.

## Pinning permissions

Pass `--permissions=<profile>` (or set `STARKITE_PERMISSIONS=<profile>` for shebang scripts) to bound the script's privilege surface. For a CI deploy that only needs `kubectl apply`-equivalent operations, a profile that allows `k8s.write` against a specific namespace and nothing else gives you both clarity and an audit trail.

See [Authoring permission profiles](authoring-permission-profiles.md) for the rule grammar.
