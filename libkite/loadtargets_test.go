package libkite

import (
	"reflect"
	"testing"
)

func TestLoadTargets(t *testing.T) {
	src := []byte(`load("acme/leaf", "leaf")
load("./helpers", "helpers")
load("json", "json")

def main():
    leaf.run()
`)
	got, err := LoadTargets("deploy.star", src)
	if err != nil {
		t.Fatalf("LoadTargets: %v", err)
	}
	want := []string{"acme/leaf", "./helpers", "json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadTargets = %v, want %v", got, want)
	}
}

func TestLoadTargetsNone(t *testing.T) {
	got, err := LoadTargets("x.star", []byte("def main():\n    pass\n"))
	if err != nil {
		t.Fatalf("LoadTargets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no targets, got %v", got)
	}
}
