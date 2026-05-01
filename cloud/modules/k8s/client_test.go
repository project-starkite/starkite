package k8s

import (
	"sort"
	"testing"

	"go.starlark.net/starlark"
)

// TestClientTryAttr verifies that Attr("try_X") returns a non-nil *starlark.Builtin
// for every method in allMethods.
func TestClientTryAttr(t *testing.T) {
	c := &K8sClient{}

	for name := range allMethods {
		tryName := "try_" + name
		v, err := c.Attr(tryName)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", tryName, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", tryName)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", tryName, v)
		}
	}
}

// TestClientTryAttrNames verifies that AttrNames() has 2*len(allMethods) entries,
// is sorted, and every method has both its name and try_ variant.
func TestClientTryAttrNames(t *testing.T) {
	c := &K8sClient{}
	names := c.AttrNames()

	expectedCount := 2 * len(allMethods)
	if len(names) != expectedCount {
		t.Fatalf("AttrNames() len = %d, want %d", len(names), expectedCount)
	}

	// Verify sorted
	if !sort.StringsAreSorted(names) {
		t.Fatal("AttrNames() is not sorted")
	}

	// Build lookup set
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	for name := range allMethods {
		if !nameSet[name] {
			t.Errorf("AttrNames() missing %q", name)
		}
		tryName := "try_" + name
		if !nameSet[tryName] {
			t.Errorf("AttrNames() missing %q", tryName)
		}
	}
}

// TestClientDirectAttrUnchanged verifies that direct Attr lookups (without try_ prefix)
// still return *starlark.Builtin for all methods.
func TestClientDirectAttrUnchanged(t *testing.T) {
	c := &K8sClient{}

	for name := range allMethods {
		v, err := c.Attr(name)
		if err != nil {
			t.Errorf("Attr(%q) error: %v", name, err)
			continue
		}
		if v == nil {
			t.Errorf("Attr(%q) returned nil", name)
			continue
		}
		if _, ok := v.(*starlark.Builtin); !ok {
			t.Errorf("Attr(%q) returned %T, want *starlark.Builtin", name, v)
		}
	}
}

// TestClientTryNonexistent verifies that Attr("try_bogus") returns (nil, nil).
func TestClientTryNonexistent(t *testing.T) {
	c := &K8sClient{}

	v, err := c.Attr("try_bogus")
	if err != nil {
		t.Fatalf("Attr(\"try_bogus\") error: %v", err)
	}
	if v != nil {
		t.Fatalf("Attr(\"try_bogus\") = %v, want nil", v)
	}
}
