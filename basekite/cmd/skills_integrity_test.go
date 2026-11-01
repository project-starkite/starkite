package cmd

import (
	"bufio"
	"strings"
	"testing"

	"go.starlark.net/syntax"
)

func TestSkillsMarkdownTableIntegrity(t *testing.T) {
	out, err := executeSkillsCmd("--skills")
	if err != nil {
		t.Fatalf("unexpected error generating skills: %v", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	inTable := false
	expectedPipes := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			// Ignore escaped pipes when counting table columns
			cleanPipes := strings.ReplaceAll(trimmed, "\\|", "")
			pipes := strings.Count(cleanPipes, "|")
			if !inTable {
				inTable = true
				expectedPipes = pipes
			} else {
				// Table row or separator
				if pipes != expectedPipes {
					t.Errorf("line %d: table row pipe count mismatch (expected %d, got %d): %s",
						lineNum, expectedPipes, pipes, trimmed)
				}
			}
		} else {
			inTable = false
			expectedPipes = 0
		}
	}
}

func TestSkillsFrontmatterYAMLValidity(t *testing.T) {
	testCases := []struct {
		target string
		args   []string
	}{
		{target: "unified", args: []string{"--skills"}},
		{target: "fs", args: []string{"fs", "--skills"}},
		{target: "cloud", args: []string{"--skills", "--category=cloud"}},
	}

	for _, tc := range testCases {
		out, err := executeSkillsCmd(tc.args...)
		if err != nil {
			t.Fatalf("skills generation failed for %s (%v): %v", tc.target, tc.args, err)
		}

		lines := strings.Split(out, "\n")
		if len(lines) < 5 || lines[0] != "---" {
			t.Fatalf("expected opening '---' frontmatter delimiter for %s", tc.target)
		}

		closingIdx := -1
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				closingIdx = i
				break
			}
		}
		if closingIdx == -1 {
			t.Fatalf("missing closing '---' frontmatter delimiter for %s", tc.target)
		}

		frontmatter := strings.Join(lines[1:closingIdx], "\n")
		if !strings.Contains(frontmatter, "name:") {
			t.Errorf("frontmatter missing 'name:' field in %s", tc.target)
		}
		if !strings.Contains(frontmatter, "description:") {
			t.Errorf("frontmatter missing 'description:' field in %s", tc.target)
		}
	}
}

func TestSkillsRecipeSyntaxValidation(t *testing.T) {
	out, err := executeSkillsCmd("--skills")
	if err != nil {
		t.Fatalf("unexpected error generating skills: %v", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	inCodeBlock := false
	var currentSnippet strings.Builder
	var recipeCount int

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```python") {
			inCodeBlock = true
			currentSnippet.Reset()
			continue
		}
		if inCodeBlock && strings.HasPrefix(line, "```") {
			inCodeBlock = false
			recipeCount++
			code := currentSnippet.String()
			_, parseErr := syntax.Parse("recipe.star", []byte(code), 0)
			if parseErr != nil {
				t.Errorf("recipe %d failed Starlark syntax validation: %v\nCode:\n%s", recipeCount, parseErr, code)
			}
			continue
		}
		if inCodeBlock {
			currentSnippet.WriteString(line)
			currentSnippet.WriteString("\n")
		}
	}

	if recipeCount == 0 {
		t.Fatalf("expected at least one recipe code block to test, found 0")
	}
}
