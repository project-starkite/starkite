package args

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// FormatScriptHelp generates a Unix-style help screen for a script based on declared arguments.
func FormatScriptHelp(ctx *libkite.ScriptArgsContext) string {
	scriptPath := ctx.ScriptPath
	if scriptPath == "" {
		scriptPath = "./script.star"
	}
	baseName := filepath.Base(scriptPath)

	var sb strings.Builder

	// Usage line
	sb.WriteString("Usage: kite run ")
	sb.WriteString(scriptPath)
	if len(ctx.Flags) > 0 {
		sb.WriteString(" [flags]")
	}
	for _, p := range ctx.Positionals {
		if p.Required {
			sb.WriteString(" <" + p.Name + ">")
		} else {
			sb.WriteString(" [" + p.Name + "]")
		}
	}
	sb.WriteString("\n")

	// Arguments section
	if len(ctx.Positionals) > 0 {
		sb.WriteString("\nArguments:\n")
		maxArgWidth := 0
		for _, p := range ctx.Positionals {
			nameCol := "<" + p.Name + ">"
			if len(nameCol) > maxArgWidth {
				maxArgWidth = len(nameCol)
			}
		}
		if maxArgWidth < 26 {
			maxArgWidth = 26
		}

		for _, p := range ctx.Positionals {
			nameCol := "<" + p.Name + ">"
			sb.WriteString("  ")
			sb.WriteString(nameCol)
			if pad := maxArgWidth - len(nameCol); pad > 0 {
				sb.WriteString(strings.Repeat(" ", pad))
			}
			sb.WriteString("  ")

			var descParts []string
			if p.Help != "" {
				descParts = append(descParts, p.Help)
			}
			if p.Required {
				descParts = append(descParts, "(required)")
			} else if p.Default != nil && p.Default != starlark.None {
				descParts = append(descParts, fmt.Sprintf("(default: %s)", p.Default))
			}
			sb.WriteString(strings.Join(descParts, " "))
			sb.WriteString("\n")
		}
	}

	// Flags section
	sb.WriteString("\nFlags:\n")
	type flagEntry struct {
		flagCol string
		desc    string
	}
	var entries []flagEntry

	for _, f := range ctx.Flags {
		var flagSyntax string
		if f.Shorthand != "" {
			flagSyntax = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Flag)
		} else {
			flagSyntax = fmt.Sprintf("    --%s", f.Flag)
		}

		switch f.Type {
		case libkite.ArgTypeString:
			flagSyntax += " string"
		case libkite.ArgTypeInt:
			flagSyntax += " int"
		case libkite.ArgTypeFloat:
			flagSyntax += " float"
		case libkite.ArgTypeList:
			if f.ItemType == "int" {
				flagSyntax += " ints"
			} else if f.ItemType == "float" {
				flagSyntax += " floats"
			} else {
				flagSyntax += " strings"
			}
		}

		var descParts []string
		if f.Help != "" {
			descParts = append(descParts, f.Help)
		}
		if len(f.Choices) > 0 {
			descParts = append(descParts, fmt.Sprintf("(choices: %s)", strings.Join(f.Choices, ", ")))
		}
		if f.Min != nil && f.Max != nil {
			if f.Type == libkite.ArgTypeInt {
				descParts = append(descParts, fmt.Sprintf("(range: %d..%d)", int64(*f.Min), int64(*f.Max)))
			} else {
				descParts = append(descParts, fmt.Sprintf("(range: %v..%v)", *f.Min, *f.Max))
			}
		}

		// Defaults
		if f.Default != nil && f.Default != starlark.None {
			switch f.Type {
			case libkite.ArgTypeString:
				descParts = append(descParts, fmt.Sprintf("(default: %q)", string(f.Default.(starlark.String))))
			case libkite.ArgTypeInt:
				if !f.Required {
					descParts = append(descParts, fmt.Sprintf("(default: %s)", f.Default))
				}
			case libkite.ArgTypeFloat:
				if !f.Required {
					descParts = append(descParts, fmt.Sprintf("(default: %s)", f.Default))
				}
			case libkite.ArgTypeList:
				if list, ok := f.Default.(*starlark.List); ok && list.Len() > 0 {
					descParts = append(descParts, fmt.Sprintf("(default: %s)", list))
				}
			}
		}

		entries = append(entries, flagEntry{
			flagCol: flagSyntax,
			desc:    strings.Join(descParts, " "),
		})
	}

	// Standard help entry
	entries = append(entries, flagEntry{
		flagCol: "-h, --help",
		desc:    fmt.Sprintf("Show help for %s", baseName),
	})

	maxFlagWidth := 0
	for _, e := range entries {
		if len(e.flagCol) > maxFlagWidth {
			maxFlagWidth = len(e.flagCol)
		}
	}
	if maxFlagWidth < 26 {
		maxFlagWidth = 26
	}

	for _, e := range entries {
		sb.WriteString("  ")
		sb.WriteString(e.flagCol)
		if pad := maxFlagWidth - len(e.flagCol); pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
		sb.WriteString("  ")
		sb.WriteString(e.desc)
		sb.WriteString("\n")
	}

	return sb.String()
}
