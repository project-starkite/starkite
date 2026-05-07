# gVisor Pin

`gvisor.dev/gvisor` is pinned to a commit on the upstream **`go` branch** —
the synthetic Go-tool-compatible branch. The default `master` branch is
Bazel-only and won't build with `go build`.

> **Do not use `@latest`.** The proxy resolves it to `master`, which breaks
> the build. Always pin to a commit on the `go` branch.

## Current pin

| Field | Value |
|---|---|
| Commit | `18babf4ea276` |
| Pseudo-version | `v0.0.0-20260505181943-18babf4ea276` |
| Pinned date | 2026-05-05 |

The pin lives in `sandbox/gvisor/go.mod`.

## Upgrade procedure

Run on Linux — the dep graph includes Linux-only packages, so `go mod tidy`
can't complete on macOS:

```bash
cd sandbox/gvisor
go get gvisor.dev/gvisor@go     # @go = the branch ref; resolves to its HEAD
go mod tidy
```

To pin a specific commit instead, pass its SHA: `go get gvisor.dev/gvisor@<sha>`.

Then:

1. rsync `go.mod` + `go.sum` back to the working copy
2. `go build ./...` in `sandbox/gvisor` and `allkite`
3. `go test ./...` in `sandbox/gvisor`
4. `STARKITE_SANDBOX_INTEGRATION=1 GOWORK=off go test ./...` in `tests/sandbox`
5. update the "Current pin" table above
