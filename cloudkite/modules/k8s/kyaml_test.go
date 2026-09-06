package k8s

import (
	"strings"
	"testing"
)

func TestKYAML_DuplicateKeyRejection(t *testing.T) {
	// 1. YAML with duplicate keys must fail
	dupYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  name: duplicate-cm
data:
  key: value
`
	_, err := parseYAML(dupYAML)
	if err == nil {
		t.Fatalf("expected error on duplicate YAML key, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate key \"name\"") {
		t.Errorf("error %q does not mention duplicate key", err.Error())
	}

	// 2. Valid YAML must succeed
	validYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key1: value1
  key2: value2
`
	d, err := parseYAML(validYAML)
	if err != nil {
		t.Fatalf("parseYAML valid error: %v", err)
	}
	if d == nil {
		t.Fatalf("expected non-nil dict")
	}
}

func TestKYAML_AmbiguousStringQuoting(t *testing.T) {
	// Test strings that in YAML 1.1 coerce to boolean, null, or numbers
	data := map[string]any{
		"boolYes":   "yes",
		"boolNo":    "no",
		"boolOn":    "on",
		"boolOff":   "off",
		"nullStr":   "null",
		"numberStr": "123",
		"plainStr":  "regular-string",
	}

	encoded, err := encodeKYAML(data)
	if err != nil {
		t.Fatalf("encodeKYAML error: %v", err)
	}
	s := string(encoded)

	// Check that ambiguous strings are quoted with double quotes
	if !strings.Contains(s, `"yes"`) {
		t.Errorf("expected quoted \"yes\", got:\n%s", s)
	}
	if !strings.Contains(s, `"no"`) {
		t.Errorf("expected quoted \"no\", got:\n%s", s)
	}
	if !strings.Contains(s, `"on"`) {
		t.Errorf("expected quoted \"on\", got:\n%s", s)
	}
	if !strings.Contains(s, `"off"`) {
		t.Errorf("expected quoted \"off\", got:\n%s", s)
	}
	if !strings.Contains(s, `"null"`) {
		t.Errorf("expected quoted \"null\", got:\n%s", s)
	}
	if !strings.Contains(s, `"123"`) {
		t.Errorf("expected quoted \"123\", got:\n%s", s)
	}
}
