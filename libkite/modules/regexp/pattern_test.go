package regexp

import (
	"testing"
)

func TestCompilePatternCache(t *testing.T) {
	re1, err := compilePattern(`^foo-[0-9]+$`, "")
	if err != nil {
		t.Fatalf("compilePattern error: %v", err)
	}
	re2, err := compilePattern(`^foo-[0-9]+$`, "")
	if err != nil {
		t.Fatalf("second compilePattern error: %v", err)
	}
	if re1 != re2 {
		t.Fatal("expected compilePattern to return cached *regexp.Regexp instance")
	}
}
