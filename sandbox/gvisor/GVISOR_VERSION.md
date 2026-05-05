# gVisor Pin

This module pins `gvisor.dev/gvisor` to a specific commit on the upstream
**`go` branch**. That branch is required, not a preference — see "Why the
go branch" below.

## Current pin

| Field | Value |
|---|---|
| Commit | `18babf4ea276` |
| Pseudo-version | `v0.0.0-20260505181943-18babf4ea276` |
| Pinned date | 2026-05-05 |

The pin lives in `sandbox/gvisor/go.mod`. After updating, run `go mod tidy`
on a Linux host (the dep graph includes Linux-only packages); rsync `go.mod`
and `go.sum` back to the working copy.

## Why the `go` branch (not `master`, not `@latest`)

`gvisor.dev/gvisor`'s primary build is Bazel. The `master` branch contains
sources that only compile under Bazel's custom Go rules — it has missing
`*_go_proto` packages (Bazel generates them), package-name collisions in
`pkg/{safecopy, refs, safemem, eventfd}` that only resolve via Bazel
package_group rules, and Bazel-specific assembly directives in
`pkg/sync/runtime_spinning_amd64.s`.

The `go` branch is a **synthetic, auto-generated** branch that contains
only the Go sources gVisor's CI produces from Bazel. The branch README
says: _"This branch is a synthetic branch, containing only Go sources,
that is compatible with standard Go tools."_ It's the one you can `go get`.

**The Go module proxy serves `@latest` from `master`.** If anyone runs
`go get gvisor.dev/gvisor@latest` in this project, the build will break
in confusing ways. Always pin to a commit on the `go` branch.

## Resolving a new pin

```bash
# On Linux (the gVisor dep graph includes Linux-only packages, so tidy
# must run on Linux):
cd sandbox/gvisor
go get gvisor.dev/gvisor@go            # @go is the branch ref; resolves to HEAD
go mod tidy
```

`go get gvisor.dev/gvisor@go` produces a pseudo-version of the form
`v0.0.0-<timestamp>-<short-hash>`, where the hash is the latest commit
on the `go` branch. Verify the resulting line in `go.mod` matches the
expected pattern before committing.

Alternative: pin to a specific commit by SHA:

```bash
go get gvisor.dev/gvisor@<full-or-short-sha-on-go-branch>
```

## Imported packages

This module currently anchors the dep with blank imports of:

- `gvisor.dev/gvisor/runsc/cmd` (multi-personality dispatch in 4b.2)
- `gvisor.dev/gvisor/runsc/config` (sandbox config in 4b.3)
- `gvisor.dev/gvisor/runsc/container` (container lifecycle in 4b.3)
- `gvisor.dev/gvisor/runsc/specutils` (OCI spec helpers in 4b.3)

When a future change adds an import beyond this set, update both the
import list above **and** the runsc/* sanity-check on upgrade.

## Upgrade procedure

1. **Pick a new go-branch commit.** Generally take the latest:
   `cd sandbox/gvisor && go get gvisor.dev/gvisor@go`. For a specific commit,
   use the full SHA. Verify the resulting pseudo-version in `go.mod`.
2. **Run `go mod tidy` on Linux.** The dep graph includes
   `vishvananda/netlink`, `creack/pty`, etc. — Linux-only packages.
3. **rsync `go.mod` + `go.sum` back to your working copy.**
4. **Build allkite on Linux.** Confirm the binary still compiles. Note
   any size delta beyond the baseline (+31 MB at initial pin).
5. **Run integration tests.** Once 4b.4 lands, `go test ./tests/sandbox/...`
   on Linux is the upgrade gate. Failures here indicate a runsc CLI or
   API change.
6. **Update this file** with the new commit, date, and any notes about
   breaking changes you had to accommodate.

## Cadence

- **Quarterly** baseline upgrade.
- **Out-of-band** on gVisor CVE publication (track
  https://github.com/google/gvisor/security/advisories).
- **Avoid upgrading reactively** if integration tests pass — go-branch
  HEAD churn is constant; a stable older pin is preferable to chasing.

## Binary size impact

| Build | Size |
|---|---|
| `kite` (allkite) on Linux, baseline (pre-pin) | 90 MB |
| `kite` (allkite) on Linux, with gVisor anchor | 121 MB |
| **Delta on Linux** | **+31 MB** |
| `kite` on macOS, with gVisor anchor | 92 MB (unchanged) |

macOS is unchanged because all gVisor imports live in
`sandbox/gvisor/runner_linux.go` (build tag `//go:build linux`). The
gVisor module's transitive graph is fully tree-shaken on darwin.

If a future change makes gVisor imports cross the build-tag boundary
(e.g., a non-Linux file imports a gVisor package that builds on darwin),
darwin binary size will jump too. Keep all gVisor imports under
`//go:build linux`.
