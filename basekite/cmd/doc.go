package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/catalog"
	"github.com/spf13/cobra"
)

var (
	docJSON bool
)

var docCmd = &cobra.Command{
	Use:   "doc [module[.symbol]]",
	Short: "Show reference documentation for Starkite modules and functions",
	Long: `Show reference documentation for Starkite modules, functions, and types.

The documentation is read directly from the runtime's symbol catalog and API reference
tables, ensuring that descriptions match the compiled edition.

Examples:
  # List all available modules and top-level builtins:
  kite doc

  # Show all functions and types in a module:
  kite doc fs
  kite doc http

  # Show documentation for a specific function or type:
  kite doc fs.path
  kite doc http.get
  kite doc fs path

  # Show documentation for a global builtin:
  kite doc print

  # Output machine-readable JSON schema:
  kite doc fs --json
  kite doc fs.path --json`,
	Args: cobra.MaximumNArgs(2),
	RunE: runDoc,
}

func init() {
	docCmd.Flags().BoolVar(&docJSON, "json", false, "output documentation in JSON format")
	docCmd.Flags().BoolVar(&docSkills, "skills", false, "generate structured agent skill documentation (SKILL.md)")
	docCmd.Flags().StringVar(&docCategory, "category", "", "scope skill generation by category (cloud, k8s, data, sys)")
	rootCmd.AddCommand(docCmd)
}

// DocOverviewJSON represents the top-level catalog summary for bare "kite doc --json".
type DocOverviewJSON struct {
	Modules []ModuleSummaryJSON `json:"modules"`
	Globals []SymbolJSON        `json:"globals"`
}

// ModuleSummaryJSON represents a single module entry in the overview list.
type ModuleSummaryJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SymbolCount int    `json:"symbol_count"`
}

// ModuleDocJSON represents the complete documentation of a module for "kite doc <module> --json".
type ModuleDocJSON struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Functions    []SymbolJSON         `json:"functions"`
	SafeVariants []SymbolJSON         `json:"safe_variants,omitempty"`
	Types        []TypeDescriptorJSON `json:"types,omitempty"`
}

// TypeDescriptorJSON represents an exported object type and its methods.
type TypeDescriptorJSON struct {
	Name    string       `json:"name"`
	Methods []SymbolJSON `json:"methods"`
}

// SymbolDocJSON represents detailed documentation for a single symbol for "kite doc <module>.<symbol> --json".
type SymbolDocJSON struct {
	Symbol  SymbolJSON   `json:"symbol"`
	Methods []SymbolJSON `json:"methods,omitempty"`
}

// SymbolJSON represents a single inspectable entity.
type SymbolJSON struct {
	Name      string   `json:"name"`
	Owner     string   `json:"owner,omitempty"`
	Kind      string   `json:"kind"`
	Signature string   `json:"signature,omitempty"`
	Params    []string `json:"params,omitempty"`
	Returns   string   `json:"returns,omitempty"`
	Doc       string   `json:"doc,omitempty"`
}

func toSymbolJSON(s catalog.Symbol) SymbolJSON {
	return SymbolJSON{
		Name:      s.Name,
		Owner:     s.Owner,
		Kind:      s.Kind.String(),
		Signature: s.Signature,
		Params:    s.Params,
		Returns:   s.Returns,
		Doc:       s.Doc,
	}
}

func runDoc(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	reg := NewRegistry(&libkite.ModuleConfig{})
	cat := catalog.Build(reg)

	// Validate mutually exclusive flags
	if docSkills && docJSON {
		return fmt.Errorf("cannot specify both --skills and --json")
	}

	// Dispatch to Agent Skills Generator
	if docSkills {
		return runSkills(cmd, cat, args, docCategory)
	}

	// Case 1: Bare "kite doc" -> Overview of modules and globals
	if len(args) == 0 {
		return renderOverview(out, cat, docJSON)
	}

	// Resolve target: normalize "kite doc fs path" to "fs.path"
	target := args[0]
	if len(args) == 2 {
		target = args[0] + "." + args[1]
	}

	// Case 2: Target is a top-level module namespace
	if cat.IsModule(target) {
		return renderModule(out, cat, target, docJSON)
	}

	// Case 3: Target is a member or method path ("fs.path", "http.get", "fs.path.read_text")
	if strings.Contains(target, ".") {
		return renderMember(out, cat, target, docJSON)
	}

	// Case 4: Target is a top-level global builtin ("print", "fail", "struct")
	if sym, ok := cat.LookupGlobal(target); ok {
		return renderGlobal(out, sym, docJSON)
	}

	// Case 5: Target not found — provide helpful diagnostic error
	return fmt.Errorf("unknown module or symbol %q. Run \"kite doc\" to list available modules.", target)
}

