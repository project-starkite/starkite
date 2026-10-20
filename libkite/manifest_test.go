package libkite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateModuleManifest_ExceedingNetworkAndRead(t *testing.T) {
	manifestYAML := `
name: acme/deployer
version: 0.1.0
sandbox:
  network: host
  paths:
    read:
      - "."
`
	m, err := ParseModuleManifest([]byte(manifestYAML), "mod.yaml")
	if err != nil {
		t.Fatalf("ParseModuleManifest: %v", err)
	}

	policy := &SandboxPolicySummary{
		ProfileName: "default",
		Enabled:     true,
		Network:     "none",
		PathsRead:   nil,
		DisplayPath: "script.star",
	}

	err = ValidateModuleManifest(m, "/workspace/deployer", policy, nil, "/workspace")
	if err == nil {
		t.Fatal("expected ModuleCapabilityError")
	}
	capErr, ok := err.(*ModuleCapabilityError)
	if !ok {
		t.Fatalf("expected *ModuleCapabilityError, got %T: %v", err, err)
	}
	if capErr.Module != "acme/deployer" {
		t.Errorf("Module = %q, want acme/deployer", capErr.Module)
	}
	msg := capErr.Error()
	if !strings.Contains(msg, `network: host (active: none)`) {
		t.Errorf("missing network violation: %q", msg)
	}
	if !strings.Contains(msg, `paths.read: ["."] (active: [])`) {
		t.Errorf("missing paths.read violation: %q", msg)
	}
	if !strings.Contains(msg, "Hint: rerun with required capabilities: kite run --sandbox-net --sandbox-path-read=. script.star") {
		t.Errorf("missing rerun hint: %q", msg)
	}
}

func TestValidateModuleManifest_ExceedingExecAndWrite(t *testing.T) {
	manifestYAML := `
name: acme/builder
version: 0.1.0
sandbox:
  paths:
    write:
      - "./dist"
  exec:
    allow:
      - kubectl
      - helm
`
	m, err := ParseModuleManifest([]byte(manifestYAML), "mod.yaml")
	if err != nil {
		t.Fatalf("ParseModuleManifest: %v", err)
	}

	policy := &SandboxPolicySummary{
		ProfileName: "default",
		Enabled:     true,
		Network:     "none",
		DisplayPath: "main.star",
	}

	// 1. Fails under default
	err = ValidateModuleManifest(m, "/workspace/builder", policy, nil, "/workspace")
	if err == nil {
		t.Fatal("expected ModuleCapabilityError")
	}
	msg := err.Error()
	if !strings.Contains(msg, `--sandbox-path-write=./dist`) {
		t.Errorf("missing write hint: %q", msg)
	}
	if !strings.Contains(msg, `--sandbox-exec-allow=kubectl,helm`) {
		t.Errorf("missing exec hint: %q", msg)
	}

	// 2. Succeeds when capabilities granted
	policyGranted := &SandboxPolicySummary{
		ProfileName: "builder",
		Enabled:     true,
		Network:     "none",
		PathsWrite:  []string{"/workspace"},
		ExecAllow:   []string{"kubectl", "helm"},
	}
	if err := ValidateModuleManifest(m, "/workspace/builder", policyGranted, nil, "/workspace"); err != nil {
		t.Errorf("expected validation success under granted policy: %v", err)
	}
}

func TestValidateModuleManifest_EnvRequirements(t *testing.T) {
	manifestYAML := `
name: acme/cloud
version: 0.1.0
sandbox:
  env:
    inherit: true
    KUBECONFIG:
`
	m, err := ParseModuleManifest([]byte(manifestYAML), "mod.yaml")
	if err != nil {
		t.Fatalf("ParseModuleManifest: %v", err)
	}

	policy := &SandboxPolicySummary{
		ProfileName: "default",
		Enabled:     true,
		EnvInherit:  false,
		EnvAllowed:  map[string]bool{"OTHER": true},
	}

	err = ValidateModuleManifest(m, "/workspace/cloud", policy, nil, "/workspace")
	if err == nil {
		t.Fatal("expected ModuleCapabilityError")
	}
	msg := err.Error()
	if !strings.Contains(msg, "env.inherit: true (active: false)") {
		t.Errorf("missing env.inherit violation: %q", msg)
	}
	if !strings.Contains(msg, `env: ["KUBECONFIG"] (not granted)`) {
		t.Errorf("missing env key violation: %q", msg)
	}
	if !strings.Contains(msg, "--env-inherit") {
		t.Errorf("missing --env-inherit flag in hint: %q", msg)
	}
	if !strings.Contains(msg, "--env KUBECONFIG") {
		t.Errorf("missing --env KUBECONFIG flag in hint: %q", msg)
	}

	// Succeeds when env.inherit is true
	policyInherit := &SandboxPolicySummary{
		ProfileName: "ci",
		Enabled:     true,
		EnvInherit:  true,
	}
	if err := ValidateModuleManifest(m, "/workspace/cloud", policyInherit, nil, "/workspace"); err != nil {
		t.Errorf("expected success with EnvInherit: %v", err)
	}
}

