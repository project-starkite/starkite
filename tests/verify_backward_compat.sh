#!/bin/bash
# Backward Compatibility Verification Script
#
# This script verifies that all existing starkite functionality works unchanged
# after the starbase permission system integration.
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
    ((pass_count++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    ((fail_count++))
}

info() {
    echo -e "${YELLOW}→${NC} $1"
}

echo "=========================================="
echo "Starbase Backward Compatibility Tests"
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

if $KITE exec 'r = exec("echo hello"); print(r.stdout)' 2>&1 | grep -q "hello"; then
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

if $KITE exec "content = read_text(\"$TMPFILE\"); print(content)" 2>&1 | grep -q "test content"; then
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
if $KITE exec 'http.config(timeout=5000); print("http factory ok")' 2>&1 | grep -q "http factory ok"; then
    pass "http.config() factory"
else
    fail "http.config() factory"
fi

# --------------------------------------------
# Test 8: DryRun mode
# --------------------------------------------
info "Test 8: --dry-run mode"

# In dry-run mode, exec should not actually run commands
if $KITE exec 'r = exec("echo should_not_appear"); print("dry-run:", r.ok)' --dry-run 2>&1 | grep -q "dry-run: True"; then
    pass "DryRun mode"
else
    fail "DryRun mode"
fi

# --------------------------------------------
# Test 9: Variables via --var flag
# --------------------------------------------
info "Test 9: --var flag"

if $KITE exec 'v = var("myvar"); print("myvar:", v)' --var myvar=hello 2>&1 | grep -q "myvar: hello"; then
    pass "--var flag"
else
    fail "--var flag"
fi

# --------------------------------------------
# Test 10: Strict profile blocks exec
# --------------------------------------------
info "Test 10: --permissions=strict blocks exec"

if $KITE exec 'exec("echo test")' --permissions=strict 2>&1 | grep -q "permission denied"; then
    pass "--permissions=strict blocks exec"
else
    fail "--permissions=strict blocks exec"
fi

# --------------------------------------------
# Test 11: Strict profile allows safe modules
# --------------------------------------------
info "Test 11: --permissions=strict allows safe modules"

if $KITE exec 'print(strings.upper("hello"))' --permissions=strict 2>&1 | grep -q "HELLO"; then
    pass "--permissions=strict allows strings"
else
    fail "--permissions=strict allows strings"
fi

# --------------------------------------------
# Test 12: Default (no --permissions) allows everything
# --------------------------------------------
info "Test 12: default trust mode allows everything"

if $KITE exec 'r = exec("echo trusted"); print(r.stdout)' 2>&1 | grep -q "trusted"; then
    pass "default trust mode allows exec"
else
    fail "default trust mode allows exec"
fi

# --------------------------------------------
# Test 13: Unknown profile errors with helpful message (Phase 2b: was a
# stderr warning + fallback in Phase 2a; now a hard error from LoadProfile).
# --------------------------------------------
info "Test 13: unknown --permissions value errors out"

if $KITE exec 'print("ok")' --permissions=bogus 2>&1 | grep -q "unknown profile"; then
    pass "unknown profile produces error"
else
    fail "unknown profile produces error"
fi

# --------------------------------------------
# Test 13b: deny-all blocks every gated op (Phase 2b)
# --------------------------------------------
info "Test 13b: --permissions=deny-all blocks fs reads even under \$CWD"

if $KITE exec 'print(read_text("README.md"))' --permissions=deny-all 2>&1 | grep -q "permission denied"; then
    pass "--permissions=deny-all blocks fs.read"
else
    fail "--permissions=deny-all blocks fs.read"
fi

# --------------------------------------------
# Test 13c: strict allows fs read under \$CWD (Phase 2b semantic change)
# --------------------------------------------
info "Test 13c: --permissions=strict allows fs read under \$CWD"

if $KITE exec 'print(read_text("README.md")[:20])' --permissions=strict 2>&1 | grep -q "starkite\|#"; then
    pass "--permissions=strict allows \$CWD fs read"
else
    fail "--permissions=strict allows \$CWD fs read"
fi

# --------------------------------------------
# Test 13d: inline rule syntax (Phase 2b)
# --------------------------------------------
info "Test 13d: inline rules --permissions=allow:os.exec"

if $KITE exec 'print(exec("echo inline"))' --permissions='allow:os.exec' 2>&1 | grep -q "inline"; then
    pass "inline rules work"
else
    fail "inline rules work"
fi

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
if ! $KITE exec 'h = hash.sha256("test"); print(len(h))' 2>&1 | grep -q "64"; then
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

if $KITE exec 'h = env("HOME"); print("home:", h)' 2>&1 | grep -q "home: /"; then
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
