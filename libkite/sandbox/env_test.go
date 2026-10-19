package sandbox

import (
	"strings"
	"testing"
)

func TestSanitizedBaselineEnv(t *testing.T) {
	mockHostEnv := []string{
		"AWS_SECRET_ACCESS_KEY=super-secret",
		"GITHUB_TOKEN=ghp_secrettoken123",
		"SSH_AUTH_SOCK=/private/tmp/com.apple.launchd.xxx/Listeners",
		"KUBECONFIG=/Users/alice/.kube/config",
		"USER=alice",
		"HOME=/Users/alice",
		"PATH=/custom/bin:/usr/bin",
		"LANG=en_US.UTF-8",
		"TERM=vt100",
		"SYSTEMROOT=C:\\Windows",
		"COMSPEC=C:\\Windows\\system32\\cmd.exe",
		"WINDIR=C:\\Windows",
		"PATHEXT=.COM;.EXE;.BAT",
	}

	baseline := SanitizedBaselineEnv(mockHostEnv)

	// Sensitive keys must be stripped
	deniedKeys := []string{
		"AWS_SECRET_ACCESS_KEY",
		"GITHUB_TOKEN",
		"SSH_AUTH_SOCK",
		"KUBECONFIG",
		"USER",
	}
	for _, k := range deniedKeys {
		if _, exists := baseline[k]; exists {
			t.Errorf("expected %q to be stripped from baseline environment, but was present", k)
		}
	}

	// Platform runtime essentials must be retained
	if baseline["PATH"] != "/custom/bin:/usr/bin" {
		t.Errorf("expected PATH to be preserved, got %q", baseline["PATH"])
	}
	if baseline["HOME"] != "/tmp" && baseline["HOME"] != baseline["TEMP"] {
		t.Errorf("expected HOME to be /tmp or TEMP, got %q", baseline["HOME"])
	}
	if baseline["LANG"] != "en_US.UTF-8" {
		t.Errorf("expected LANG to be preserved, got %q", baseline["LANG"])
	}
	if baseline["TERM"] != "vt100" {
		t.Errorf("expected TERM to be preserved, got %q", baseline["TERM"])
	}

	// Windows runtime essentials should be preserved when present
	if baseline["SYSTEMROOT"] != `C:\Windows` {
		t.Errorf("expected SYSTEMROOT to be preserved, got %q", baseline["SYSTEMROOT"])
	}
	if baseline["COMSPEC"] != `C:\Windows\system32\cmd.exe` {
		t.Errorf("expected COMSPEC to be preserved, got %q", baseline["COMSPEC"])
	}
	if baseline["WINDIR"] != `C:\Windows` {
		t.Errorf("expected WINDIR to be preserved, got %q", baseline["WINDIR"])
	}
	if baseline["PATHEXT"] != ".COM;.EXE;.BAT" {
		t.Errorf("expected PATHEXT to be preserved, got %q", baseline["PATHEXT"])
	}
}

