package dogfood_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/tests/dogfood"
)

func locateOrBuildKite(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}
	exeName := "kite"
	if runtime.GOOS == "windows" {
		exeName = "kite.exe"
	}
	kiteBin := filepath.Join(repoRoot, "bin", exeName)
	if _, err := os.Stat(kiteBin); err == nil {
		return kiteBin
	}

	tempKite := filepath.Join(t.TempDir(), exeName)
	buildCmd := exec.Command("go", "build", "-o", tempKite, filepath.Join(repoRoot, "kite"))
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build kite: %v\nOutput: %s", err, string(out))
	}
	return tempKite
}

func TestAgentSelfCorrectionLoop(t *testing.T) {
	kiteBin := locateOrBuildKite(t)
	workDir := t.TempDir()

	// Create helper.star for import test
	helperPath := filepath.Join(workDir, "helper.star")
	if err := os.WriteFile(helperPath, []byte(`def greet():
    print("HELPER_GREET")
`), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		initialScript string
		expectedCode  string
		healedScript  string
		expectedRun   string
	}{
		{
			name: "Top-Level main() invocation remediation (L001)",
			initialScript: `def main():
    print("HELLO_STARKITE")

main()
`,
			expectedCode: "L001",
			healedScript: `def main():
    print("HELLO_STARKITE")
`,
			expectedRun: "HELLO_STARKITE",
		},
		{
			name: "Unchecked try_ Result remediation (L002)",
			initialScript: `def main():
    res = os.try_exec("go", ["version"])
    print(res.value)
`,
			expectedCode: "L002",
			healedScript: `def main():
    res = os.try_exec("go", ["version"])
    if res.ok:
        print("SAFE_EXEC")
`,
			expectedRun: "SAFE_EXEC",
		},
		{
			name: "Deprecated os.system remediation (L003)",
			initialScript: `def main():
    os.system("echo DEPRECATED")
`,
			expectedCode: "L003",
			healedScript: `def main():
    res = os.exec("go", ["version"])
    print("SAFE_EXEC")
`,
			expectedRun: "SAFE_EXEC",
		},
		{
			name: "Privileged port sandbox policy remediation (L004)",
			initialScript: `def main():
    srv = http.server(port=80)
`,
			expectedCode: "L004",
			healedScript: `def main():
    srv = http.server(port=8080)
    print("SERVER_CONFIGURED")
`,
			expectedRun: "SERVER_CONFIGURED",
		},
		{
			name: "Bare .star load() import syntax remediation (L005)",

			initialScript: `load("helper.star", "helper")

def main():
    helper.greet()
`,
			expectedCode: "L005",
			healedScript: `load("./helper.star", "helper")

def main():
    helper.greet()
`,
			expectedRun: "HELPER_GREET",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scriptPath := filepath.Join(workDir, "test_"+tc.expectedCode+".star")
			if err := os.WriteFile(scriptPath, []byte(tc.initialScript), 0644); err != nil {
				t.Fatal(err)
			}

			// Step 1: Pre-flight validation - must catch deliberate defect
			results, err := dogfood.RunValidate(kiteBin, scriptPath)
			if err != nil {
				t.Fatalf("runValidate error: %v", err)
			}
			if len(results) == 0 {
				t.Fatalf("expected validation results, got none")
			}

			if !dogfood.HasDiagnosticCode(results, tc.expectedCode) {
				t.Fatalf("expected diagnostic code %s not found in %+v", tc.expectedCode, results[0].Diagnostics)
			}

			// Step 2: Agent self-heals script
			if err := os.WriteFile(scriptPath, []byte(tc.healedScript), 0644); err != nil {
				t.Fatal(err)
			}

			// Step 3: Re-validate healed script - must pass cleanly (no errors or warnings)
			healedResults, err := dogfood.RunValidate(kiteBin, scriptPath)
			if err != nil {
				t.Fatalf("re-runValidate error: %v", err)
			}
			for _, d := range healedResults[0].Diagnostics {
				if d.Severity == "error" || d.Severity == "warning" {
					t.Fatalf("unexpected warning/error on healed script: %+v", d)
				}
			}

			// Step 4: Execute healed script under sandbox
			out, err := dogfood.RunScript(kiteBin, scriptPath, "--sandbox-enabled=false")
			if err != nil {
				t.Fatalf("execution failed: %v\nOutput: %s", err, out)
			}

			if !strings.Contains(out, tc.expectedRun) {
				t.Fatalf("expected run output %q, got: %s", tc.expectedRun, out)
			}
		})
	}
}
