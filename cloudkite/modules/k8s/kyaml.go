package k8s

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// checkDuplicateKeys recursively inspects a yaml.Node and rejects duplicate mapping keys.
func checkDuplicateKeys(node *yaml.Node) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := checkDuplicateKeys(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := make(map[string]int)
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			if line, ok := seen[keyNode.Value]; ok {
				return fmt.Errorf("yaml: duplicate key %q on line %d (previously declared on line %d)", keyNode.Value, keyNode.Line, line)
			}
			seen[keyNode.Value] = keyNode.Line

			if err := checkDuplicateKeys(valNode); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := checkDuplicateKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// isAmbiguousKYAMLString checks if an unquoted string would be coerced to a boolean, null, or number.
func isAmbiguousKYAMLString(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "y", "yes", "true", "on", "n", "no", "false", "off", "null", "~", "":
		return true
	}
	// Check if string parses as an integer or float
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

// sanitizeKYAMLNode ensures that strings matching ambiguous tokens are explicitly double-quoted.
func sanitizeKYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		if isAmbiguousKYAMLString(node.Value) {
			node.Style = yaml.DoubleQuotedStyle
		}
	}

	for _, child := range node.Content {
		sanitizeKYAMLNode(child)
	}
}

// encodeKYAML converts a Go value into strict, unambiguous KYAML bytes.
func encodeKYAML(val any) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(val); err != nil {
		return nil, err
	}

	sanitizeKYAMLNode(&node)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
