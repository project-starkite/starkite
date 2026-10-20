package libkite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// SandboxPolicySummary summarizes the operator's active sandbox profile and
// CLI capability grants so the runtime and module loader can validate
// third-party module manifests (mod.yaml) without importing libkite/sandbox.
type SandboxPolicySummary struct {
	ProfileName string          // Active profile name (e.g., "default", "deny-all", "ci-builder")
	Enabled     bool            // True when sandbox confinement is enabled; false when unconfined
	Network     string          // Active network mode ("none", "loopback", "host")
	PathsRead   []string        // Active paths.read grants
	PathsWrite  []string        // Active paths.write grants
	ExecAllow   []string        // Active exec.allow grants
	ExecDeny    []string        // Active exec.deny rules
	CallsAllow  []string        // Active calls.allow rules
	CallsDeny   []string        // Active calls.deny rules
	EnvInherit  bool            // True when wholesale host env inheritance is active
	EnvAllowed  map[string]bool // Explicit/forwarded env keys permitted by operator
	DisplayPath string          // Script/module target path as invoked on CLI for rerun hints
}

// ModuleCapabilityError is returned when a loaded or executed module's mod.yaml
// declares sandbox capabilities that exceed the operator's active profile and CLI flags.
type ModuleCapabilityError struct {
	Module     string   // Module qualified name (e.g., "acme/deployer")
	Profile    string   // Active operator profile name (e.g., "default")
	Violations []string // Formatted capability mismatch items
	Flags      []string // Synthesized CLI flags required to satisfy the module
	ScriptPath string   // Target script or module reference for the rerun hint
}

// Error implements the error interface.
func (e *ModuleCapabilityError) Error() string {
	prof := e.Profile
	if prof == "" {
		prof = "default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "module %q requires capabilities exceeding active profile %q:", e.Module, prof)
	for _, v := range e.Violations {
		fmt.Fprintf(&b, "\n  - %s", v)
	}
	if len(e.Flags) > 0 {
		target := e.ScriptPath
		if target == "" {
			target = "script.star"
		}
		fmt.Fprintf(&b, "\n  Hint: rerun with required capabilities: kite run %s %s",
			strings.Join(e.Flags, " "), target)
	}
	return b.String()
}

// IsModuleCapabilityError reports whether err is or wraps a *ModuleCapabilityError.
func IsModuleCapabilityError(err error) bool {
	if err == nil {
		return false
	}
	var target *ModuleCapabilityError
	return errors.As(err, &target)
}

// NetworkModeAndAllow extracts the normalized network mode ("none", "loopback", "host")
// and any endpoint allowlist from a ManifestSandboxSpec.
func (s *ManifestSandboxSpec) NetworkModeAndAllow() (string, []string) {
	if s == nil {
		return "", nil
	}
	var mode string
	var allow []string
	if s.Network != nil {
		switch v := s.Network.(type) {
		case string:
			mode = strings.TrimSpace(v)
		case map[string]any:
			if m, ok := v["mode"].(string); ok {
				mode = strings.TrimSpace(m)
			}
			if rawAllow, ok := v["allow"].([]any); ok {
				for _, item := range rawAllow {
					if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
						allow = append(allow, strings.TrimSpace(str))
					}
				}
			}
			if mode == "" && len(allow) > 0 {
				mode = "host"
			}
		}
	}
	if mode == "" && s.Base == "sandbox-net" {
		mode = "host"
	}
	return mode, allow
}

