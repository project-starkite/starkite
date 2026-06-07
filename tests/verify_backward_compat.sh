#!/bin/bash
# Core Functionality Verification Script
#
# Smoke-tests core starkite functionality under the permission model. Gated
# operations are run with an explicit --permissions profile; the default is
# deny-all.
#
# Usage: ./tests/verify_backward_compat.sh [/path/to/kite]

set -e

KITE="${1:-./kite}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

pass_count=0
fail_count=0

pass() {
    echo -e "${GREEN}✓${NC} $1"
    pass_count=$((pass_count + 1))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    fail_count=$((fail_count + 1))
}

info() {
    echo -e "${YELLOW}→${NC} $1"
}

echo "=========================================="
echo "Starkite Core Functionality Tests"
echo "=========================================="
echo "Using: $KITE"
echo ""

# Check kite exists
if [ ! -x "$KITE" ]; then
    echo "Error: $KITE not found or not executable"
    echo "Build with: go build -o kite ./"
    exit 1
fi

# Get version
VERSION=$($KITE version 2>&1 || echo "unknown")
echo "Version: $VERSION"
echo ""

# --------------------------------------------
# Test 1: Basic script execution (no config file)
# --------------------------------------------
info "Test 1: kite run script.star (no config file)"

TMPFILE=$(mktemp /tmp/kite_test_XXXXXX.star)
cat > "$TMPFILE" << 'EOF'
result = 1 + 2 + 3
print("Sum:", result)
assert(result == 6, "math should work")
EOF

if $KITE run "$TMPFILE" 2>&1 | grep -q "Sum: 6"; then
    pass "Basic script execution"
else
    fail "Basic script execution"
fi
rm -f "$TMPFILE"

# --------------------------------------------
# Test 2: Inline code execution
# --------------------------------------------
info "Test 2: kite exec (inline execution)"

if $KITE exec 'print("inline test")' 2>&1 | grep -q "inline test"; then
    pass "Inline execution"
else
    fail "Inline execution"
fi

# --------------------------------------------
# Test 3: Command execution via os.exec
# --------------------------------------------
info "Test 3: os.exec command execution"

if $KITE exec 'print(exec("echo hello"))' --permissions=allow-all 2>&1 | grep -q "hello"; then
    pass "os.exec execution"
else
    fail "os.exec execution"
fi

# --------------------------------------------
# Test 4: File I/O
# --------------------------------------------
info "Test 4: File I/O operations"

TMPFILE=$(mktemp /tmp/kite_test_XXXXXX.txt)
echo "test content" > "$TMPFILE"

if $KITE exec "content = read_text(\"$TMPFILE\"); print(content)" --permissions=allow-fs 2>&1 | grep -q "test content"; then
    pass "File read"
else
    fail "File read"
fi
rm -f "$TMPFILE"

# --------------------------------------------
# Test 5: Test runner
# --------------------------------------------
info "Test 5: kite test (test runner)"

TMPD=$(mktemp -d /tmp/kite_test_XXXXXX)
cat > "$TMPD/sample_test.star" << 'EOF'
def test_addition():
    assert(1 + 1 == 2, "math works")

def test_strings():
    assert("hello".upper() == "HELLO", "string methods work")
EOF

if $KITE test "$TMPD" 2>&1 | grep -q "passed"; then
    pass "Test runner"
else
    fail "Test runner"
fi
rm -rf "$TMPD"

# --------------------------------------------
# Test 6: External module loading
# --------------------------------------------
info "Test 6: External module loading via load()"

TMPD=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$TMPD/modules"

# Create a simple module
cat > "$TMPD/modules/mylib.star" << 'EOF'
def greet(name):
    return "Hello, " + name + "!"
EOF

# Create script that uses the module
cat > "$TMPD/main.star" << 'EOF'
load("./modules/mylib.star", "mylib")
result = mylib.greet("World")
print(result)
assert(result == "Hello, World!", "module should work")
EOF

if $KITE run "$TMPD/main.star" 2>&1 | grep -q "Hello, World"; then
    pass "External module loading"
else
    fail "External module loading"
fi
rm -rf "$TMPD"

# --------------------------------------------
# Test 7: Factory modules (http.config, ssh.config)
# --------------------------------------------
info "Test 7: Factory modules"

