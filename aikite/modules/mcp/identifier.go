package mcp

import "unicode"

// isValidIdentifier reports whether s conforms to Starlark's identifier grammar:
// [A-Za-z_][A-Za-z0-9_]*. Empty strings and names with hyphens, dots, slashes,
// or whitespace all fail. Used to decide whether an MCP tool name is reachable
// via the `client.tools.<name>` attribute shortcut or only via `client.call`.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}
