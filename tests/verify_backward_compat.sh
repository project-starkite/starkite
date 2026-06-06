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

TMPDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
cat > "$TMPDIR/sample_test.star" << 'EOF'
def test_addition():
    assert(1 + 1 == 2, "math works")

def test_strings():
    assert("hello".upper() == "HELLO", "string methods work")
EOF

if $KITE test "$TMPDIR" 2>&1 | grep -q "passed"; then
    pass "Test runner"
else
    fail "Test runner"
fi
rm -rf "$TMPDIR"

# --------------------------------------------
# Test 6: External module loading
# --------------------------------------------
info "Test 6: External module loading via load()"

TMPDIR=$(mktemp -d /tmp/kite_test_XXXXXX)
mkdir -p "$TMPDIR/modules"

# Create a simple module
cat > "$TMPDIR/modules/mylib.star" << 'EOF'
def greet(name):
    return "Hello, " + name + "!"
EOF

# Create script that uses the module
cat > "$TMPDIR/main.star" << 'EOF'
load("./modules/mylib.star", "mylib")
result = mylib.greet("World")
print(result)
assert(result == "Hello, World!", "module should work")
EOF

if $KITE run "$TMPDIR/main.star" 2>&1 | grep -q "Hello, World"; then
    pass "External module loading"
else
    fail "External module loading"
fi
rm -rf "$TMPDIR"

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
