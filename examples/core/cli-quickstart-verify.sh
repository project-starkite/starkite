#!/usr/bin/env bash
# cli-quickstart-verify.sh — verifies the CLI commands shown in
# docs/getting-started/quickstarts/cli.md run cleanly against
# examples/core/hello.star and examples/core/hello_test.star.
#
# Captured output of each command is what the quickstart page embeds
# as the expected-output block. Re-run this script after any change to
# the CLI surface or the hello example.
#
# Skipped: `kite repl` and `kite watch` (interactive — require a TTY).
# The CLI quickstart documents both but the verify script cannot
# automate them without complex pty plumbing.

set -euo pipefail

KITE=${KITE:-./bin/kite}
SCRIPT=examples/core/hello.star
TEST_SCRIPT=examples/core/hello_test.star

if [[ ! -x "$KITE" ]]; then
  echo "FAIL: $KITE not found or not executable" >&2
  exit 2
fi

run_step() {
  local label="$1"; shift
  echo "===== $label ====="
  "$@"
  echo
}

run_step "kite version"             "$KITE" version --short
run_step "kite run <script>"        "$KITE" run "$SCRIPT"
run_step "kite <script> (shorthand)" "$KITE" "$SCRIPT"
run_step "kite exec '<code>'"       "$KITE" exec 'print("hello from exec")'
run_step "kite validate <script>"   "$KITE" validate "$SCRIPT"
run_step "kite test <test_script>"  "$KITE" test "$TEST_SCRIPT"

echo "OK — all CLI quickstart commands exit clean."