func renderOverview(w io.Writer, cat *catalog.Catalog, asJSON bool) error {
	modulesMap := cat.Modules()
	moduleNames := make([]string, 0, len(modulesMap))
	for name := range modulesMap {
		moduleNames = append(moduleNames, name)
	}
	sort.Strings(moduleNames)

	globals := cat.Globals()

	if asJSON {
		overview := DocOverviewJSON{
			Modules: make([]ModuleSummaryJSON, 0, len(moduleNames)),
			Globals: make([]SymbolJSON, 0, len(globals)),
		}
		for _, name := range moduleNames {
			members := cat.Members(name)
			overview.Modules = append(overview.Modules, ModuleSummaryJSON{
				Name:        name,
				Description: modulesMap[name],
				SymbolCount: len(members),
			})
		}
		for _, g := range globals {
			if !cat.IsModule(g.Name) {
				overview.Globals = append(overview.Globals, toSymbolJSON(g))
			}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(overview)
	}

	fmt.Fprintln(w, "Starkite Reference Documentation")
	fmt.Fprintln(w, "================================")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Available Modules:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, name := range moduleNames {
		desc := modulesMap[name]
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", name, desc)
	}
	tw.Flush()

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Predeclared Globals:")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, g := range globals {
		if cat.IsModule(g.Name) {
			continue
		}
		doc := g.Doc
		if doc == "" {
			doc = g.Signature
		}
		if doc == "" {
			doc = g.Kind.String()
		}
		fmt.Fprintf(tw, "  %s\t%s\n", g.Name, doc)
	}
	tw.Flush()

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use \"kite doc <module>\" for module details (e.g. \"kite doc fs\").")
	fmt.Fprintln(w, "Use \"kite doc <module>.<symbol>\" for function details (e.g. \"kite doc fs.path\").")
	return nil
}

