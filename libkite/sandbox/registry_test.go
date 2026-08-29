package sandbox

import (
	"runtime"
	"slices"
	"testing"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()

	d1 := &mockDriver{name: "mock1", available: true}
	d2 := &mockDriver{name: "mock2", available: false}

	reg.Register(d1)
	reg.Register(d2)

	got1, err := reg.Get("mock1")
	if err != nil {
		t.Fatalf("Get(mock1) error: %v", err)
	}
	if got1.Name() != "mock1" {
		t.Errorf("got %q, want 'mock1'", got1.Name())
	}

	got2, err := reg.Get("mock2")
	if err != nil {
		t.Fatalf("Get(mock2) error: %v", err)
	}
	if got2.Available() != false {
		t.Errorf("expected mock2 to be unavailable")
	}

	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Errorf("expected error for unregistered driver, got nil")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDriver{name: "beta", available: true})
	reg.Register(&mockDriver{name: "alpha", available: true})
	reg.Register(&mockDriver{name: "gamma", available: true})

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}
	if list[0] != "alpha" || list[1] != "beta" || list[2] != "gamma" {
		t.Errorf("List() returned %v, want [alpha beta gamma]", list)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDriver{name: "temp", available: true})

	if _, err := reg.Get("temp"); err != nil {
		t.Fatalf("expected temp to be registered")
	}

	reg.Unregister("temp")
	if _, err := reg.Get("temp"); err == nil {
		t.Errorf("expected temp to be unregistered")
	}
}

func TestRegistry_Resolve(t *testing.T) {
	reg := NewRegistry()

	landlock := &mockDriver{name: DriverLandlock, available: true}
	seatbelt := &mockDriver{name: DriverSeatbelt, available: true}
	podman := &mockDriver{name: DriverPodman, available: true}

	reg.Register(landlock)
	reg.Register(seatbelt)
	reg.Register(podman)

	// Explicit name resolution
	d, err := reg.Resolve(DriverPodman)
	if err != nil {
		t.Fatalf("Resolve(podman) error: %v", err)
	}
	if d.Name() != DriverPodman {
		t.Errorf("Resolve(podman) = %s, want %s", d.Name(), DriverPodman)
	}

	// Auto/default resolution
	autoD, err := reg.Resolve(DriverDefault)
	if err != nil {
		t.Fatalf("Resolve(default) error: %v", err)
	}

	switch runtime.GOOS {
	case "linux":
		if autoD.Name() != DriverLandlock {
			t.Errorf("Resolve(default) on Linux = %s, want %s", autoD.Name(), DriverLandlock)
		}
	case "darwin":
		if autoD.Name() != DriverSeatbelt {
			t.Errorf("Resolve(default) on macOS = %s, want %s", autoD.Name(), DriverSeatbelt)
		}
	}
}

func TestRegistry_Resolve_UnavailableErrors(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDriver{name: "unavail", available: false})

	_, err := reg.Resolve("unavail")
	if err == nil {
		t.Errorf("expected error when resolving unavailable driver, got nil")
	}
}

func TestGlobalRegistry_Functions(t *testing.T) {
	d := &mockDriver{name: "global-mock", available: true}
	Register(d)
	defer Unregister("global-mock")

	got, err := Get("global-mock")
	if err != nil {
		t.Fatalf("Get(global-mock) error: %v", err)
	}
	if got.Name() != "global-mock" {
		t.Errorf("got %q, want 'global-mock'", got.Name())
	}

	found := slices.Contains(List(), "global-mock")
	if !found {
		t.Errorf("expected global-mock in List()")
	}
}