# http.config configures the HTTP client
if $KITE exec 'http.config(timeout="5s"); print("http factory ok")' --permissions=allow-net 2>&1 | grep -q "http factory ok"; then
    pass "http.config() factory"
else
    fail "http.config() factory"
fi

# --------------------------------------------
# Test 8: DryRun mode
# --------------------------------------------
info "Test 8: --dry-run mode"

# In dry-run mode, exec should not actually run commands
if $KITE exec 'exec("echo should_not_appear"); print("dry-run ran")' --dry-run --permissions=allow-all 2>&1 | grep -q "dry-run ran"; then
    pass "DryRun mode"
else
    fail "DryRun mode"
fi

# --------------------------------------------
# Test 9: Variables via --var flag
# --------------------------------------------
info "Test 9: --var flag"

if $KITE exec 'print("myvar:", var_str("myvar"))' --var myvar=hello 2>&1 | grep -q "myvar: hello"; then
    pass "--var flag"
else
    fail "--var flag"
fi

# --------------------------------------------
# Test 10: deny-all blocks exec
# --------------------------------------------
info "Test 10: --permissions=deny-all blocks exec"

if $KITE exec 'exec("echo test")' --permissions=deny-all 2>&1 | grep -q "permission denied"; then
    pass "--permissions=deny-all blocks exec"
else
    fail "--permissions=deny-all blocks exec"
fi

# --------------------------------------------
# Test 11: deny-all allows pure-compute modules
# --------------------------------------------
info "Test 11: --permissions=deny-all allows pure-compute modules"

if $KITE exec 'print("hello".upper())' --permissions=deny-all 2>&1 | grep -q "HELLO"; then
    pass "--permissions=deny-all allows strings"
else
    fail "--permissions=deny-all allows strings"
fi

# --------------------------------------------
# Test 12: Default (no --permissions) is deny-all
# --------------------------------------------
info "Test 12: default mode (no flag) blocks exec"

if $KITE exec 'exec("echo nope")' 2>&1 | grep -q "permission denied"; then
    pass "default deny-all blocks exec"
else
    fail "default deny-all blocks exec"
fi

# --------------------------------------------
# Test 13: Unknown profile errors with a helpful message.
# --------------------------------------------
info "Test 13: unknown --permissions value errors out"

if $KITE exec 'print("ok")' --permissions=bogus 2>&1 | grep -q "unknown profile"; then
    pass "unknown profile produces error"
else
    fail "unknown profile produces error"
fi

# --------------------------------------------
# Test 13b: deny-all blocks every gated op
# --------------------------------------------
info "Test 13b: --permissions=deny-all blocks fs reads even under \$CWD"

if $KITE exec 'print(read_text("README.md"))' --permissions=deny-all 2>&1 | grep -q "permission denied"; then
    pass "--permissions=deny-all blocks fs.read"
else
    fail "--permissions=deny-all blocks fs.read"
fi

# --------------------------------------------
# Test 13c: allow-fs allows fs read
# --------------------------------------------
info "Test 13c: --permissions=allow-fs allows fs read"

if $KITE exec 'print(read_text("README.md")[:40])' --permissions=allow-fs 2>&1 | grep -qE "align|starkite|#"; then
    pass "--permissions=allow-fs allows fs read"
else
    fail "--permissions=allow-fs allows fs read"
fi

# --------------------------------------------
# Test 13d: inline rule syntax
# --------------------------------------------
info "Test 13d: inline rules --permissions=allow:os.exec"

if $KITE exec 'print(exec("echo inline"))' --permissions='allow:os.exec' 2>&1 | grep -q "inline"; then
    pass "inline rules work"
else
    fail "inline rules work"
fi

# --------------------------------------------
# Test 13e: a loaded module is bound by the runtime permission, and a denial
# inside it is attributed to the module.
# --------------------------------------------
info "Test 13e: loaded module bound by runtime permission + attributed"

MODDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
printf 'def reach():\n    return exec("uname -s")\n' > "$MODDIR/mod.star"
printf 'load("mod.star", "mod")\nprint(mod.reach())\n' > "$MODDIR/host.star"

