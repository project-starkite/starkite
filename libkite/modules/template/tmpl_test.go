package template

import (
	"testing"
)

func TestGetOrParseTemplateCache(t *testing.T) {
	t1, err := getOrParseTemplate("Hello {{.Name}}", "", "")
	if err != nil {
		t.Fatalf("getOrParseTemplate error: %v", err)
	}
	t2, err := getOrParseTemplate("Hello {{.Name}}", "", "")
	if err != nil {
		t.Fatalf("second getOrParseTemplate error: %v", err)
	}
	if t1 != t2 {
		t.Fatal("expected getOrParseTemplate to return cached *template.Template instance")
	}

	// Different delimiters should produce a distinct cache entry
	t3, err := getOrParseTemplate("Hello {{.Name}}", "<%", "%>")
	if err != nil {
		t.Fatalf("getOrParseTemplate with custom delims error: %v", err)
	}
	if t3 == t1 {
		t.Fatal("expected distinct *template.Template for different delimiters")
	}
}
