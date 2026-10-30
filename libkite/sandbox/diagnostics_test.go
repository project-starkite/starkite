package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDiagnostics_FormatPermissionError(t *testing.T) {
	home, _ := os.UserHomeDir()
	spec := &ExecutionSpec{
		Cwd:    "/workspace",
		NoExec: true,
		Mounts: []Mount{
			{Source: "/tmp/scratch", Destination: "/scratch", Mode: MountRW},
		},
		AllowedExecPaths: []string{"/usr/bin/git"},
	}

	tests := []struct {
		name       string
		err        error
		targetPath string
		spec       *ExecutionSpec
		wantSubstr []string
	}{
		{
			name:       "nil error",
			err:        nil,
			wantSubstr: nil,
		},
		{
			name:       "no_exec without path",
			err:        syscall.EACCES,
			spec:       &ExecutionSpec{NoExec: true},
			wantSubstr: []string{"subprocess execution denied by no_exec policy", "Hint: pass --sandbox-exec-allow=* on CLI"},
		},
		{
			name:       "dev shm isolation",
			err:        syscall.EACCES,
			targetPath: "/dev/shm/queue",
			spec:       spec,
			wantSubstr: []string{"access to \"/dev/shm/queue\" denied by shared memory (/dev/shm) isolation", "Hint: shared memory segments (/dev/shm) are isolated by default"},
		},
		{
			name:       "WX scratch mount /tmp",
			err:        syscall.EACCES,
			targetPath: "/tmp/exploit.sh",
			spec:       spec,
			wantSubstr: []string{"W^X noexec policy", "Hint: pass --sandbox-exec-allow=exploit.sh on CLI"},
		},
		{
			name:       "WX writable bind mount",
			err:        syscall.EACCES,
			targetPath: "/scratch/run.sh",
			spec:       spec,
			wantSubstr: []string{"execution of \"/scratch/run.sh\" denied on writable mount \"/scratch\"", "Hint: pass --sandbox-exec-allow=run.sh on CLI"},
		},
	}

	if home != "" {
		tests = append(tests, struct {
			name       string
			err        error
			targetPath string
			spec       *ExecutionSpec
			wantSubstr []string
		}{
			name:       "home directory isolation",
			err:        syscall.EACCES,
			targetPath: filepath.Join(home, ".kube", "config"),
			spec:       spec,
			wantSubstr: []string{"home directory isolation", "Hint: pass --sandbox-path-read="},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPermissionError(tt.err, tt.targetPath, tt.spec)
			if tt.err == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			errMsg := got.Error()
			for _, w := range tt.wantSubstr {
				if !strings.Contains(errMsg, w) {
					t.Errorf("FormatPermissionError() = %q, missing %q", errMsg, w)
				}
			}
		})
	}
}

func TestDiagnostics_FormatNetworkError(t *testing.T) {
	spec := &ExecutionSpec{
		Network: NetworkNone,
	}
	err := FormatNetworkError(syscall.EPERM, "api.github.com:443", spec)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "outbound network socket creation denied for host \"api.github.com:443\" (sandbox network: none)") {
		t.Errorf("FormatNetworkError missing header: %q", msg)
	}
	if !strings.Contains(msg, "Hint: pass --sandbox-net-allow=api.github.com:443 on CLI, or add \"api.github.com:443\" to sandbox.<profile>.network.allow in config.yaml") {
		t.Errorf("FormatNetworkError missing hint: %q", msg)
	}
}

func TestDiagnostics_FormatEnvScrubbedDiagnostic(t *testing.T) {
	got := FormatEnvScrubbedDiagnostic("KUBECONFIG")
	want := "[DEBUG] env \"KUBECONFIG\" requested by script is present on host but scrubbed by sandbox.\n  Hint: pass --env KUBECONFIG on CLI, or declare \"KUBECONFIG:\" in sandbox.<profile>.env in config.yaml"
	if got != want {
		t.Errorf("FormatEnvScrubbedDiagnostic() = %q, want %q", got, want)
	}
}

