package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.starlark.net/syntax"
)

// Options configures linter execution.
type Options struct {
	EnableDeprecations bool
	EnableSandboxHints bool
	EnableLoadCheck    bool
	EnableConventions  bool
	CallerDir          string
	KnownModules       map[string]bool
}

// DefaultOptions returns standard linter configuration with all rules enabled.
func DefaultOptions(callerDir string) Options {
	return Options{
		EnableDeprecations: true,
		EnableSandboxHints: true,
		EnableLoadCheck:    true,
		EnableConventions:  true,
		CallerDir:          callerDir,
		KnownModules: map[string]bool{
			"ai": true, "args": true, "base64": true, "concur": true, "containers": true,
			"crypto": true, "csv": true, "env": true, "fleet": true, "fmt": true,
			"fs": true, "gzip": true, "hash": true, "http": true, "inventory": true,
			"io": true, "json": true, "k8s": true, "log": true, "math": true,
			"mcp": true, "os": true, "regexp": true, "retry": true, "runtime": true,
			"semver": true, "sql": true, "ssh": true, "strings": true, "table": true,
			"template": true, "test": true, "time": true, "uuid": true, "vars": true,
			"yaml": true, "zip": true,
		},
	}
}

// Lint inspects a parsed Starlark file and returns all diagnostic findings.
func Lint(file *syntax.File, filename string, opts Options) []Diagnostic {
	if file == nil {
		return nil
	}

	if opts.CallerDir == "" && filename != "" {
		opts.CallerDir = filepath.Dir(filename)
	}

	var diags []Diagnostic

	if opts.EnableConventions {
		diags = append(diags, checkTopLevelMain(file, filename)...)
		diags = append(diags, checkTryResults(file, filename)...)
	}

	if opts.EnableDeprecations || opts.EnableSandboxHints {
		diags = append(diags, checkCallsAndAttrs(file, filename, opts)...)
	}

	if opts.EnableLoadCheck {
		diags = append(diags, checkLoads(file, filename, opts)...)
	}

	return diags
}

// checkTopLevelMain implements L001.
func checkTopLevelMain(file *syntax.File, filename string) []Diagnostic {
	var diags []Diagnostic
	definesMain := false
	var mainCalls []*syntax.CallExpr

	for _, stmt := range file.Stmts {
		switch s := stmt.(type) {
		case *syntax.DefStmt:
			if s.Name != nil && s.Name.Name == "main" {
				definesMain = true
			}
		case *syntax.ExprStmt:
			if call, ok := s.X.(*syntax.CallExpr); ok {
				if ident, ok := call.Fn.(*syntax.Ident); ok && ident.Name == "main" {
					mainCalls = append(mainCalls, call)
				}
			}
		}
	}

	if definesMain {
		for _, call := range mainCalls {
			diags = append(diags, Diagnostic{
				File:     filename,
				Range:    SpanFromNode(call),
				Severity: SeverityHint,
				Code:     CodeTopLevelMain,
				Source:   "kite",
				Message:  "the runtime detects this top-level call and skips its automatic main() invocation",
			})
		}
	}
	return diags
}

// checkTryResults implements L002.
func checkTryResults(file *syntax.File, filename string) []Diagnostic {
	var diags []Diagnostic

	syntax.Walk(file, func(n syntax.Node) bool {
		switch s := n.(type) {
		case *syntax.ExprStmt:
			if call, ok := s.X.(*syntax.CallExpr); ok && isTryCall(call) {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(call),
					Severity: SeverityWarning,
					Code:     CodeUncheckedTry,
					Source:   "kite",
					Message:  "result of try_ call is discarded: check res.ok or call the plain variant to raise on failure",
				})
			}
		case *syntax.AssignStmt:
			if s.Op != syntax.EQ {
				return true
			}
			call, ok := s.RHS.(*syntax.CallExpr)
			if !ok || !isTryCall(call) {
				return true
			}
			target, ok := s.LHS.(*syntax.Ident)
			if !ok {
				return true
			}
			if !resultIsChecked(file, target) {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(s.LHS),
					Severity: SeverityWarning,
					Code:     CodeUncheckedTry,
					Source:   "kite",
					Message: fmt.Sprintf("a try_ call returns a Result: read %s.ok before %s.value, or call the plain variant to raise on failure",
						target.Name, target.Name),
				})
			}
		}
		return true
	})

	return diags
}

func isTryCall(call *syntax.CallExpr) bool {
	switch fn := call.Fn.(type) {
	case *syntax.Ident:
		return hasTryPrefix(fn.Name)
	case *syntax.DotExpr:
		return fn.Name != nil && hasTryPrefix(fn.Name.Name)
	}
	return false
}

func hasTryPrefix(name string) bool {
	return len(name) > 4 && strings.HasPrefix(name, "try_")
}

func resultIsChecked(file *syntax.File, target *syntax.Ident) bool {
	checked := false
	syntax.Walk(file, func(n syntax.Node) bool {
		if checked {
			return false
		}
		dot, ok := n.(*syntax.DotExpr)
		if !ok || dot.Name == nil {
			return true
		}
		ident, ok := dot.X.(*syntax.Ident)
		if !ok || ident.Name != target.Name {
			return true
		}
		switch dot.Name.Name {
		case "ok", "error", "err":
			checked = true
			return false
		}
		return true
	})
	return checked
}

