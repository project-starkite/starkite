package lint

import (
	"fmt"

	"go.starlark.net/syntax"
)

// Severity represents the diagnostic severity level matching LSP specifications.
type Severity int

const (
	SeverityError Severity = iota + 1
	SeverityWarning
	SeverityInformation
	SeverityHint
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

func (s Severity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// Position represents a 1-based text location in a script.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Range represents a span between start and end positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Diagnostic represents an actionable compiler or convention lint finding.
type Diagnostic struct {
	File     string   `json:"file"`
	Range    Range    `json:"range"`
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
	Target   string   `json:"target,omitempty"`
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: [%s] %s (%s)",
		d.File, d.Range.Start.Line, d.Range.Start.Column, d.Severity, d.Message, d.Code)
}

// SpanFromNode calculates a Range from a Starlark AST node.
func SpanFromNode(node syntax.Node) Range {
	if node == nil {
		return Range{Start: Position{Line: 1, Column: 1}, End: Position{Line: 1, Column: 1}}
	}
	start, end := node.Span()
	return Range{
		Start: Position{Line: int(start.Line), Column: int(start.Col)},
		End:   Position{Line: int(end.Line), Column: int(end.Col)},
	}
}