# Under allow-fs the module's exec is denied, naming the module.
# load("mod.star") resolves relative to host.star's directory.
if $KITE run "$MODDIR/host.star" --allow-fs 2>&1 | grep -q 'permission denied (module "mod")'; then
    pass "loaded module denial is bound and attributed"
else
    fail "loaded module denial is bound and attributed"
fi

# Under allow-all the same module call succeeds.
if $KITE run "$MODDIR/host.star" --allow-all 2>&1 | grep -qE "Linux|Darwin"; then
    pass "loaded module call succeeds under allow-all"
else
    fail "loaded module call succeeds under allow-all"
fi
rm -rf "$MODDIR"

# --------------------------------------------
# Test 13f: kite run forms — directory module, library error, @namespace/name
# --------------------------------------------
info "Test 13f: kite run directory module / library / @namespace"

RUNDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$RUNDIR/execmod" "$RUNDIR/libmod"
printf 'namespace: t\nname: execmod\n' > "$RUNDIR/execmod/mod.yaml"
printf 'def main():\n    print("ran execmod")\n' > "$RUNDIR/execmod/main.star"
printf 'namespace: t\nname: libmod\n' > "$RUNDIR/libmod/mod.yaml"
printf 'def helper():\n    return 1\n' > "$RUNDIR/libmod/main.star"

# Directory module with main() runs.
if $KITE run "$RUNDIR/execmod" --allow-all 2>&1 | grep -q "ran execmod"; then
    pass "kite run ./dir (executable module)"
else
    fail "kite run ./dir (executable module)"
fi

# Directory module without main() is a library — not runnable.
if $KITE run "$RUNDIR/libmod" --allow-all 2>&1 | grep -q "library"; then
    pass "kite run ./dir (library) errors"
else
    fail "kite run ./dir (library) errors"
fi

# Install as a namespaced module and run via @namespace/name.
$KITE module install "$RUNDIR/execmod" --as t/execmod --force >/dev/null 2>&1 || true
if $KITE run @t/execmod --allow-all 2>&1 | grep -q "ran execmod"; then
    pass "kite run @namespace/name"
else
    fail "kite run @namespace/name"
fi
$KITE module remove t/execmod >/dev/null 2>&1 || true
rm -rf "$RUNDIR"

# --------------------------------------------
# Test 13g: kite run resolves a declared dependency and writes mod.lock
# --------------------------------------------
info "Test 13g: declared dependency resolution + mod.lock"

DEPDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$DEPDIR/leaf" "$DEPDIR/app"
printf 'namespace: dep\nname: leaf\nversion: 0.1.0\n' > "$DEPDIR/leaf/mod.yaml"
printf 'def greet():\n    return "hi from leaf"\n' > "$DEPDIR/leaf/main.star"
printf 'namespace: dep\nname: app\nversion: 0.1.0\ndependencies:\n  dep/leaf: %s\n' "$DEPDIR/leaf" > "$DEPDIR/app/mod.yaml"
printf 'load("dep/leaf", "leaf")\n\ndef main():\n    print(leaf.greet())\n' > "$DEPDIR/app/main.star"

if $KITE run "$DEPDIR/app" --allow-all 2>&1 | grep -q "hi from leaf"; then
    pass "kite run resolves and loads a declared dependency"
else
    fail "kite run resolves and loads a declared dependency"
fi

if [ -f "$DEPDIR/app/mod.lock" ] && grep -q "dep/leaf" "$DEPDIR/app/mod.lock"; then
    pass "mod.lock written with resolved dependency"
else
    fail "mod.lock written with resolved dependency"
fi
rm -rf "$DEPDIR"

# --------------------------------------------
# Test 13h: kite module verify detects intact vs tampered modules
# --------------------------------------------
info "Test 13h: kite module verify"

VERDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$VERDIR/src"
printf 'namespace: ver\nname: tool\nversion: 0.1.0\n' > "$VERDIR/src/mod.yaml"
printf 'def main():\n    pass\n' > "$VERDIR/src/main.star"
$KITE module install "$VERDIR/src" --as ver/tool --force >/dev/null 2>&1 || true

if $KITE module verify ver/tool 2>&1 | grep -q "ok"; then
    pass "verify passes for an intact module"
else
    fail "verify passes for an intact module"
fi