func TestDiagnostics_EnrichPermissionStderr(t *testing.T) {
	spec := &ExecutionSpec{
		Command: []string{"/bin/sh", "-c", "curl https://example.com"},
		Network: NetworkNone,
	}

	// 1. Network denial in stderr
	rawNetStderr := "curl: (7) Failed to connect to example.com port 443: Network is unreachable"
	enrichedNet := enrichPermissionStderr(rawNetStderr, spec)
	if !strings.Contains(enrichedNet, "sandbox: outbound network socket creation denied") {
		t.Errorf("enrichPermissionStderr network failed: %q", enrichedNet)
	}
	if !strings.Contains(enrichedNet, "Hint: pass --sandbox-net-allow on CLI") {
		t.Errorf("enrichPermissionStderr hint failed: %q", enrichedNet)
	}

	// 2. Exec denial in stderr
	specExec := &ExecutionSpec{
		Command: []string{"/usr/local/bin/deployer"},
		NoExec:  true,
	}
	rawExecStderr := "deployer: fork/exec /usr/local/bin/deployer: permission denied"
	enrichedExec := enrichPermissionStderr(rawExecStderr, specExec)
	if !strings.Contains(enrichedExec, "sandbox: execution of \"/usr/local/bin/deployer\" denied by no_exec policy") {
		t.Errorf("enrichPermissionStderr exec failed: %q", enrichedExec)
	}
	if !strings.Contains(enrichedExec, "Hint: pass --sandbox-exec-allow=deployer on CLI") {
		t.Errorf("enrichPermissionStderr exec hint failed: %q", enrichedExec)
	}
}

func TestDiagnostics_PolicySummary(t *testing.T) {
	prof := Profile{
		Name:    "ci-builder",
		Enabled: true,
		Network: NetworkNone,
		Paths: PathsSpec{
			Read:  []string{"/workspace"},
			Write: []string{"/tmp/out"},
		},
		Exec: ExecPolicySpec{
			Allow: []string{"go", "git"},
			Deny:  []string{"rm"},
		},
		Calls: CallsPolicySpec{
			Allow: []string{"fs.read"},
			Deny:  []string{"fs.write(/etc/**)"},
		},
		Env: EnvPolicySpec{
			Inherit: false,
			Vars: map[string]*string{
				"CI": nil,
			},
		},
	}

	summary := prof.PolicySummary([]string{"AWS_REGION=us-west-2", "KUBECONFIG"}, false, "build.star")
	if summary.ProfileName != "ci-builder" {
		t.Errorf("ProfileName = %q, want ci-builder", summary.ProfileName)
	}
	if summary.Network != "none" {
		t.Errorf("Network = %q, want none", summary.Network)
	}
	if !summary.EnvAllowed["CI"] {
		t.Error("EnvAllowed missing CI")
	}
	if !summary.EnvAllowed["AWS_REGION"] {
		t.Error("EnvAllowed missing AWS_REGION")
	}
	if !summary.EnvAllowed["KUBECONFIG"] {
		t.Error("EnvAllowed missing KUBECONFIG")
	}
	if summary.DisplayPath != "build.star" {
		t.Errorf("DisplayPath = %q, want build.star", summary.DisplayPath)
	}
}

