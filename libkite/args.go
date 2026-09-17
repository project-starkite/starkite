package libkite

import (
	"fmt"
	"strings"
	"sync"

	"go.starlark.net/starlark"
)

const scriptArgsKey = "libkite.scriptArgs"

// ArgType represents the data type of an argument or flag.
type ArgType string

const (
	ArgTypeString ArgType = "string"
	ArgTypeInt    ArgType = "int"
	ArgTypeBool   ArgType = "bool"
	ArgTypeFloat  ArgType = "float"
	ArgTypeList   ArgType = "list"
)

// FlagDef defines a declared command-line flag.
type FlagDef struct {
	Name        string         // Starlark struct attribute name (normalized e.g. "key_path")
	Flag        string         // CLI flag name (e.g. "key-path" for --key-path)
	Type        ArgType        // "string", "int", "bool", "float", "list"
	Shorthand   string         // Single-letter alias (e.g. "a" for -a)
	Default     starlark.Value // Default value (starlark.None if not set)
	Required    bool           // If true, flag must be provided
	Choices     []string       // Allowed string values (for string flags)
	Min         *float64       // Min bound (for numeric flags)
	Max         *float64       // Max bound (for numeric flags)
	Help        string         // Help description
	VarFallback string         // Fallback variable name in VarStore
	ItemType    string         // Element type for list flags ("string", "int", "float")
}

// PositionalDef defines a declared positional command-line argument.
type PositionalDef struct {
	Name     string         // Positional argument name (e.g. "cluster-name")
	Attr     string         // Normalized Starlark struct attribute name (e.g. "cluster_name")
	Required bool           // Whether this argument is required (default true)
	Default  starlark.Value // Default value (starlark.None if not set)
	Help     string         // Help description
}

// ScriptArgsContext holds command-line arguments forwarded to the script
// and records declared flag/positional argument schemas.
type ScriptArgsContext struct {
	RawArgs     []string
	ScriptPath  string
	Parsed      bool
	Result      starlark.Value
	Flags       []FlagDef
	Positionals []PositionalDef

	mu              sync.RWMutex
	flagNames       map[string]int
	attrNames       map[string]int
	shorthands      map[string]int
	positionalNames map[string]int
}

// NormalizeAttributeName converts hyphenated identifiers into valid Starlark attribute names
// (e.g. "key-path" -> "key_path").
func NormalizeAttributeName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// NormalizeFlagName determines the CLI flag name from user-provided name and optional explicit flag.
// If an explicit flag is given, leading hyphens are stripped.
// Otherwise, underscores in name are replaced with hyphens (e.g. "key_path" -> "key-path").
func NormalizeFlagName(name string, flag string) string {
	if flag != "" {
		return strings.TrimPrefix(flag, "--")
	}
	return strings.ReplaceAll(name, "_", "-")
}

// AddFlag registers a FlagDef into the context.
// Returns an error if the flag name, attribute name, or shorthand collides with an existing flag or positional argument.
func (ctx *ScriptArgsContext) AddFlag(def FlagDef) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.flagNames == nil {
		ctx.flagNames = make(map[string]int)
		ctx.attrNames = make(map[string]int)
		ctx.shorthands = make(map[string]int)
		ctx.positionalNames = make(map[string]int)
	}

	if def.Flag == "" {
		return fmt.Errorf("flag name cannot be empty")
	}
	if def.Name == "" {
		return fmt.Errorf("flag attribute name cannot be empty")
	}

	if _, exists := ctx.flagNames[def.Flag]; exists {
		return fmt.Errorf("flag %q is already defined", def.Flag)
	}
	if _, exists := ctx.attrNames[def.Name]; exists {
		return fmt.Errorf("flag attribute %q is already defined or conflicts with an existing argument", def.Name)
	}
	if def.Shorthand != "" {
		if len(def.Shorthand) != 1 {
			return fmt.Errorf("shorthand flag for %q must be a single character, got %q", def.Flag, def.Shorthand)
		}
		if _, exists := ctx.shorthands[def.Shorthand]; exists {
			return fmt.Errorf("shorthand -%s is already defined", def.Shorthand)
		}
		ctx.shorthands[def.Shorthand] = len(ctx.Flags)
	}

	ctx.flagNames[def.Flag] = len(ctx.Flags)
	ctx.attrNames[def.Name] = len(ctx.Flags)
	ctx.Flags = append(ctx.Flags, def)
	return nil
}

// AddPositional registers a PositionalDef into the context.
// Returns an error if the positional name collides, or if a required positional follows an optional one.
func (ctx *ScriptArgsContext) AddPositional(def PositionalDef) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.positionalNames == nil {
		ctx.flagNames = make(map[string]int)
		ctx.attrNames = make(map[string]int)
		ctx.shorthands = make(map[string]int)
		ctx.positionalNames = make(map[string]int)
	}

	if def.Name == "" {
		return fmt.Errorf("positional argument name cannot be empty")
	}
	if def.Attr == "" {
		def.Attr = NormalizeAttributeName(def.Name)
	}

	if _, exists := ctx.positionalNames[def.Name]; exists {
		return fmt.Errorf("positional argument %q is already defined", def.Name)
	}
	if _, exists := ctx.attrNames[def.Attr]; exists {
		return fmt.Errorf("positional argument attribute %q conflicts with an existing argument", def.Attr)
	}

	if def.Required {
		for _, p := range ctx.Positionals {
			if !p.Required {
				return fmt.Errorf("required positional argument %q cannot follow optional positional argument %q", def.Name, p.Name)
			}
		}
	}

	ctx.positionalNames[def.Name] = len(ctx.Positionals)
	ctx.attrNames[def.Attr] = len(ctx.Positionals)
	ctx.Positionals = append(ctx.Positionals, def)
	return nil
}

// SetScriptArgs stores a ScriptArgsContext in thread.Local.
func SetScriptArgs(thread *starlark.Thread, ctx *ScriptArgsContext) {
	if thread != nil {
		thread.SetLocal(scriptArgsKey, ctx)
	}
}

// GetScriptArgs retrieves the ScriptArgsContext from thread.Local.
// Returns nil if no script args context is set.
func GetScriptArgs(thread *starlark.Thread) *ScriptArgsContext {
	if thread == nil {
		return nil
	}
	ctx, _ := thread.Local(scriptArgsKey).(*ScriptArgsContext)
	return ctx
}

// EnsureScriptArgs retrieves the ScriptArgsContext from thread.Local,
// allocating and attaching a new one if none is currently present.
func EnsureScriptArgs(thread *starlark.Thread) *ScriptArgsContext {
	if thread == nil {
		return &ScriptArgsContext{}
	}
	ctx := GetScriptArgs(thread)
	if ctx == nil {
		ctx = &ScriptArgsContext{}
		SetScriptArgs(thread, ctx)
	}
	return ctx
}
