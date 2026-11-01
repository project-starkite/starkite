package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/project-starkite/starkite/libkite/catalog"
	"github.com/spf13/cobra"
)

var (
	docSkills   bool
	docCategory string
)

// categoryModules maps category names to their respective module namespaces.
var categoryModules = map[string][]string{
	"cloud": {"http", "k8s", "concur", "sql", "ssh", "containers"},
	"k8s":   {"k8s"},
	"data":  {"json", "yaml", "csv", "zip", "gzip", "semver"},
	"sys":   {"fs", "os", "time", "log", "crypto", "math", "strings", "args", "env", "runtime"},
}

// SkillConfig defines generation parameters for an agent skill document.
type SkillConfig struct {
	Name        string
	Title       string
	Description string
	Modules     []string
	IsModular   bool
}

// runSkills generates structured SKILL.md output based on target arguments and category flags.
func runSkills(cmd *cobra.Command, cat *catalog.Catalog, args []string, category string) error {
	out := cmd.OutOrStdout()
	cfg, err := resolveSkillConfig(cat, args, category)
	if err != nil {
		return err
	}
	return renderSkill(out, cat, cfg)
}

// resolveSkillConfig calculates skill metadata and module list from arguments.
func resolveSkillConfig(cat *catalog.Catalog, args []string, category string) (SkillConfig, error) {
	// Case 1: Targeted module (e.g. "kite doc k8s --skills")
	if len(args) > 0 {
		target := args[0]
		if !cat.IsModule(target) {
			return SkillConfig{}, fmt.Errorf("unknown module %q for skill generation. Run \"kite doc\" to list available modules.", target)
		}
		name := "starkite-" + target
		title := fmt.Sprintf("Starkite %s Automation Skill", strings.ToUpper(target))
		desc := fmt.Sprintf("Automate %s workflows, resource lifecycles, and systems operations using the Starkite %s module. "+
			"Use when writing, inspecting, or debugging Starkite scripts interacting with %s.", target, target, target)
		return SkillConfig{
			Name:        name,
			Title:       title,
			Description: desc,
			Modules:     []string{target},
			IsModular:   true,
		}, nil
	}

	// Case 2: Category-scoped skill (e.g. "kite doc --skills --category=cloud")
	if category != "" {
		mods, ok := categoryModules[strings.ToLower(category)]
		if !ok {
			validCats := make([]string, 0, len(categoryModules))
			for c := range categoryModules {
				validCats = append(validCats, c)
			}
			sort.Strings(validCats)
			return SkillConfig{}, fmt.Errorf("unknown category %q. Available categories: %s", category, strings.Join(validCats, ", "))
		}
		catName := strings.ToLower(category)
		name := "starkite-" + catName
		title := fmt.Sprintf("Starkite %s Operations Skill", titleCase(catName))
		desc := fmt.Sprintf("Automate %s operations, infrastructure lifecycles, and data workflows using Starkite. "+
			"Use when authoring or debugging Starkite automation scripts targeting %s modules (%s).",
			catName, catName, strings.Join(mods, ", "))
		return SkillConfig{
			Name:        name,
			Title:       title,
			Description: desc,
			Modules:     mods,
			IsModular:   true,
		}, nil
	}

	// Case 3: Unified Starkite skill ("kite doc --skills")
	allModulesMap := cat.Modules()
	allMods := make([]string, 0, len(allModulesMap))
	for m := range allModulesMap {
		allMods = append(allMods, m)
	}
	sort.Strings(allMods)

	return SkillConfig{
		Name:  "starkite",
		Title: "Starkite Automation DSL Skill",
		Description: "Automate cloud-native infrastructure, systems operations, Kubernetes workflows, " +
			"and local process tasks using the Starkite Starlark DSL. Use when writing, reviewing, " +
			"or debugging Starkite scripts (.star), executing cloud/system tasks with kite, " +
			"or automating workflows across files, HTTP, CLI commands, and Kubernetes.",
		Modules:   allMods,
		IsModular: false,
	}, nil
}

func titleCase(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// renderSkill generates the complete 3-tier progressive disclosure SKILL.md document.
func renderSkill(w io.Writer, cat *catalog.Catalog, cfg SkillConfig) error {
	// Frontmatter
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "name: %s\n", cfg.Name)
	fmt.Fprintln(w, "description: >-")
	for _, line := range wrapLines(cfg.Description, 76) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)

	// Document Title & Purpose
	fmt.Fprintf(w, "# %s\n\n", cfg.Title)
	fmt.Fprintln(w, "This skill teaches autonomous agents how to author idiomatic, sandboxed, and robust Starkite automation scripts.")
	fmt.Fprintln(w)

	// Tier 1: Core Language Invariants
	renderTier1Invariants(w)

	// Tier 2: Scripting Idioms & Recipes
	renderTier2Recipes(w, cfg)

	// Tier 3: Complete Module & Member Reference Tables
	renderTier3References(w, cat, cfg)

	return nil
}

