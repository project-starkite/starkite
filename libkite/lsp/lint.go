package lsp

import (
	"fmt"
	"path/filepath"

	"github.com/M31-Labs/starlsp"
	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite/lint"
)

// conventionLints executes dialect convention, security, and static analysis lints
// against the parsed Starlark document.
func (h *Host) conventionLints(doc *starlsp.Document, file *syntax.File) []starlsp.Diagnostic {
	if file == nil || doc == nil {
		return nil
	}

	docPath := doc.Path()
	callerDir := filepath.Dir(docPath)
	opts := lint.DefaultOptions(callerDir)

	findings := lint.Lint(file, docPath, opts)
	if len(findings) == 0 {
		return nil
	}

	out := make([]starlsp.Diagnostic, len(findings))
	for i, f := range findings {
		startLine := f.Range.Start.Line - 1
		if startLine < 0 {
			startLine = 0
		}
		startChar := f.Range.Start.Column - 1
		if startChar < 0 {
			startChar = 0
		}
		endLine := f.Range.End.Line - 1
		if endLine < 0 {
			endLine = 0
		}
		endChar := f.Range.End.Column - 1
		if endChar < 0 {
			endChar = 0
		}

		out[i] = starlsp.Diagnostic{
			Range: starlsp.Range{
				Start: starlsp.Position{Line: startLine, Character: startChar},
				End:   starlsp.Position{Line: endLine, Character: endChar},
			},
			Severity: toLspSeverity(f.Severity),
			Source:   f.Source,
			Message:  fmt.Sprintf("[%s] %s", f.Code, f.Message),
		}
	}
	return out
}

func toLspSeverity(s lint.Severity) starlsp.DiagnosticSeverity {
	switch s {
	case lint.SeverityError:
		return starlsp.SeverityError
	case lint.SeverityWarning:
		return starlsp.SeverityWarning
	case lint.SeverityInformation:
		return starlsp.SeverityInformation
	case lint.SeverityHint:
		return starlsp.SeverityHint
	default:
		return starlsp.SeverityInformation
	}
}
