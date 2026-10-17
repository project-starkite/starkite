package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRule(t *testing.T) {
	ruleStr := "group=apps version=v1 resource=deployments operations=CREATE,UPDATE"
	rule, err := parseRule(ruleStr)
	if err != nil {
		t.Fatalf("parseRule error: %v", err)
	}

	apiGroups, ok := rule["apiGroups"].([]string)
	if !ok || len(apiGroups) != 1 || apiGroups[0] != "apps" {
		t.Fatalf("apiGroups = %v, want ['apps']", rule["apiGroups"])
	}

	apiVersions, ok := rule["apiVersions"].([]string)
	if !ok || len(apiVersions) != 1 || apiVersions[0] != "v1" {
		t.Fatalf("apiVersions = %v, want ['v1']", rule["apiVersions"])
	}

	resources, ok := rule["resources"].([]string)
	if !ok || len(resources) != 1 || resources[0] != "deployments" {
		t.Fatalf("resources = %v, want ['deployments']", rule["resources"])
	}

	ops, ok := rule["operations"].([]string)
	if !ok || len(ops) != 2 || ops[0] != "CREATE" || ops[1] != "UPDATE" {
		t.Fatalf("operations = %v, want ['CREATE', 'UPDATE']", rule["operations"])
	}

	// Invalid format
	if _, err := parseRule("invalid_rule"); err == nil {
		t.Fatal("expected error for invalid rule syntax")
	}

	// Unknown key
	if _, err := parseRule("foo=bar"); err == nil {
		t.Fatal("expected error for unknown rule key")
	}
}

func TestWriteDockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	genController = "controllers/reconciler.star"
	genResource = "crds/my_crd.star"
	defer func() {
		genController = ""
		genResource = ""
	}()

	err := writeDockerfile(dockerfilePath)
	if err != nil {
		t.Fatalf("writeDockerfile failed: %v", err)
	}

	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read Dockerfile failed: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "COPY controllers/reconciler.star /app/reconciler.star") {
		t.Fatalf("Dockerfile missing expected controller COPY: %s", content)
	}
	if !strings.Contains(content, "COPY crds/my_crd.star /app/my_crd.star") {
		t.Fatalf("Dockerfile missing expected resource COPY: %s", content)
	}
	if !strings.Contains(content, `CMD ["kite", "run", "/app/reconciler.star"]`) {
		t.Fatalf("Dockerfile missing expected CMD: %s", content)
	}

	// Reject paths with newline
	genController = "foo\nbar"
	if err := writeDockerfile(dockerfilePath); err == nil {
		t.Fatal("expected writeDockerfile to reject newline in controller path")
	}
}
