package version

import (
	"strings"
	"testing"
)

func TestEditionDefaults(t *testing.T) {
	origEdition := Edition
	defer func() { Edition = origEdition }()

	Edition = ""
	if EditionName() != "base" {
		t.Fatalf("EditionName() = %q, want 'base'", EditionName())
	}
	if !IsBaseEdition() {
		t.Fatal("IsBaseEdition() should be true when Edition is empty")
	}

	Edition = "base"
	if EditionName() != "base" {
		t.Fatalf("EditionName() = %q, want 'base'", EditionName())
	}
	if !IsBaseEdition() {
		t.Fatal("IsBaseEdition() should be true when Edition is 'base'")
	}
}

func TestEditionCustom(t *testing.T) {
	origEdition := Edition
	defer func() { Edition = origEdition }()

	Edition = "cloud"
	if EditionName() != "cloud" {
		t.Fatalf("EditionName() = %q, want 'cloud'", EditionName())
	}
	if IsBaseEdition() {
		t.Fatal("IsBaseEdition() should be false for 'cloud'")
	}
	s := String()
	if !strings.Contains(s, "(cloud)") {
		t.Fatalf("String() = %q, want it to contain '(cloud)'", s)
	}

	Edition = "ai"
	if EditionName() != "ai" {
		t.Fatalf("EditionName() = %q, want 'ai'", EditionName())
	}
	if IsBaseEdition() {
		t.Fatal("IsBaseEdition() should be false for 'ai'")
	}
	s = String()
	if !strings.Contains(s, "(ai)") {
		t.Fatalf("String() = %q, want it to contain '(ai)'", s)
	}
}

func TestEditionAll(t *testing.T) {
	origEdition := Edition
	defer func() { Edition = origEdition }()

	Edition = "all"
	if EditionName() != "all" {
		t.Fatalf("EditionName() = %q, want 'all'", EditionName())
	}
	if IsBaseEdition() {
		t.Fatal("IsBaseEdition() should be false for 'all'")
	}
	s := String()
	if strings.Contains(s, "(all)") {
		t.Fatalf("String() for 'all' edition should not contain '(all)': %q", s)
	}
	if !strings.Contains(s, Version) || !strings.Contains(s, GitCommit) {
		t.Fatalf("String() = %q, should contain version %q and commit %q", s, Version, GitCommit)
	}
}
