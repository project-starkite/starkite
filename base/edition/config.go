package edition

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigPath returns the path to the unified starkite config file (~/.starkite/config.yaml).
func ConfigPath() (string, error) {
	starkiteDir, err := StarkiteDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(starkiteDir, "config.yaml"), nil
}

// ActiveEdition reads the active edition from config.yaml.
// Returns EditionBase if the config file doesn't exist or active_edition is unset.
func ActiveEdition() string {
	configPath, err := ConfigPath()
	if err != nil {
		return EditionBase
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return EditionBase
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return EditionBase
	}

	if active, ok := config["active_edition"]; ok {
		if s, ok := active.(string); ok && s != "" {
			return s
		}
	}

	return EditionBase
}

// SetActiveEdition updates the active_edition in config.yaml.
// It performs a careful read-modify-write to preserve all other config sections.
func SetActiveEdition(name string) error {
	configPath, err := ConfigPath()
	if err != nil {
		return fmt.Errorf("cannot determine config path: %w", err)
	}

	// Read existing config (if any)
	var config map[string]interface{}

	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}
	if config == nil {
		config = make(map[string]interface{})
	}

	// Update or remove the active_edition key
	if name == EditionBase || name == "" {
		delete(config, "active_edition")
	} else {
		config["active_edition"] = name
	}

	// Marshal and write back
	out, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
