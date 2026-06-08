package libkite_test

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestRuntimeCleanups_LIFO_Idempotent(t *testing.T) {
	rt, err := libkite.NewTrusted(nil)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}

	var order []int
	rt.AddCleanup(func() { order = append(order, 1) })
	rt.AddCleanup(func() { order = append(order, 2) })

	rt.Close()
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Errorf("cleanups ran %v, want [2 1] (LIFO)", order)
	}

	rt.Close() // second close must not re-run cleanups
	if len(order) != 2 {
		t.Errorf("cleanups re-ran on second Close: %v", order)
	}
}

func TestRegisterCleanup_ViaThread(t *testing.T) {
	rt, err := libkite.NewTrusted(nil)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	th := rt.NewThread("cleanup-test")

	ran := false
	libkite.RegisterCleanup(th, func() { ran = true })

	rt.Close()
	if !ran {
		t.Error("RegisterCleanup'd function did not run at Close")
	}
}