func TestDiagnostics_Win32PermissionErrors(t *testing.T) {
	spec := &ExecutionSpec{
		Cwd:    `C:\workspace`,
		NoExec: true,
		Mounts: []Mount{
			{Source: `C:\scratch`, Destination: `C:\scratch`, Mode: MountRW},
		},
		AllowedExecPaths: []string{`C:\Windows\System32\cmd.exe`},
	}

	tests := []struct {
		name       string
		err        error
		targetPath string
		spec       *ExecutionSpec
		wantSubstr []string
	}{
		{
			name:       "winErrorAccessDenied on scratch mount",
			err:        winErrorAccessDenied,
			targetPath: `C:\Users\alice\AppData\Local\Temp\starkite-sbx-1234\payload.exe`,
			spec:       spec,
			wantSubstr: []string{"W^X noexec policy", "Hint: pass --sandbox-exec-allow=payload on CLI"},
		},
		{
			name:       "winErrorAccessDenied on writable mount",
			err:        winErrorAccessDenied,
			targetPath: `C:\scratch\run.bat`,
			spec:       spec,
			wantSubstr: []string{"denied on writable mount", "W^X noexec policy", "Hint: pass --sandbox-exec-allow=run on CLI"},
		},
		{
			name:       "winErrorAccessDenied on exec under NoExec",
			err:        winErrorAccessDenied,
			targetPath: `C:\Windows\System32\whoami.exe`,
			spec:       spec,
			wantSubstr: []string{"denied by no_exec policy", "Hint: pass --sandbox-exec-allow=whoami on CLI"},
		},
		{
			name:       "winErrorNotEnoughQuota under NoExec",
			err:        winErrorNotEnoughQuota,
			spec:       spec,
			wantSubstr: []string{"subprocess execution denied by no_exec policy (active process limit)", "Hint: pass --sandbox-exec-allow=* on CLI"},
		},
		{
			name:       "wsaErrorAccessDenied in FormatNetworkError",
			err:        wsaErrorAccessDenied,
			spec:       &ExecutionSpec{Network: NetworkNone},
			wantSubstr: []string{"outbound network socket creation denied for host \"api.github.com:443\"", "Hint: pass --sandbox-net-allow=api.github.com:443 on CLI"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got error
			if tt.name == "wsaErrorAccessDenied in FormatNetworkError" {
				got = FormatNetworkError(tt.err, "api.github.com:443", tt.spec)
			} else {
				got = FormatPermissionError(tt.err, tt.targetPath, tt.spec)
			}
			if got == nil {
				t.Fatal("expected non-nil error")
			}
			errMsg := got.Error()
			for _, w := range tt.wantSubstr {
				if !strings.Contains(errMsg, w) {
					t.Errorf("got %q, missing substring %q", errMsg, w)
				}
			}
		})
	}
}

func TestDiagnostics_EnrichPermissionStderr_Windows(t *testing.T) {
	// 1. Windows subprocess exec denial under NoExec
	specExec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "whoami.exe"},
		NoExec:  true,
	}
	rawExecStderr := "Access is denied."
	enrichedExec := enrichPermissionStderr(rawExecStderr, specExec)
	if !strings.Contains(enrichedExec, "sandbox: execution of \"whoami.exe\" denied by no_exec policy") {
		t.Errorf("enrichPermissionStderr exec failed: %q", enrichedExec)
	}
	if !strings.Contains(enrichedExec, "Hint: pass --sandbox-exec-allow=whoami on CLI") {
		t.Errorf("enrichPermissionStderr exec hint failed: %q", enrichedExec)
	}

	// 2. Windows socket denial in stderr
	specNet := &ExecutionSpec{
		Command: []string{"powershell.exe", "-Command", "New-Object System.Net.Sockets.TcpClient('1.1.1.1', 80)"},
		Network: NetworkNone,
	}
	rawNetStderr := "An attempt was made to access a socket in a way forbidden by its access permissions."
	enrichedNet := enrichPermissionStderr(rawNetStderr, specNet)
	if !strings.Contains(enrichedNet, "sandbox: outbound network socket creation denied") {
		t.Errorf("enrichPermissionStderr network failed: %q", enrichedNet)
	}
	if !strings.Contains(enrichedNet, "Hint: pass --sandbox-net-allow on CLI") {
		t.Errorf("enrichPermissionStderr hint failed: %q", enrichedNet)
	}
}