# Tamper with the installed copy and confirm verify fails (non-zero exit).
INSTALLED=$($KITE module info ver/tool 2>/dev/null | awk '/^Path:/ {print $2}')
if [ -n "$INSTALLED" ]; then
    printf 'def main():\n    print("tampered")\n' > "$INSTALLED/main.star"
fi
if $KITE module verify ver/tool >/dev/null 2>&1; then
    fail "verify fails for a tampered module"
else
    pass "verify fails for a tampered module"
fi
$KITE module remove ver/tool >/dev/null 2>&1 || true
rm -rf "$VERDIR"

# --------------------------------------------
# Test 13i: loose `kite run file.star` resolves load()s from cache only
# --------------------------------------------
info "Test 13i: loose-file dependency resolution (cache only)"

LOOSEDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$LOOSEDIR/src"
printf 'namespace: loose\nname: lib\nversion: 0.1.0\n' > "$LOOSEDIR/src/mod.yaml"
printf 'def hello():\n    return "from loose lib"\n' > "$LOOSEDIR/src/main.star"
$KITE module install "$LOOSEDIR/src" --as loose/lib --force >/dev/null 2>&1 || true

printf 'load("loose/lib", "lib")\n\ndef main():\n    print(lib.hello())\n' > "$LOOSEDIR/script.star"
if $KITE run "$LOOSEDIR/script.star" --allow-all 2>&1 | grep -q "from loose lib"; then
    pass "loose run loads an installed module and writes mod.lock"
else
    fail "loose run loads an installed module and writes mod.lock"
fi
if [ -f "$LOOSEDIR/mod.lock" ] && grep -q "loose/lib" "$LOOSEDIR/mod.lock"; then
    pass "mod.lock written beside the loose script"
else
    fail "mod.lock written beside the loose script"
fi

# An uninstalled dependency is an error (no fetch in the loose path).
printf 'load("loose/missing", "missing")\n\ndef main():\n    pass\n' > "$LOOSEDIR/bad.star"
if $KITE run "$LOOSEDIR/bad.star" --allow-all >/dev/null 2>&1; then
    fail "loose run errors on an uninstalled dependency"
else
    pass "loose run errors on an uninstalled dependency"
fi
$KITE module remove loose/lib >/dev/null 2>&1 || true
rm -rf "$LOOSEDIR"

# --------------------------------------------
# Test 13j: kite init scaffolds a runnable module
# --------------------------------------------
info "Test 13j: kite init scaffolds a runnable module"

INITDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
$KITE init "$INITDIR/widget" >/dev/null 2>&1
SCAFFOLD_OK=true
for f in mod.yaml main.star mod.lock README.md; do
    [ -f "$INITDIR/widget/$f" ] || SCAFFOLD_OK=false
done
if [ "$SCAFFOLD_OK" = true ]; then
    pass "init creates main.star, mod.yaml, mod.lock, README.md"
else
    fail "init creates main.star, mod.yaml, mod.lock, README.md"
fi
if $KITE run "$INITDIR/widget" --allow-all 2>&1 | grep -q "hello from starkite"; then
    pass "the scaffolded module runs"
else
    fail "the scaffolded module runs"
fi
rm -rf "$INITDIR"

# --------------------------------------------
# Test 13k: the full kite module subcommand surface
#   install / list / info / verify / update / remove
# Runs against an isolated module cache (own $HOME) so the real cache is
# untouched.
# --------------------------------------------
info "Test 13k: kite module install/list/info/verify/update/remove"

MODHOME=$(mktemp -d /tmp/kite_test_XXXXXX)
MODSRC=$(mktemp -d /tmp/kite_test_XXXXXX)
printf 'namespace: acme\nname: tool\nversion: 0.1.0\ndescription: a tool\n' > "$MODSRC/mod.yaml"
printf 'def greet():\n    return "v1"\n' > "$MODSRC/main.star"
MK(){ HOME="$MODHOME" $KITE "$@"; }

# install
if MK module install "$MODSRC" --as acme/tool 2>&1 | grep -q "Installed acme/tool"; then
    pass "module install"
else
    fail "module install"
fi

# list
if MK module list 2>&1 | grep -q "acme/tool"; then
    pass "module list shows the module"
else
    fail "module list shows the module"
fi

