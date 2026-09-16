package args_test

import (
	"context"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
)

type mockVarStore struct {
	data map[string]any
}

func (m *mockVarStore) Get(key string) (any, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *mockVarStore) GetWithDefault(key string, def any) any {
	if v, ok := m.data[key]; ok {
		return v
	}
	return def
}

func (m *mockVarStore) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockVarStore) Keys() []string {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

func newRuntimeWithArgs(t *testing.T, rawArgs []string, varStore libkite.VarStore) *libkite.Runtime {
	t.Helper()
	var modCfg *libkite.ModuleConfig
	if varStore != nil {
		modCfg = &libkite.ModuleConfig{VarStore: varStore}
	}
	reg := loader.NewDefaultRegistry(modCfg)
	rt, err := libkite.New(&libkite.Config{
		Registry:   reg,
		ScriptArgs: rawArgs,
		VarStore:   varStore,
	})
	if err != nil {
		t.Fatalf("failed to create runtime with args: %v", err)
	}
	return rt
}

func TestParseAllTypes(t *testing.T) {
	rawArgs := []string{
		"--action", "upgrade",
		"--replicas", "5",
		"--dry-run",
		"--ratio", "0.85",
		"--workers", "node-1",
		"--workers", "node-2",
		"production-cluster",
	}

	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.string("action", default="install", choices=["install", "upgrade"])
args.int("replicas", default=1, min=1, max=10)
args.bool("dry-run", default=False)
args.float("ratio", default=0.5, min=0.0, max=1.0)
args.list("workers", default=[])
args.positional("cluster-name")

p = args.parse()

assert(p.action == "upgrade", "action should be upgrade")
assert(p.replicas == 5, "replicas should be 5")
assert(p.dry_run == True, "dry_run should be True")
assert(p.ratio == 0.85, "ratio should be 0.85")
assert(len(p.workers) == 2, "workers should have 2 items")
assert(p.workers[0] == "node-1", "worker 0")
assert(p.workers[1] == "node-2", "worker 1")
assert(p.cluster_name == "production-cluster", "cluster_name should match positional")

# Test .get() method
assert(p.get("action") == "upgrade", "get('action')")
assert(p.get("key-not-set", "fallback") == "fallback", "get with fallback")
assert(p.get("key-not-set") == None, "get without fallback returns None")

# Test dict indexing
assert(p["action"] == "upgrade", "dict indexing p['action']")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected script failure: %v", err)
	}
}

func TestParseShorthands(t *testing.T) {
	rawArgs := []string{"-a", "setup-keys", "-r", "4", "-d", "-w", "n1,n2", "k8s-local"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.string("action", shorthand="a")
args.int("replicas", shorthand="r")
args.bool("dry-run", shorthand="d")
args.list("workers", shorthand="w")
args.positional("cluster")

p = args.parse()
assert(p.action == "setup-keys")
assert(p.replicas == 4)
assert(p.dry_run == True)
assert(len(p.workers) == 2)
assert(p.cluster == "k8s-local")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseBoolNegation(t *testing.T) {
	rawArgs := []string{"--no-dry-run"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.bool("dry-run", default=True)
p = args.parse()
assert(p.dry_run == False, "dry_run should be negated by --no-dry-run")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseVarFallback(t *testing.T) {
	store := &mockVarStore{
		data: map[string]any{
			"ACTION":   "install",
			"REPLICAS": 7,
			"RATIO":    0.75,
			"DRY_RUN":  true,
			"WORKERS":  "w1,w2,w3",
		},
	}
	// No CLI args passed; everything should resolve from VarStore
	rt := newRuntimeWithArgs(t, []string{}, store)
	script := `
args.string("action", var_fallback="ACTION")
args.int("replicas", var_fallback="REPLICAS")
args.bool("dry-run", var_fallback="DRY_RUN")
args.float("ratio", var_fallback="RATIO")
args.list("workers", var_fallback="WORKERS")

p = args.parse()
assert(p.action == "install", "fallback action")
assert(p.replicas == 7, "fallback replicas")
assert(p.dry_run == True, "fallback dry_run")
assert(p.ratio == 0.75, "fallback ratio")
assert(len(p.workers) == 3, "fallback workers")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseCLIOverridesVarFallback(t *testing.T) {
	store := &mockVarStore{
		data: map[string]any{
			"ACTION": "from-var",
		},
	}
	rawArgs := []string{"--action", "from-cli"}
	rt := newRuntimeWithArgs(t, rawArgs, store)
	script := `
args.string("action", var_fallback="ACTION")
p = args.parse()
assert(p.action == "from-cli", "CLI argument must override VarStore fallback")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseUnknownFlagError(t *testing.T) {
	rawArgs := []string{"--bogus-flag"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.string("action", default="install")
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --bogus-flag") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

func TestParseMissingRequiredFlag(t *testing.T) {
	rt := newRuntimeWithArgs(t, []string{}, nil)
	script := `
args.string("token", required=True)
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --token") {
		t.Fatalf("expected missing required flag error, got %v", err)
	}
}

func TestParseMissingRequiredPositional(t *testing.T) {
	rt := newRuntimeWithArgs(t, []string{}, nil)
	script := `
args.positional("cluster-name", required=True)
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "missing required positional argument: <cluster-name>") {
		t.Fatalf("expected missing required positional error, got %v", err)
	}
}

func TestParseExtraPositionalError(t *testing.T) {
	rawArgs := []string{"expected-pos", "unexpected-pos"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.positional("cluster-name")
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional argument: unexpected-pos") {
		t.Fatalf("expected unexpected positional argument error, got %v", err)
	}
}

func TestParseChoicesValidationFailure(t *testing.T) {
	rawArgs := []string{"--action", "destroy"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.string("action", choices=["install", "upgrade"])
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "invalid value for --action: \"destroy\"") {
		t.Fatalf("expected choice validation error, got %v", err)
	}
}

func TestParseBoundsValidationFailure(t *testing.T) {
	rawArgs := []string{"--replicas", "20"}
	rt := newRuntimeWithArgs(t, rawArgs, nil)
	script := `
args.int("replicas", max=10)
p = args.parse()
`
	err := rt.Execute(context.Background(), script)
	if err == nil || !strings.Contains(err.Error(), "cannot be greater than 10") {
		t.Fatalf("expected bounds validation error, got %v", err)
	}
}