func TestDiagnostics_EnrichExecDiagnostics(t *testing.T) {
	// 1. Nil result
	EnrichExecDiagnostics(nil, nil)

	// 2. Success exit code: no modifications
	resOk := &ExecResult{ExitCode: 0, Stderr: "ModuleNotFoundError: No module named 'foo'"}
	EnrichExecDiagnostics(resOk, nil)
	if strings.Contains(resOk.Stderr, "Hint:") {
		t.Errorf("expected no hint for ExitCode=0, got: %q", resOk.Stderr)
	}

	// 3. Python ModuleNotFoundError
	resPy := &ExecResult{
		ExitCode: 1,
		Stderr:   "Traceback (most recent call last):\nModuleNotFoundError: No module named 'requests'",
	}
	EnrichExecDiagnostics(resPy, nil)
	wantPy := "Hint: pass --sandbox-profile=python-env on CLI, or add dependencies via --sandbox-path-read=<path>"
	if !strings.Contains(resPy.Stderr, wantPy) {
		t.Errorf("expected python hint in stderr, got: %q", resPy.Stderr)
	}

	// 4. Node.js Cannot find module
	resNode := &ExecResult{
		ExitCode: 1,
		Stderr:   "Error: Cannot find module 'axios'\nRequire stack:\n- /app/index.js",
	}
	EnrichExecDiagnostics(resNode, nil)
	wantNode := "Hint: pass --sandbox-profile=node-env on CLI, or grant access via --sandbox-path-read=node_modules"
	if !strings.Contains(resNode.Stderr, wantNode) {
		t.Errorf("expected node hint in stderr, got: %q", resNode.Stderr)
	}

	// 5. Shared library missing (Linux and Darwin)
	resSo := &ExecResult{
		ExitCode: 127,
		Stderr:   "./app: error while loading shared libraries: libssl.so.3: cannot open shared object file: No such file or directory",
	}
	EnrichExecDiagnostics(resSo, nil)
	wantLib := "Hint: grant read access to library location via --sandbox-path-read=<dir>"
	if !strings.Contains(resSo.Stderr, wantLib) {
		t.Errorf("expected library hint in stderr, got: %q", resSo.Stderr)
	}

	resDylib := &ExecResult{
		ExitCode: 127,
		Stderr:   "dyld[100]: Library not loaded: @rpath/libfoo.dylib\nReason: image not found",
	}
	EnrichExecDiagnostics(resDylib, nil)
	if !strings.Contains(resDylib.Stderr, wantLib) {
		t.Errorf("expected library hint in stderr for dyld, got: %q", resDylib.Stderr)
	}

	// 6. Network failure under isolated network
	specNetNone := &ExecutionSpec{
		Network: NetworkNone,
	}
	resNet := &ExecResult{
		ExitCode: 7,
		Stderr:   "curl: (7) Failed to connect to api.example.com: Connection refused",
	}
	EnrichExecDiagnostics(resNet, specNetNone)
	wantNet := "Hint: pass --sandbox-net-allow on CLI, or add endpoint to sandbox.<profile>.network.allow in config.yaml"
	if !strings.Contains(resNet.Stderr, wantNet) {
		t.Errorf("expected network hint in stderr, got: %q", resNet.Stderr)
	}

	// 7. Network failure under NetworkHost: hint should not be added
	specNetHost := &ExecutionSpec{
		Network: NetworkHost,
	}
	resNetHost := &ExecResult{
		ExitCode: 7,
		Stderr:   "curl: (7) Failed to connect to api.example.com: Connection refused",
	}
	EnrichExecDiagnostics(resNetHost, specNetHost)
	if strings.Contains(resNetHost.Stderr, wantNet) {
		t.Errorf("expected no network hint under NetworkHost, got: %q", resNetHost.Stderr)
	}

	// 8. Idempotency
	resIdempotent := &ExecResult{
		ExitCode: 1,
		Stderr:   "ModuleNotFoundError: No module named 'test'",
	}
	EnrichExecDiagnostics(resIdempotent, nil)
	firstPass := resIdempotent.Stderr
	EnrichExecDiagnostics(resIdempotent, nil)
	if resIdempotent.Stderr != firstPass {
		t.Errorf("expected idempotent stderr, got second pass:\n%s\nfirst pass:\n%s", resIdempotent.Stderr, firstPass)
	}
}