# info
if MK module info acme/tool 2>&1 | grep -q "Revision:"; then
    pass "module info reports a revision"
else
    fail "module info reports a revision"
fi

# verify (intact)
if MK module verify acme/tool 2>&1 | grep -q "ok"; then
    pass "module verify passes for an intact module"
else
    fail "module verify passes for an intact module"
fi

# update after source change → a second revision
printf 'def greet():\n    return "v2"\n' > "$MODSRC/main.star"
if MK module update acme/tool 2>&1 | grep -q "Updated acme/tool"; then
    pass "module update"
else
    fail "module update"
fi
REVCOUNT=$(MK module list 2>/dev/null | grep -c "acme/tool")
if [ "$REVCOUNT" -eq 2 ]; then
    pass "update adds a second revision (list shows both)"
else
    fail "update adds a second revision (list shows both, got $REVCOUNT)"
fi
# info and verify stay usable with multiple revisions
if MK module info acme/tool >/dev/null 2>&1 && MK module verify acme/tool 2>&1 | grep -q "ok"; then
    pass "info and verify tolerate multiple revisions"
else
    fail "info and verify tolerate multiple revisions"
fi

# remove (all revisions)
MK module remove acme/tool >/dev/null 2>&1
if MK module list 2>&1 | grep -q "No modules installed"; then
    pass "module remove deletes all revisions"
else
    fail "module remove deletes all revisions"
fi

rm -rf "$MODHOME" "$MODSRC"

# --------------------------------------------
# Test 13l: git install, @rev run selection, invalid-module rejection
# --------------------------------------------
info "Test 13l: git install / @rev selection / invalid modules"

GHOME=$(mktemp -d /tmp/kite_test_XXXXXX)
GREPO=$(mktemp -d /tmp/kite_test_XXXXXX)/repo
GK(){ HOME="$GHOME" $KITE "$@"; }

if command -v git >/dev/null 2>&1; then
    mkdir -p "$GREPO"
    printf 'namespace: acme\nname: leaf\nversion: 0.1.0\n' > "$GREPO/mod.yaml"
    printf 'def main():\n    print("v1")\n' > "$GREPO/main.star"
    ( cd "$GREPO" && git init -q && git add -A && \
      GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@e GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@e \
      git commit -qm init ) >/dev/null 2>&1
    COMMIT=$(cd "$GREPO" && git rev-parse --short HEAD)

    if GK module install "file://$GREPO" --as acme/leaf 2>&1 | grep -q "Installed acme/leaf"; then
        pass "git install from file:// repo"
    else
        fail "git install from file:// repo"
    fi
    # The installed revision is the commit, and .git is not carried in.
    if GK module list 2>/dev/null | grep -q "$COMMIT" && \
       ! ls -a "$GHOME/.starkite/modules/acme/"*/ 2>/dev/null | grep -q '\.git'; then
        pass "git install records commit rev and strips .git"
    else
        fail "git install records commit rev and strips .git"
    fi

    # Update to a second revision, then select by @rev and newest.
    printf 'def main():\n    print("v2")\n' > "$GREPO/main.star"
    ( cd "$GREPO" && git add -A && \
      GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@e GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@e \
      git commit -qm v2 ) >/dev/null 2>&1
    GK module update acme/leaf >/dev/null 2>&1
    if [ "$(GK run @acme/leaf --allow-all 2>&1)" = "v2" ]; then
        pass "bare @namespace/name runs the newest revision"
    else
        fail "bare @namespace/name runs the newest revision"
    fi
    if [ "$(GK run "@acme/leaf@$COMMIT" --allow-all 2>&1)" = "v1" ]; then
        pass "@namespace/name@rev pins a specific revision"
    else
        fail "@namespace/name@rev pins a specific revision"
    fi
    if GK run "@acme/leaf@nosuchrev" --allow-all >/dev/null 2>&1; then
        fail "unknown @rev errors"
    else
        pass "unknown @rev errors"
    fi
else
    info "  (git not available — skipping git install / @rev tests)"
fi