func TestBareKeyHostForwarding(t *testing.T) {
	hostEnv := []string{
		"KUBECONFIG=/var/run/secrets/kubeconfig",
		"ALLOWED_VAR=present_value",
	}

	p := &Profile{
		Env: EnvPolicySpec{
			Vars: map[string]*string{
				"KUBECONFIG": nil, // bare key: present on host
				"UNSET_VAR":  nil, // bare key: absent from host
			},
		},
	}

	res, err := p.SynthesizeEnvMap(hostEnv, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res["KUBECONFIG"] != "/var/run/secrets/kubeconfig" {
		t.Errorf("expected KUBECONFIG to be forwarded from host, got %q", res["KUBECONFIG"])
	}
	val, exists := res["UNSET_VAR"]
	if !exists {
		t.Errorf("expected UNSET_VAR to be present in synthesized env")
	}
	if val != "" {
		t.Errorf("expected unset bare key to be empty string, got %q", val)
	}
}

func TestDynamicHostVariableExpansion(t *testing.T) {
	hostEnv := []string{
		"PROD_DB=postgres://cluster.internal/prod",
		"PORT=5432",
		"SECRET_TOKEN=xyz123",
	}

	dbUrl := "$PROD_DB"
	connStr := "pgsql://${PROD_DB}:${PORT}?ssl=true"
	p := &Profile{
		Env: EnvPolicySpec{
			Vars: map[string]*string{
				"DATABASE_URL": &dbUrl,
				"CONN":         &connStr,
			},
		},
	}

	res, err := p.SynthesizeEnvMap(hostEnv, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res["DATABASE_URL"] != "postgres://cluster.internal/prod" {
		t.Errorf("expected DATABASE_URL expansion, got %q", res["DATABASE_URL"])
	}
	if res["CONN"] != "pgsql://postgres://cluster.internal/prod:5432?ssl=true" {
		t.Errorf("expected CONN expansion, got %q", res["CONN"])
	}

	// PROD_DB and PORT must NOT be directly leaked into the sandbox table
	if _, leaked := res["PROD_DB"]; leaked {
		t.Errorf("PROD_DB was leaked into synthesized environment")
	}
	if _, leaked := res["PORT"]; leaked {
		t.Errorf("PORT was leaked into synthesized environment")
	}
	if _, leaked := res["SECRET_TOKEN"]; leaked {
		t.Errorf("SECRET_TOKEN was leaked into synthesized environment")
	}
}

func TestCLIEnvFlagParsing(t *testing.T) {
	hostMap := map[string]string{
		"HOST_VAR": "host_value",
	}

	tests := []struct {
		name      string
		flags     []string
		want      map[string]string
		wantError bool
	}{
		{
			name:  "explicit assignment",
			flags: []string{"FOO=bar", "PORT=8080"},
			want:  map[string]string{"FOO": "bar", "PORT": "8080"},
		},
		{
			name:  "multiple equals signs in value",
			flags: []string{"EXPR=a=b=c"},
			want:  map[string]string{"EXPR": "a=b=c"},
		},
		{
			name:  "value with comma",
			flags: []string{"QUERY=SELECT id, name FROM users"},
			want:  map[string]string{"QUERY": "SELECT id, name FROM users"},
		},
		{
			name:  "bare key present on host",
			flags: []string{"HOST_VAR"},
			want:  map[string]string{"HOST_VAR": "host_value"},
		},
		{
			name:  "bare key absent from host",
			flags: []string{"MISSING_VAR"},
			want:  map[string]string{"MISSING_VAR": ""},
		},
		{
			name:      "empty key",
			flags:     []string{"=value"},
			wantError: true,
		},
		{
			name:      "whitespace in key",
			flags:     []string{"FOO BAR=val"},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCLIEnvFlags(tc.flags, hostMap)
			if tc.wantError {
				if err == nil {
					t.Errorf("expected error for flags %v, got nil", tc.flags)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, wantVal := range tc.want {
				if got[k] != wantVal {
					t.Errorf("key %q: got %q, want %q", k, got[k], wantVal)
				}
			}
		})
	}
}

func Test3TierPrecedence(t *testing.T) {
	hostEnv := []string{
		"FOO=from_host",
		"BAR=from_host",
		"BAZ=from_host",
	}

	yamlFoo := "from_yaml"
	yamlBar := "from_yaml"
	p := &Profile{
		Env: EnvPolicySpec{
			Vars: map[string]*string{
				"FOO": &yamlFoo,
				"BAR": &yamlBar,
			},
		},
	}

	cliFlags := []string{"FOO=from_cli"}

	res, err := p.SynthesizeEnvMap(hostEnv, cliFlags, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tier 3 overrides Tier 2
	if res["FOO"] != "from_cli" {
		t.Errorf("FOO: want %q (from CLI), got %q", "from_cli", res["FOO"])
	}
	// Tier 2 overrides Tier 1
	if res["BAR"] != "from_yaml" {
		t.Errorf("BAR: want %q (from YAML), got %q", "from_yaml", res["BAR"])
	}
	// BAZ should be scrubbed (neither in YAML nor in CLI)
	if _, exists := res["BAZ"]; exists {
		t.Errorf("BAZ should have been scrubbed by zero-trust baseline")
	}
	// Baseline HOME must exist
	if res["HOME"] == "" {
		t.Errorf("HOME must be present in baseline")
	}
}

func TestEnvironmentInheritance(t *testing.T) {
	hostEnv := []string{
		"HOST_A=alpha",
		"HOST_B=beta",
		"OVERRIDE_ME=old",
	}

	// Test inherit via profile
	overrideVal := "new_yaml"
	p := &Profile{
		Env: EnvPolicySpec{
			Inherit: true,
			Vars: map[string]*string{
				"OVERRIDE_ME": &overrideVal,
			},
		},
	}

	res, err := p.SynthesizeEnvMap(hostEnv, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["HOST_A"] != "alpha" || res["HOST_B"] != "beta" {
		t.Errorf("expected host variables to be inherited, got HOST_A=%q HOST_B=%q", res["HOST_A"], res["HOST_B"])
	}
	if res["OVERRIDE_ME"] != "new_yaml" {
		t.Errorf("expected OVERRIDE_ME to be overridden, got %q", res["OVERRIDE_ME"])
	}

	// Test inherit via CLI flag
	resCLI, err := (&Profile{}).SynthesizeEnvMap(hostEnv, []string{"OVERRIDE_ME=new_cli"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resCLI["HOST_A"] != "alpha" || resCLI["HOST_B"] != "beta" {
		t.Errorf("expected host variables to be inherited with cliEnvInherit, got %v", resCLI)
	}
	if resCLI["OVERRIDE_ME"] != "new_cli" {
		t.Errorf("expected OVERRIDE_ME to be %q, got %q", "new_cli", resCLI["OVERRIDE_ME"])
	}
}

func TestSynthesizeEnvSlice(t *testing.T) {
	p := &Profile{}
	hostEnv := []string{"PATH=/bin"}
	slice, err := p.SynthesizeEnv(hostEnv, []string{"A=1", "B=2"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundA := false
	foundB := false
	for _, entry := range slice {
		if strings.HasPrefix(entry, "A=") {
			foundA = true
		}
		if strings.HasPrefix(entry, "B=") {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Errorf("expected A and B in slice, got %v", slice)
	}
}