func renderModule(w io.Writer, cat *catalog.Catalog, moduleName string, asJSON bool) error {
	desc, _ := cat.LookupModule(moduleName)
	members := cat.Members(moduleName)

	var funcs, safeVariants []catalog.Symbol
	for _, m := range members {
		if strings.HasPrefix(m.Name, "try_") {
			safeVariants = append(safeVariants, m)
		} else {
			funcs = append(funcs, m)
		}
	}

	// Check for exported types associated with this module
	var types []TypeDescriptorJSON
	for _, m := range members {
		typeKey := moduleName + "." + m.Name
		if typeMembers := cat.Members(typeKey); len(typeMembers) > 0 {
			td := TypeDescriptorJSON{Name: typeKey}
			for _, tm := range typeMembers {
				td.Methods = append(td.Methods, toSymbolJSON(tm))
			}
			types = append(types, td)
		}
	}

	if asJSON {
		modDoc := ModuleDocJSON{
			Name:         moduleName,
			Description:  desc,
			Functions:    make([]SymbolJSON, len(funcs)),
			SafeVariants: make([]SymbolJSON, len(safeVariants)),
			Types:        types,
		}
		for i, f := range funcs {
			modDoc.Functions[i] = toSymbolJSON(f)
		}
		for i, s := range safeVariants {
			modDoc.SafeVariants[i] = toSymbolJSON(s)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(modDoc)
	}

	fmt.Fprintf(w, "MODULE: %s\n", moduleName)
	if desc != "" {
		fmt.Fprintf(w, "%s\n", desc)
	}
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintln(w)

	if len(funcs) > 0 {
		fmt.Fprintln(w, "Functions & Properties:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, f := range funcs {
			sig := f.Signature
			if sig == "" {
				sig = fmt.Sprintf("%s.%s()", moduleName, f.Name)
			}
			ret := f.Returns
			if ret != "" {
				ret = "-> " + ret
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", sig, ret, f.Doc)
		}
		tw.Flush()
		fmt.Fprintln(w)
	}

	if len(safeVariants) > 0 {
		fmt.Fprintln(w, "Safe Result Variants (try_*):")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, s := range safeVariants {
			sig := s.Signature
			if sig == "" {
				sig = fmt.Sprintf("%s.%s()", moduleName, s.Name)
			} else if strings.HasPrefix(s.Name, "try_") && !strings.Contains(sig, "try_") {
				baseName := strings.TrimPrefix(s.Name, "try_")
				sig = strings.Replace(sig, baseName, s.Name, 1)
			}
			ret := s.Returns
			if ret == "" {
				ret = "-> Result"
			} else {
				ret = "-> " + ret
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", sig, ret, s.Doc)
		}
		tw.Flush()
		fmt.Fprintln(w)
	}

	if len(types) > 0 {
		fmt.Fprintln(w, "Exported Types:")
		for _, t := range types {
			fmt.Fprintf(w, "  Type: %s (%d methods)\n", t.Name, len(t.Methods))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Use \"kite doc %s.<symbol>\" for parameter details.\n", moduleName)
	return nil
}

func renderMember(w io.Writer, cat *catalog.Catalog, target string, asJSON bool) error {
	lastDot := strings.LastIndex(target, ".")
	owner := target[:lastDot]
	memberName := target[lastDot+1:]

	if owner == "" || memberName == "" {
		return fmt.Errorf("invalid symbol format %q. Expected \"<module>.<symbol>\" (e.g. \"fs.path\").", target)
	}

	sym, found := cat.LookupMember(owner, memberName)
	if !found {
		// Check if the target itself is an exported type (e.g. "fs.path")
		if typeMembers := cat.Members(target); len(typeMembers) > 0 {
			return renderExportedType(w, target, typeMembers, asJSON)
		}
		return fmt.Errorf("symbol %q not found in %q. Run \"kite doc %s\" to see available members.", memberName, owner, owner)
	}

	typeMembers := cat.Members(target)

	if asJSON {
		res := SymbolDocJSON{Symbol: toSymbolJSON(sym)}
		for _, tm := range typeMembers {
			res.Methods = append(res.Methods, toSymbolJSON(tm))
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	fmt.Fprintf(w, "SYMBOL: %s.%s\n", sym.Owner, sym.Name)
	fmt.Fprintln(w, strings.Repeat("-", 60))
	if sym.Signature != "" {
		fmt.Fprintf(w, "Signature:  %s\n", sym.Signature)
	}
	if sym.Returns != "" {
		fmt.Fprintf(w, "Returns:    %s\n", sym.Returns)
	}
	if len(sym.Params) > 0 {
		fmt.Fprintf(w, "Parameters: %s\n", strings.Join(sym.Params, ", "))
	}
	fmt.Fprintf(w, "Kind:       %s\n", sym.Kind)
	if sym.Doc != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Description:\n  %s\n", sym.Doc)
	}

	// Check if this symbol has an associated try_ safe variant
	if !strings.HasPrefix(sym.Name, "try_") {
		tryName := "try_" + sym.Name
		if trySym, ok := cat.LookupMember(owner, tryName); ok {
			trySig := trySym.Signature
			if trySig == "" {
				trySig = fmt.Sprintf("%s.%s()", owner, tryName)
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "Safe Variant:\n  %s (returns Result with .ok, .value, .err)\n", trySig)
		}
	}

	if len(typeMembers) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Exported Type Methods on %s (%d total):\n", target, len(typeMembers))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, m := range typeMembers {
			sig := m.Signature
			if sig == "" {
				sig = fmt.Sprintf("%s.%s()", target, m.Name)
			}
			ret := m.Returns
			if ret != "" {
				ret = "-> " + ret
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", sig, ret, m.Doc)
		}
		tw.Flush()
	}

	return nil
}

func renderExportedType(w io.Writer, typeName string, methods []catalog.Symbol, asJSON bool) error {
	if asJSON {
		td := TypeDescriptorJSON{Name: typeName}
		for _, m := range methods {
			td.Methods = append(td.Methods, toSymbolJSON(m))
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(td)
	}

	fmt.Fprintf(w, "TYPE: %s\n", typeName)
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintf(w, "Exported Methods & Properties (%d total):\n\n", len(methods))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, m := range methods {
		sig := m.Signature
		if sig == "" {
			sig = fmt.Sprintf("%s.%s()", typeName, m.Name)
		}
		ret := m.Returns
		if ret != "" {
			ret = "-> " + ret
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", sig, ret, m.Doc)
	}
	tw.Flush()
	return nil
}

func renderGlobal(w io.Writer, sym catalog.Symbol, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(SymbolDocJSON{Symbol: toSymbolJSON(sym)})
	}

	fmt.Fprintf(w, "GLOBAL: %s\n", sym.Name)
	fmt.Fprintln(w, strings.Repeat("-", 60))
	if sym.Signature != "" {
		fmt.Fprintf(w, "Signature:   %s\n", sym.Signature)
	}
	if sym.Returns != "" {
		fmt.Fprintf(w, "Returns:     %s\n", sym.Returns)
	}
	fmt.Fprintf(w, "Kind:        %s\n", sym.Kind)
	if sym.Doc != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Description:\n  %s\n", sym.Doc)
	}
	return nil
}
