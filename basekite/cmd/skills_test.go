package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeSkillsCmd(args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	docJSON = false
	docSkills = false
	docCategory = ""

	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--skills":
			docSkills = true
		case a == "--json":
			docJSON = true
		case strings.HasPrefix(a, "--category="):
			docCategory = strings.TrimPrefix(a, "--category=")
		case a == "--category" && i+1 < len(args):
			docCategory = args[i+1]
			i++
		default:
			filteredArgs = append(filteredArgs, a)
		}
	}
	err := runDoc(cmd, filteredArgs)
	return buf.String(), err
}

func TestSkillsFlagConflict(t *testing.T) {
	_, err := executeSkillsCmd("--skills", "--json")
	if err == nil {
		t.Fatalf("expected error when passing both --skills and --json, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify both --skills and --json") {
		t.Errorf("expected conflict error message, got: %v", err)
	}
}

func TestSkillsUnifiedGeneration(t *testing.T) {
	out, err := executeSkillsCmd("--skills")
	if err != nil {
		t.Fatalf("unexpected error running kite doc --skills: %v", err)
	}

	// 1. Verify Frontmatter
	if !strings.HasPrefix(out, "---\nname: starkite\n") {
		t.Errorf("expected valid YAML frontmatter with name 'starkite', got:\n%s", out[:min(len(out), 100)])
	}
	if !strings.Contains(out, "description: >-") {
		t.Errorf("expected multiline YAML description in frontmatter")
	}

	// 2. Verify Tier 1 Invariants
	expectedInvariants := []string{
		"Tier 1: Core Language Invariants",
		"Automatic Entrypoint Execution (`main()`)",
		"Error Handling via `try_` Result Pattern",
		"Kernel Sandbox Isolation Constraints",
		"Auto-Loaded Standard Library Modules",
	}
	for _, inv := range expectedInvariants {
		if !strings.Contains(out, inv) {
			t.Errorf("expected output to contain invariant %q", inv)
		}
	}

	// 3. Verify Tier 2 Recipes
	expectedRecipes := []string{
		"Tier 2: Scripting Idioms & Production Recipes",
		"Robust File I/O & Path Traversal",
		"Subprocess Execution & Command Piping",
		"Structured Data Transformation",
		"Concurrent Fan-Out",
	}
	for _, rec := range expectedRecipes {
		if !strings.Contains(out, rec) {
			t.Errorf("expected output to contain recipe %q", rec)
		}
	}

	// 4. Verify Tier 3 Markdown Tables
	if !strings.Contains(out, "Tier 3: Complete Module & Member Reference Tables") {
		t.Errorf("expected Tier 3 reference tables heading")
	}
	if !strings.Contains(out, "| Member | Signature | Returns | Description | Safe Variant |") {
		t.Errorf("expected standard markdown table columns")
	}
}

func TestSkillsTargetedModule(t *testing.T) {
	out, err := executeSkillsCmd("fs", "--skills")
	if err != nil {
		t.Fatalf("unexpected error running kite doc fs --skills: %v", err)
	}

	if !strings.Contains(out, "name: starkite-fs") {
		t.Errorf("expected skill name 'starkite-fs', got:\n%s", out[:min(len(out), 100)])
	}
	if !strings.Contains(out, "### Module `fs`") {
		t.Errorf("expected module fs reference section")
	}
	// Should not contain unrelated modules
	if strings.Contains(out, "### Module `ssh`") {
		t.Errorf("targeted fs skill should not include ssh module")
	}
}

func TestSkillsCategoryScoped(t *testing.T) {
	out, err := executeSkillsCmd("--skills", "--category=cloud")
	if err != nil {
		t.Fatalf("unexpected error running kite doc --skills --category=cloud: %v", err)
	}

	if !strings.Contains(out, "name: starkite-cloud") {
		t.Errorf("expected skill name 'starkite-cloud', got:\n%s", out[:min(len(out), 100)])
	}
	if !strings.Contains(out, "### Module `http`") {
		t.Errorf("expected cloud skill to include http module")
	}
	if !strings.Contains(out, "### Module `concur`") {
		t.Errorf("expected cloud skill to include concur module")
	}
	// Should not contain non-cloud modules
	if strings.Contains(out, "### Module `zip`") {
		t.Errorf("cloud skill should not contain zip module")
	}
}

func TestSkillsInvalidCategory(t *testing.T) {
	_, err := executeSkillsCmd("--skills", "--category=invalid-cat")
	if err == nil {
		t.Fatalf("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "unknown category \"invalid-cat\"") {
		t.Errorf("expected unknown category error, got: %v", err)
	}
}

func TestSkillsCategoryWithoutSkills(t *testing.T) {
	_, err := executeSkillsCmd("--category=cloud")
	if err == nil {
		t.Fatalf("expected error when passing --category without --skills, got nil")
	}
	if !strings.Contains(err.Error(), "--category flag requires --skills") {
		t.Errorf("expected '--category flag requires --skills' error, got: %v", err)
	}
}

func TestSkillsModuleWithCategory(t *testing.T) {
	_, err := executeSkillsCmd("k8s", "--skills", "--category=cloud")
	if err == nil {
		t.Fatalf("expected error when passing both module arg and --category, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify both a module argument and --category") {
		t.Errorf("expected 'cannot specify both a module argument and --category' error, got: %v", err)
	}
}