func renderTier1Invariants(w io.Writer) {
	fmt.Fprintln(w, "## Tier 1: Core Language Invariants & Execution Rules")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Autonomous agents generating Starkite scripts MUST adhere to four load-bearing execution rules:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. **Automatic Entrypoint Execution (`main()`)**:")
	fmt.Fprintln(w, "   - Starkite scripts define entry logic inside `def main():` or `def main(ctx):`.")
	fmt.Fprintln(w, "   - **NEVER** call `main()` at the top-level. The runtime automatically discovers and invokes `main()`.")
	fmt.Fprintln(w, "   - Calling `main()` manually causes duplicate execution and corrupts exit status codes.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "2. **Error Handling via `try_` Result Pattern**:")
	fmt.Fprintln(w, "   - Built-in functions have safe `try_` variants (e.g. `fs.try_read_text`, `os.try_exec`, `http.try_get`).")
	fmt.Fprintln(w, "   - Safe functions return a `Result` struct with three fields: `res.ok` (bool), `res.value` (any), and `res.err` (string).")
	fmt.Fprintln(w, "   - **ALWAYS** check `res.ok` before reading `res.value`. If `res.ok` is False, handle `res.err` gracefully.")
	fmt.Fprintln(w, "   - Direct functions (e.g. `fs.read_text`) raise uncatchable runtime failures on error.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "3. **Kernel Sandbox Isolation Constraints**:")
	fmt.Fprintln(w, "   - Scripts execute within OS-level sandboxes (Landlock LSM on Linux, Seatbelt on macOS, AppContainers on Windows).")
	fmt.Fprintln(w, "   - By default, file writes and network access outside the script directory are restricted.")
	fmt.Fprintln(w, "   - Always use `fs.path` for path normalization across operating systems (automatically converts `/` and `\\`).")
	fmt.Fprintln(w, "   - Scripts requiring network or write access must use appropriate CLI flags (`--sandbox-net-allow`, `--sandbox-path-write`).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "4. **Auto-Loaded Standard Library Modules**:")
	fmt.Fprintln(w, "   - Standard modules (`fs`, `http`, `os`, `json`, `yaml`, `concur`, `time`, `log`, `csv`, `zip`, `gzip`, `semver`, `crypto`, `math`, `strings`) are predeclared globals.")
	fmt.Fprintln(w, "   - **DO NOT** write `import fs` or `load(\"fs\", ...)` for built-in modules.")
	fmt.Fprintln(w, "   - Use `load(\"//path/to/script.star\", \"symbol\")` strictly for relative or installed user modules.")
	fmt.Fprintln(w)
}

func renderTier2Recipes(w io.Writer, cfg SkillConfig) {
	io.WriteString(w, "## Tier 2: Scripting Idioms & Production Recipes\n\n")

	// File I/O & Path Recipe
	if containsAny(cfg.Modules, "fs") {
		io.WriteString(w, "### 1. Robust File I/O & Path Traversal\n```python\n"+
			"p = fs.path(\"data/config.json\")\n"+
			"if p.exists():\n"+
			"    # Safe read with Result handling\n"+
			"    res = fs.try_read_text(p)\n"+
			"    if res.ok:\n"+
			"        log.info(\"Read %d bytes from %s\", len(res.value), p.base())\n"+
			"    else:\n"+
			"        log.error(\"Read failed: %s\", res.err)\n"+
			"```\n\n")
	}

	// Subprocess & Command Execution Recipe
	if containsAny(cfg.Modules, "os") {
		io.WriteString(w, "### 2. Subprocess Execution & Command Piping\n```python\n"+
			"# Execute system command with argument vector (prevents shell injection)\n"+
			"res = os.try_exec([\"git\", \"status\", \"--porcelain\"])\n"+
			"if res.ok:\n"+
			"    stdout = res.value.stdout.strip()\n"+
			"    if stdout:\n"+
			"        log.warn(\"Uncommitted changes:\\n%s\", stdout)\n"+
			"else:\n"+
			"    log.error(\"Command failed with exit code %d: %s\", res.value.exit_code, res.err)\n"+
			"```\n\n")
	}

	// Structured Data Transform Recipe
	if containsAny(cfg.Modules, "json", "yaml") {
		io.WriteString(w, "### 3. Structured Data Transformation (JSON to YAML)\n```python\n"+
			"raw_json = '{\"service\": \"api\", \"replicas\": 3, \"enabled\": true}'\n"+
			"data = json.decode(raw_json)\n"+
			"data[\"environment\"] = \"production\"\n\n"+
			"# Encode transformed data directly into YAML\n"+
			"yaml_out = yaml.encode(data)\n"+
			"fs.write_text(\"deploy.yaml\", yaml_out)\n"+
			"```\n\n")
	}

	// Concurrency Recipe
	if containsAny(cfg.Modules, "concur", "http") {
		io.WriteString(w, "### 4. Concurrent Fan-Out & Rate-Limited Pipelines\n```python\n"+
			"def check_endpoint(target):\n"+
			"    res = http.try_get(\"https://\" + target + \"/healthz\")\n"+
			"    return {\"target\": target, \"alive\": res.ok and res.value.status_code == 200}\n\n"+
			"endpoints = [\"node1.internal\", \"node2.internal\", \"node3.internal\"]\n"+
			"# Concurrently check endpoints with worker pool limit of 2\n"+
			"results = concur.each(endpoints, check_endpoint, limit=2)\n"+
			"for r in results:\n"+
			"    log.info(\"Endpoint %s alive=%s\", r[\"target\"], r[\"alive\"])\n"+
			"```\n\n")
	}

	// Kubernetes Cluster Automation Recipe
	if containsAny(cfg.Modules, "k8s") {
		io.WriteString(w, "### 5. Kubernetes Cluster Triage & Resource Inspection\n```python\n"+
			"client = k8s.client()\n"+
			"pods_res = client.try_list(\"pods\", namespace=\"default\")\n"+
			"if pods_res.ok:\n"+
			"    for pod in pods_res.value:\n"+
			"        phase = pod.status.phase\n"+
			"        if phase != \"Running\":\n"+
			"            log.warn(\"Pod %s is in state: %s\", pod.metadata.name, phase)\n"+
			"else:\n"+
			"    log.error(\"Failed to query cluster: %s\", pods_res.err)\n"+
			"```\n\n")
	}
}

func renderTier3References(w io.Writer, cat *catalog.Catalog, cfg SkillConfig) {
	fmt.Fprintln(w, "## Tier 3: Complete Module & Member Reference Tables")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Reference documentation for all functions, methods, and return types within this skill's scope.")
	fmt.Fprintln(w)

	sortedMods := make([]string, len(cfg.Modules))
	copy(sortedMods, cfg.Modules)
	sort.Strings(sortedMods)

	for _, modName := range sortedMods {
		desc, _ := cat.LookupModule(modName)
		members := cat.Members(modName)
		if len(members) == 0 && desc == "" {
			continue
		}

		fmt.Fprintf(w, "### Module `%s`\n\n", modName)
		if desc != "" {
			fmt.Fprintf(w, "%s\n\n", desc)
		}

		// Separate plain functions and safe variants
		var funcs []catalog.Symbol
		for _, m := range members {
			if !strings.HasPrefix(m.Name, "try_") {
				funcs = append(funcs, m)
			}
		}

		if len(funcs) > 0 {
			fmt.Fprintln(w, "| Member | Signature | Returns | Description | Safe Variant |")
			fmt.Fprintln(w, "|---|---|---|---|---|")
			for _, f := range funcs {
				sig := f.Signature
				if sig == "" {
					sig = fmt.Sprintf("%s.%s()", modName, f.Name)
				}
				ret := f.Returns
				if ret == "" {
					ret = "None"
				}
				doc := cleanDoc(f.Doc)
				tryVariant := "-"
				tryName := "try_" + f.Name
				if _, ok := cat.LookupMember(modName, tryName); ok {
					tryVariant = fmt.Sprintf("`%s.%s`", modName, tryName)
				}
				fmt.Fprintf(w, "| `%s` | `%s` | `%s` | %s | %s |\n", f.Name, escapePipes(sig), ret, doc, tryVariant)
			}
			fmt.Fprintln(w)
		}

		// Check for exported types
		for _, m := range members {
			typeKey := modName + "." + m.Name
			if typeMembers := cat.Members(typeKey); len(typeMembers) > 0 {
				fmt.Fprintf(w, "#### Exported Type `%s` Methods\n\n", typeKey)
				fmt.Fprintln(w, "| Method | Signature | Returns | Description |")
				fmt.Fprintln(w, "|---|---|---|---|")
				for _, tm := range typeMembers {
					sig := tm.Signature
					if sig == "" {
						sig = fmt.Sprintf("%s.%s()", typeKey, tm.Name)
					}
					ret := tm.Returns
					if ret == "" {
						ret = "None"
					}
					doc := cleanDoc(tm.Doc)
					fmt.Fprintf(w, "| `%s` | `%s` | `%s` | %s |\n", tm.Name, escapePipes(sig), ret, doc)
				}
				fmt.Fprintln(w)
			}
		}
	}
}

func containsAny(slice []string, items ...string) bool {
	for _, s := range slice {
		for _, item := range items {
			if s == item {
				return true
			}
		}
	}
	return false
}

func cleanDoc(doc string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return "-"
	}
	firstLine := strings.Split(doc, "\n")[0]
	return escapePipes(firstLine)
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func wrapLines(s string, limit int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	curr := words[0]
	for _, w := range words[1:] {
		if len(curr)+1+len(w) > limit {
			lines = append(lines, curr)
			curr = w
		} else {
			curr += " " + w
		}
	}
	lines = append(lines, curr)
	return lines
}
