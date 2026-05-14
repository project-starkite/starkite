---
title: "Multi-step automation"
description: "Composing scripts with load(), retry, and concurrency"
weight: 30
---

# Multi-step automation

Real ops automation rarely fits in one script. This guide covers the patterns starkite gives you for composing larger workflows: splitting logic across files with `load()`, retrying flaky steps with the `retry` module, and running independent steps in parallel with `concur`.

!!! info "Coming soon"
    A worked example (multi-host deploy with health checks, rollback, and parallel rolling restarts) is in progress.

## The building blocks

- [`load()`](../fundamentals/modules.md#loading-script-modules) — import symbols from another `.star` file in the same project
- [`retry`](../references/api/retry.md) — fixed-delay and exponential-backoff retry with `RetryResult`
- [`concur`](../references/api/concur.md) — worker-pool execution with timeout, `on_error` policy, and `try_` variants
- [`defer`](../references/api/runtime.md) and [`on_signal`](../references/api/runtime.md) — cleanup hooks that run regardless of how the script ends
