package exec

import (
	"errors"
	"strings"
	"unicode"
)

var (
	// ErrUnclosedSingleQuote is returned when a command string terminates before closing a single quote.
	ErrUnclosedSingleQuote = errors.New("exec: unclosed single quote in command string")

	// ErrUnclosedDoubleQuote is returned when a command string terminates before closing a double quote.
	ErrUnclosedDoubleQuote = errors.New("exec: unclosed double quote in command string")

	// ErrTrailingEscape is returned when a command string terminates with an unescaped trailing backslash.
	ErrTrailingEscape = errors.New("exec: trailing backslash escape in command string")
)

// Parse tokenizes a command-line string into an executable command and its argument slice.
// It handles single quotes ('...'), double quotes ("..."), and backslash escapes (\X),
// stripping outer quotes while preserving literal values without invoking a shell.
//
// If cmdStr contains only whitespace, Parse returns command == "" and args == nil.
// If an unclosed quote or dangling backslash escape is encountered, a descriptive error is returned.
func Parse(cmdStr string) (command string, args []string, err error) {
	tokens, err := Split(cmdStr)
	if err != nil {
		return "", nil, err
	}
	if len(tokens) == 0 {
		return "", nil, nil
	}
	command = tokens[0]
	if len(tokens) > 1 {
		args = tokens[1:]
	}
	return command, args, nil
}

// Split tokenizes a command-line string into discrete argument tokens.
// It respects whitespace delimiters, single quotes, double quotes, and backslash escapes.
func Split(cmdStr string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inToken := false
	inSingle := false
	inDouble := false

	runes := []rune(cmdStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inSingle {
			if r == '\'' {
				inSingle = false
			} else {
				current.WriteRune(r)
			}
			inToken = true
			continue
		}

		if inDouble {
			if r == '"' {
				inDouble = false
			} else if r == '\\' {
				// Handle escapes inside double quotes
				if i+1 < len(runes) {
					next := runes[i+1]
					if next == '"' || next == '\\' || next == '$' || next == '`' {
						current.WriteRune(next)
						i++
					} else if next == '\n' {
						// Line continuation inside double quotes
						i++
					} else {
						// Other escaped characters inside double quotes preserve the backslash
						current.WriteRune('\\')
					}
				} else {
					return nil, ErrTrailingEscape
				}
			} else {
				current.WriteRune(r)
			}
			inToken = true
			continue
		}

		// Outside quotes
		if r == '\'' {
			inSingle = true
			inToken = true
			continue
		}

		if r == '"' {
			inDouble = true
			inToken = true
			continue
		}

		if r == '\\' {
			if i+1 < len(runes) {
				if runes[i+1] == '\n' {
					// Line continuation outside quotes
					i++
					continue
				}
				current.WriteRune(runes[i+1])
				i++
				inToken = true
				continue
			}
			return nil, ErrTrailingEscape
		}

		if unicode.IsSpace(r) {
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
			continue
		}

		// Regular character outside quotes
		current.WriteRune(r)
		inToken = true
	}

	if inSingle {
		return nil, ErrUnclosedSingleQuote
	}
	if inDouble {
		return nil, ErrUnclosedDoubleQuote
	}

	if inToken {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

// Join reconstructs a command string from a command and its argument slice,
// safely quoting arguments containing whitespace or special shell characters.
func Join(command string, args []string) string {
	if command == "" && len(args) == 0 {
		return ""
	}
	var parts []string
	if command != "" {
		parts = append(parts, Quote(command))
	}
	for _, arg := range args {
		parts = append(parts, Quote(arg))
	}
	return strings.Join(parts, " ")
}

// Quote returns a double-quoted, escaped representation of s if it contains
// whitespace, quotes, or special characters; otherwise it returns s unmodified.
func Quote(s string) string {
	if s == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '\'' || r == '\\' || r == '$' || r == '`' || r == ';' || r == '&' || r == '|' || r == '<' || r == '>' || r == '*' || r == '?' || r == '~' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return s
	}

	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' || r == '$' || r == '`' {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	sb.WriteByte('"')
	return sb.String()
}