# Invalid module installs are rejected (no git needed).
BADSRC=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$BADSRC/nomanifest"; printf 'def main(): pass\n' > "$BADSRC/nomanifest/main.star"
mkdir -p "$BADSRC/noentry"; printf 'namespace: x\nname: noentry\n' > "$BADSRC/noentry/mod.yaml"
mkdir -p "$BADSRC/noname"; printf 'version: 0.1.0\n' > "$BADSRC/noname/mod.yaml"; printf 'def main(): pass\n' > "$BADSRC/noname/main.star"
INVALID_OK=true
for d in nomanifest noentry noname; do
    if GK module install "$BADSRC/$d" --as "x/$d" >/dev/null 2>&1; then
        INVALID_OK=false
    fi
done
if [ "$INVALID_OK" = true ]; then
    pass "invalid modules (no manifest / no entry / no name) are rejected"
else
    fail "invalid modules (no manifest / no entry / no name) are rejected"
fi

rm -rf "$GHOME" "$GREPO" "$BADSRC"

# --------------------------------------------
# Test 13m: mod.lock pins a dependency revision (reproducibility)
# A run stays on the locked revision even when a newer one is cached.
# --------------------------------------------
info "Test 13m: mod.lock pins the dependency revision"

LHOME=$(mktemp -d /tmp/kite_test_XXXXXX)
LWORK=$(mktemp -d /tmp/kite_test_XXXXXX)
LK(){ HOME="$LHOME" $KITE "$@"; }
mkdir -p "$LWORK/leaf" "$LWORK/app"
printf 'namespace: acme\nname: leaf\nversion: 0.1.0\n' > "$LWORK/leaf/mod.yaml"
printf 'def greet():\n    return "v1"\n' > "$LWORK/leaf/main.star"
printf 'namespace: acme\nname: app\nversion: 0.1.0\ndependencies:\n  acme/leaf: %s\n' "$LWORK/leaf" > "$LWORK/app/mod.yaml"
printf 'load("acme/leaf", "leaf")\n\ndef main():\n    print(leaf.greet())\n' > "$LWORK/app/main.star"

LK run --permissions=allow-all "$LWORK/app" >/dev/null 2>&1   # first run writes mod.lock pinning v1
# Publish a newer leaf revision into the cache.
printf 'def greet():\n    return "v2-NEWER"\n' > "$LWORK/leaf/main.star"
LK module install "$LWORK/leaf" --as acme/leaf >/dev/null 2>&1
OUT=$(LK run --permissions=allow-all "$LWORK/app" 2>&1)
if [ "$OUT" = "v1" ]; then
    pass "load() honors mod.lock (stays on pinned rev despite newer cached rev)"
else
    fail "load() honors mod.lock (got '$OUT', want v1)"
fi
rm -rf "$LHOME" "$LWORK"

# --------------------------------------------
# Test 14: Built-in modules work
# --------------------------------------------
info "Test 14: Built-in modules"

TESTS_PASSED=true

# json
if ! $KITE exec 'print(json.encode({"a": 1}))' 2>&1 | grep -q '"a"'; then
    TESTS_PASSED=false
fi

# yaml
if ! $KITE exec 'print(yaml.encode({"b": 2}))' 2>&1 | grep -q "b:"; then
    TESTS_PASSED=false
fi

# time
if ! $KITE exec 'print(time.now().year)' 2>&1 | grep -qE "20[0-9][0-9]"; then
    TESTS_PASSED=false
fi

# uuid
if ! $KITE exec 'u = uuid.v4(); print(len(u))' 2>&1 | grep -q "36"; then
    TESTS_PASSED=false
fi

# hash
if ! $KITE exec 'h = hash.text("test").sha256(); print(len(h))' 2>&1 | grep -q "64"; then
    TESTS_PASSED=false
fi

if [ "$TESTS_PASSED" = true ]; then
    pass "Built-in modules"
else
    fail "Built-in modules"
fi

# --------------------------------------------
# Test 15: Environment access
# --------------------------------------------
info "Test 15: Environment access"

if $KITE exec 'h = env("HOME"); print("home:", h)' --permissions=allow-fs 2>&1 | grep -q "home: /"; then
    pass "Environment access"
else
    fail "Environment access"
fi

# --------------------------------------------
# Summary
# --------------------------------------------
echo ""
echo "=========================================="
echo "Results: $pass_count passed, $fail_count failed"
echo "=========================================="

if [ $fail_count -gt 0 ]; then
    exit 1
fi
exit 0