func TestValidateModuleManifest_UnconfinedBypasses(t *testing.T) {
	manifestYAML := `
name: acme/unconfined-module
version: 0.1.0
sandbox:
  base: unconfined
  network: host
  paths:
    read: ["/"]
    write: ["/"]
  exec:
    allow: ["*"]
`
	m, err := ParseModuleManifest([]byte(manifestYAML), "mod.yaml")
	if err != nil {
		t.Fatalf("ParseModuleManifest: %v", err)
	}

	policyUnconfined := &SandboxPolicySummary{
		ProfileName: "unconfined",
		Enabled:     false,
	}
	if err := ValidateModuleManifest(m, "/workspace/unconfined", policyUnconfined, nil, "/workspace"); err != nil {
		t.Errorf("expected unconfined policy to bypass validation: %v", err)
	}
}

func TestRuntime_LoadModuleDirTrustBoundary(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "deployer")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	modYAML := `
name: acme/deployer
version: 0.1.0
sandbox:
  network: host
`
	if err := os.WriteFile(filepath.Join(modDir, "mod.yaml"), []byte(modYAML), 0644); err != nil {
		t.Fatalf("WriteFile mod.yaml: %v", err)
	}
	starContent := `
def deploy():
    return "deployed"
`
	if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte(starContent), 0644); err != nil {
		t.Fatalf("WriteFile main.star: %v", err)
	}

	// 1. Runtime with policy refusing network: host
	rt, err := New(&Config{
		ScriptPath: filepath.Join(dir, "script.star"),
		SandboxPolicy: &SandboxPolicySummary{
			ProfileName: "default",
			Enabled:     true,
			Network:     "none",
			DisplayPath: "script.star",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer rt.Cleanup()

	err = rt.Execute(context.Background(), `load("./deployer", "deployer")`)
	if err == nil {
		t.Fatal("expected module trust boundary error on load()")
	}
	if !IsModuleCapabilityError(err) {
		t.Fatalf("expected IsModuleCapabilityError(err) == true, got %v", err)
	}
	if !strings.Contains(err.Error(), `network: host (active: none)`) {
		t.Errorf("error missing network mismatch: %v", err)
	}

	// 2. Runtime with network: host policy succeeds
	rtGranted, err := New(&Config{
		ScriptPath: filepath.Join(dir, "script.star"),
		SandboxPolicy: &SandboxPolicySummary{
			ProfileName: "net-profile",
			Enabled:     true,
			Network:     "host",
			DisplayPath: "script.star",
		},
	})
	if err != nil {
		t.Fatalf("New granted: %v", err)
	}
	defer rtGranted.Cleanup()

	err = rtGranted.Execute(context.Background(), `
load("./deployer", "deployer")
status = deployer.deploy()
`)
	if err != nil {
		t.Fatalf("expected load to succeed under granted network: %v", err)
	}
}

func TestValidateClosureManifests(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "submod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	modYAML := `
name: local/submod
version: 0.1.0
sandbox:
  paths:
    read:
      - "/var/data"
`
	if err := os.WriteFile(filepath.Join(modDir, "mod.yaml"), []byte(modYAML), 0644); err != nil {
		t.Fatalf("WriteFile mod.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte("x = 1"), 0644); err != nil {
		t.Fatalf("WriteFile main.star: %v", err)
	}

	mainScript := filepath.Join(dir, "script.star")
	if err := os.WriteFile(mainScript, []byte(`load("./submod", "submod")`), 0644); err != nil {
		t.Fatalf("WriteFile script.star: %v", err)
	}

	closure, err := ResolveStaticLoadClosure(mainScript, dir, nil)
	if err != nil {
		t.Fatalf("ResolveStaticLoadClosure: %v", err)
	}

	policy := &SandboxPolicySummary{
		ProfileName: "default",
		Enabled:     true,
		PathsRead:   []string{"/workspace"},
	}

	err = ValidateClosureManifests(closure, policy, nil, dir)
	if err == nil {
		t.Fatal("expected ValidateClosureManifests to fail on ungranted /var/data")
	}
	if !strings.Contains(err.Error(), `paths.read: ["/var/data"]`) {
		t.Errorf("error missing paths.read violation: %v", err)
	}
}
