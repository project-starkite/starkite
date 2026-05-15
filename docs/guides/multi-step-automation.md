---
title: "Compose multi-step automation"
description: "Composing scripts with load(), retry, and concurrency"
weight: 30
---

# Compose multi-step automation

Larger workflows compose across files and across modules. The primitives:

- [`load()`](../concepts/modules.md#loading-script-modules) — import symbols from another `.star` file in the same project.
- [`retry`](../references/api/retry.md) — fixed-delay and exponential-backoff retry with `RetryResult`.
- [`concur`](../references/api/concur.md) — worker-pool execution with timeout, `on_error` policy, and `try_` variants.
- [`defer`](../references/api/runtime.md) and [`on_signal`](../references/api/runtime.md) — cleanup hooks that run regardless of how the script ends.