// parseManifestEnv extracts inherit and declared env variable keys from ManifestSandboxSpec.Env.
func (s *ManifestSandboxSpec) parseManifestEnv() (inherit bool, keys []string) {
	if s == nil || s.Env.Kind != yaml.MappingNode {
		return false, nil
	}
	for i := 0; i < len(s.Env.Content); i += 2 {
		kNode := s.Env.Content[i]
		vNode := s.Env.Content[i+1]
		key := strings.TrimSpace(kNode.Value)
		if key == "inherit" {
			_ = vNode.Decode(&inherit)
			continue
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	return inherit, keys
}

func networkRank(mode string) int {
	switch mode {
	case "host":
		return 2
	case "loopback":
		return 1
	default:
		return 0
	}
}

func formatStringSliceYAML(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// ValidateModuleManifest checks whether a module's declared sandbox requirements
// in mod.yaml are satisfied by the operator's active SandboxPolicySummary and/or
// PermissionChecker. Returns a *ModuleCapabilityError if the module demands
// capabilities exceeding operator grants.
func ValidateModuleManifest(
	m *ModuleManifest,
	moduleDir string,
	policy *SandboxPolicySummary,
	checker *PermissionChecker,
	workDir string,
) error {
	if m == nil || m.Sandbox == nil {
		return nil
	}
	// If neither policy nor permission checker is active (trusted embedded mode), allow.
	if policy == nil && checker == nil {
		return nil
	}
	// If operator explicitly disabled sandboxing (--sandbox-enabled=false / unconfined), allow.
	if policy != nil && !policy.Enabled {
		return nil
	}
	if checker != nil && policy == nil && checker.default_ == DefaultAllow && len(checker.deny) == 0 {
		return nil
	}

	sb := m.Sandbox
	cwd := workDir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	home, _ := os.UserHomeDir()

	profileName := "default"
	activeNet := "none"
	var activeRead, activeWrite, activeExecAllow []string
	var displayPath string

	if policy != nil {
		if policy.ProfileName != "" {
			profileName = policy.ProfileName
		}
		if policy.Network != "" {
			activeNet = policy.Network
		}
		activeRead = policy.PathsRead
		activeWrite = policy.PathsWrite
		activeExecAllow = policy.ExecAllow
		displayPath = policy.DisplayPath
	} else if checker != nil {
		if checker.allows("http", "client", "get", "https://example.com") {
			activeNet = "host"
		} else if checker.allows("http", "server", "serve", "127.0.0.1:80") {
			activeNet = "loopback"
		}
	}

	var violations []string
	var flags []string
	addFlag := func(f string) {
		if !slices.Contains(flags, f) {
			flags = append(flags, f)
		}
	}

	// 1. Check base: unconfined escalation
	if sb.Base == "unconfined" {
		violations = append(violations, fmt.Sprintf("base: unconfined (active: %s)", profileName))
		addFlag("--sandbox-enabled=false")
	}

	// 2. Check Network requirement
	reqNet, _ := sb.NetworkModeAndAllow()
	if reqNet != "" && networkRank(reqNet) > networkRank(activeNet) {
		violations = append(violations, fmt.Sprintf("network: %s (active: %s)", reqNet, activeNet))
		addFlag("--sandbox-net")
	}

	// 3. Check Paths.Read requirements (including implied "." from base rungs)
	reqReads := slices.Clone(sb.Paths.Read)
	if (sb.Base == "sandbox-path" || sb.Base == "sandbox-exec" || sb.Base == "sandbox-net") && !slices.Contains(reqReads, ".") {
		reqReads = append(reqReads, ".")
	}
	var missingReads []string
	for _, rp := range reqReads {
		if !isPathGranted(rp, false, cwd, home, moduleDir, policy, checker) {
			missingReads = append(missingReads, rp)
		}
	}
	if len(missingReads) > 0 {
		violations = append(violations, fmt.Sprintf("paths.read: %s (active: %s)",
			formatStringSliceYAML(missingReads), formatStringSliceYAML(activeRead)))
		for _, mr := range missingReads {
			addFlag("--sandbox-path-read=" + mr)
		}
	}

	// 4. Check Paths.Write requirements (including implied "." from base rungs)
	reqWrites := slices.Clone(sb.Paths.Write)
	if (sb.Base == "sandbox-path" || sb.Base == "sandbox-exec" || sb.Base == "sandbox-net") && !slices.Contains(reqWrites, ".") {
		reqWrites = append(reqWrites, ".")
	}
	var missingWrites []string
	for _, wp := range reqWrites {
		if !isPathGranted(wp, true, cwd, home, moduleDir, policy, checker) {
			missingWrites = append(missingWrites, wp)
		}
	}
	if len(missingWrites) > 0 {
		violations = append(violations, fmt.Sprintf("paths.write: %s (active: %s)",
			formatStringSliceYAML(missingWrites), formatStringSliceYAML(activeWrite)))
		for _, mw := range missingWrites {
			addFlag("--sandbox-path-write=" + mw)
		}
	}

	// 5. Check Exec.Allow and AllowedExecPaths requirements
	reqExec := slices.Clone(sb.Exec.Allow)
	for _, aep := range sb.AllowedExecPaths {
		if !slices.Contains(reqExec, aep) {
			reqExec = append(reqExec, aep)
		}
	}
	if (sb.Base == "sandbox-exec" || sb.Base == "sandbox-net") && !slices.Contains(reqExec, "*") {
		reqExec = append(reqExec, "*")
	}
	var missingExec []string
	for _, bin := range reqExec {
		if !isExecGranted(bin, policy, checker) {
			missingExec = append(missingExec, bin)
		}
	}
	if len(missingExec) > 0 {
		violations = append(violations, fmt.Sprintf("exec.allow: %s (active: %s)",
			formatStringSliceYAML(missingExec), formatStringSliceYAML(activeExecAllow)))
		if slices.Contains(missingExec, "*") {
			addFlag("--sandbox-exec-allow=*")
		} else {
			addFlag("--sandbox-exec-allow=" + strings.Join(missingExec, ","))
		}
	}

	// 6. Check Calls.Allow requirements
	var missingCalls []string
	for _, pattern := range sb.Calls.Allow {
		if !isCallPatternGranted(pattern, cwd, home, policy, checker) {
			missingCalls = append(missingCalls, pattern)
		}
	}
	if len(missingCalls) > 0 {
		activeCalls := []string(nil)
		if policy != nil {
			activeCalls = policy.CallsAllow
		}
		violations = append(violations, fmt.Sprintf("calls.allow: %s (active: %s)",
			formatStringSliceYAML(missingCalls), formatStringSliceYAML(activeCalls)))
		for _, mc := range missingCalls {
			addFlag("--sandbox-call-allow=" + mc)
		}
	}

	// 7. Check Env requirements (inherit: true and required bare keys)
	reqEnvInherit, reqEnvKeys := sb.parseManifestEnv()
	if reqEnvInherit && (policy == nil || !policy.EnvInherit) {
		violations = append(violations, "env.inherit: true (active: false)")
		addFlag("--env-inherit")
	}
	if policy != nil && !policy.EnvInherit && len(reqEnvKeys) > 0 {
		var missingEnv []string
		for _, k := range reqEnvKeys {
			if !policy.EnvAllowed[k] {
				missingEnv = append(missingEnv, k)
			}
		}
		if len(missingEnv) > 0 {
			violations = append(violations, fmt.Sprintf("env: %s (not granted)", formatStringSliceYAML(missingEnv)))
			for _, k := range missingEnv {
				addFlag("--env " + k)
			}
		}
	}

	if len(violations) == 0 {
		return nil
	}

	return &ModuleCapabilityError{
		Module:     m.QualifiedName(),
		Profile:    profileName,
		Violations: violations,
		Flags:      flags,
		ScriptPath: displayPath,
	}
}

// ValidateClosureManifests validates all module manifests discovered in a
// StaticLoadClosure against the operator's active sandbox policy and permissions.
func ValidateClosureManifests(
	closure *StaticLoadClosure,
	policy *SandboxPolicySummary,
	checker *PermissionChecker,
	workDir string,
) error {
	if closure == nil {
		return nil
	}
	dirs := make([]string, 0, len(closure.ModuleDirs))
	for dir := range closure.ModuleDirs {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)

	for _, dir := range dirs {
		manifestPath := filepath.Join(dir, ManifestFile)
		data, ok := closure.Files[manifestPath]
		if !ok {
			var err error
			data, err = os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
		}
		m, err := ParseModuleManifest(data, manifestPath)
		if err != nil {
			return err
		}
		if err := ValidateModuleManifest(m, dir, policy, checker, workDir); err != nil {
			return err
		}
	}
	return nil
}

func resolveManifestPath(rawPath, cwd, home, moduleDir string) string {
	p := strings.TrimSpace(rawPath)
	if p == "." || p == "./" || p == "$CWD" {
		return cwd
	}
	if after, ok := strings.CutPrefix(p, "./"); ok {
		return filepath.Join(cwd, after)
	}
	if after, ok := strings.CutPrefix(p, "$CWD/"); ok {
		return filepath.Join(cwd, after)
	}
	if home != "" {
		if p == "$HOME" || p == "~" {
			return home
		}
		if after, ok := strings.CutPrefix(p, "$HOME/"); ok {
			return filepath.Join(home, after)
		}
		if after, ok := strings.CutPrefix(p, "~/"); ok {
			return filepath.Join(home, after)
		}
	}
	if !filepath.IsAbs(p) && moduleDir != "" {
		return filepath.Join(cwd, p)
	}
	return p
}

func isPathGranted(rawPath string, write bool, cwd, home, moduleDir string, policy *SandboxPolicySummary, checker *PermissionChecker) bool {
	target := resolveManifestPath(rawPath, cwd, home, moduleDir)
	canonTarget := CanonicalizePath(target)
	probeFile := filepath.Join(canonTarget, ".probe")

	if checker != nil {
		if write {
			if checker.allows("fs", "write", "write_file", probeFile) ||
				checker.allows("fs", "write", "write_file", canonTarget) {
				return true
			}
		} else {
			if checker.allows("fs", "read", "read_file", probeFile) ||
				checker.allows("fs", "read", "read_file", canonTarget) {
				return true
			}
		}
	}

	if policy != nil {
		for _, wp := range policy.PathsWrite {
			if wp == rawPath || wp == "*" {
				return true
			}
			canonW := CanonicalizePath(resolveManifestPath(wp, cwd, home, ""))
			if canonTarget == canonW || strings.HasPrefix(canonTarget, canonW+string(filepath.Separator)) {
				return true
			}
		}
		if !write {
			for _, rp := range policy.PathsRead {
				if rp == rawPath || rp == "*" {
					return true
				}
				canonR := CanonicalizePath(resolveManifestPath(rp, cwd, home, ""))
				if canonTarget == canonR || strings.HasPrefix(canonTarget, canonR+string(filepath.Separator)) {
					return true
				}
			}
		}
	}
	return false
}

func isExecGranted(bin string, policy *SandboxPolicySummary, checker *PermissionChecker) bool {
	bin = strings.TrimSpace(bin)
	if checker != nil {
		if bin == "*" {
			if checker.allows("os", "exec", "exec", "*") && checker.allows("os", "shell", "shell", "*") {
				return true
			}
		} else if checker.allows("os", "exec", "exec", bin) {
			return true
		}
	}
	if policy != nil {
		if slices.Contains(policy.ExecDeny, bin) || slices.Contains(policy.ExecDeny, "*") {
			return false
		}
		if slices.Contains(policy.ExecAllow, "*") || slices.Contains(policy.ExecAllow, bin) {
			return true
		}
		base := filepath.Base(bin)
		return slices.Contains(policy.ExecAllow, base)
	}
	return false
}

func isCallPatternGranted(pattern, cwd, home string, policy *SandboxPolicySummary, checker *PermissionChecker) bool {
	if policy != nil && slices.Contains(policy.CallsAllow, pattern) {
		return true
	}
	if checker == nil {
		return false
	}
	rule, err := ParseRule(pattern)
	if err != nil {
		return false
	}
	mod := rule.Module
	if mod == "*" {
		mod = "os"
	}
	cat := rule.Category
	if cat == "*" {
		cat = "exec"
	}
	fn := "call"
	if len(rule.Functions) > 0 {
		fn = rule.Functions[0]
	}
	res := expandPathVariables(rule.Resource, cwd, home)
	if res == "" {
		res = "*"
	}
	return checker.allows(mod, cat, fn, res)
}
