package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunTestsSuccess(t *testing.T) {
	dir := t.TempDir()
	content := `def test_simple_pass():
    assert(10 + 20 == 30, "arithmetic check")
`
	if err := os.WriteFile(filepath.Join(dir, "pass_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error on passing tests, got: %v", err)
	}
}

func TestRunTestsFailure(t *testing.T) {
	dir := t.TempDir()
	content := `def test_simple_fail():
    assert(1 == 2, "intentional failure")
`
	if err := os.WriteFile(filepath.Join(dir, "fail_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTests(nil, []string{dir})
	if err == nil {
		t.Fatal("expected error on failing tests, got nil")
	}
}

func TestRunTestsParallel(t *testing.T) {
	dir := t.TempDir()
	content1 := `def test_part1():
    assert(True, "part 1")
`
	content2 := `def test_part2():
    assert(True, "part 2")
`
	if err := os.WriteFile(filepath.Join(dir, "part1_test.star"), []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "part2_test.star"), []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	oldParallel := testParallel
	testParallel = 2
	defer func() { testParallel = oldParallel }()

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error in parallel test runner, got: %v", err)
	}
}

func TestRunTestsFilter(t *testing.T) {
	dir := t.TempDir()
	content := `def test_alpha():
    assert(True, "alpha")
def test_beta():
    assert(False, "beta should not run")
`
	if err := os.WriteFile(filepath.Join(dir, "filter_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldFilter := testFilter
	testFilter = "alpha"
	defer func() { testFilter = oldFilter }()

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error when filtering for passing test, got: %v", err)
	}
}

func TestRunTestsNoFiles(t *testing.T) {
	dir := t.TempDir()
	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error for empty directory, got: %v", err)
	}
}