// checkCallsAndAttrs implements L003 and L004.
func checkCallsAndAttrs(file *syntax.File, filename string, opts Options) []Diagnostic {
	var diags []Diagnostic

	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}

		dot, isDot := call.Fn.(*syntax.DotExpr)
		if !isDot || dot.Name == nil {
			return true
		}

		receiver, isIdent := dot.X.(*syntax.Ident)
		if !isIdent {
			return true
		}

		owner := receiver.Name
		member := dot.Name.Name

		// L003: Deprecations
		if opts.EnableDeprecations {
			if msg, deprecated := DeprecatedMessage(owner, member); deprecated {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(call.Fn),
					Severity: SeverityWarning,
					Code:     CodeDeprecatedSymbol,
					Source:   "kite",
					Message:  msg,
					Target:   owner + "." + member,
				})
			}
		}

		// L004: Sandbox Policy Hints
		if opts.EnableSandboxHints {
			// Subprocess execution check
			if owner == "os" && (member == "exec" || member == "try_exec" || member == "sh" || member == "try_sh") {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(call),
					Severity: SeverityHint,
					Code:     CodeSandboxPolicy,
					Source:   "kite",
					Message:  fmt.Sprintf("operation '%s.%s' executes external binaries and requires sandbox execution permissions (--sandbox-exec-allow)", owner, member),
				})
			}

			// Privileged port binding check (http.server)
			if owner == "http" && member == "server" {
				for _, arg := range call.Args {
					if b, ok := arg.(*syntax.BinaryExpr); ok && b.Op == syntax.EQ {
						if k, ok := b.X.(*syntax.Ident); ok && k.Name == "port" {
							if lit, ok := b.Y.(*syntax.Literal); ok {
								if port, err := strconv.Atoi(fmt.Sprintf("%v", lit.Value)); err == nil && port > 0 && port < 1024 {
									diags = append(diags, Diagnostic{
										File:     filename,
										Range:    SpanFromNode(arg),
										Severity: SeverityWarning,
										Code:     CodeSandboxPolicy,
										Source:   "kite",
										Message:  fmt.Sprintf("binding port %d requires root privilege; standard unprivileged execution should use ports >= 1024", port),
									})
								}
							}
						}
					}
				}
			}

			// Sensitive filesystem path check
			if owner == "fs" && (member == "write" || member == "remove" || member == "path") {
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*syntax.Literal); ok {
						if p, ok := lit.Value.(string); ok && IsSensitivePath(p) {
							diags = append(diags, Diagnostic{
								File:     filename,
								Range:    SpanFromNode(lit),
								Severity: SeverityWarning,
								Code:     CodeSandboxPolicy,
								Source:   "kite",
								Message:  fmt.Sprintf("mutating path %q outside workspace requires write permission (--sandbox-path-write)", p),
							})
						}
					}
				}
			}
		}

		return true
	})

	return diags
}

// checkLoads implements L005.
func checkLoads(file *syntax.File, filename string, opts Options) []Diagnostic {
	var diags []Diagnostic
	callerDir := opts.CallerDir
	if callerDir == "" && filename != "" {
		callerDir = filepath.Dir(filename)
	}

	for _, stmt := range file.Stmts {
		loadStmt, ok := stmt.(*syntax.LoadStmt)
		if !ok || loadStmt.Module == nil {
			continue
		}

		target := loadStmt.ModuleName()

		// 1. Check for bare .star file without path prefix
		if strings.HasSuffix(target, ".star") && !strings.HasPrefix(target, "./") && !strings.HasPrefix(target, "../") && !filepath.IsAbs(target) {
			diags = append(diags, Diagnostic{
				File:     filename,
				Range:    SpanFromNode(loadStmt.Module),
				Severity: SeverityError,
				Code:     CodeUnresolvedLoad,
				Source:   "kite",
				Message:  fmt.Sprintf("bare .star name %q is invalid: use relative path prefix %q", target, "./"+target),
			})
			continue
		}

		// 2. Relative file import check
		if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") || filepath.IsAbs(target) {
			resolvedPath := target
			if !filepath.IsAbs(resolvedPath) && callerDir != "" {
				resolvedPath = filepath.Join(callerDir, resolvedPath)
			}
			if !strings.HasSuffix(resolvedPath, ".star") {
				resolvedPath += ".star"
			}

			info, err := os.Stat(resolvedPath)
			if err != nil || info.IsDir() {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(loadStmt.Module),
					Severity: SeverityError,
					Code:     CodeUnresolvedLoad,
					Source:   "kite",
					Message:  fmt.Sprintf("cannot resolve load target %q: file not found on disk", target),
				})
			}
			continue
		}

		// 3. Built-in module vs installed module check
		if !strings.Contains(target, "/") {
			// Bare name that isn't a known module namespace
			if opts.KnownModules != nil && !opts.KnownModules[target] {
				diags = append(diags, Diagnostic{
					File:     filename,
					Range:    SpanFromNode(loadStmt.Module),
					Severity: SeverityWarning,
					Code:     CodeUnresolvedLoad,
					Source:   "kite",
					Message:  fmt.Sprintf("unknown module %q: built-in modules do not require load()", target),
				})
			}
		}
	}

	return diags
}
