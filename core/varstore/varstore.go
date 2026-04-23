// Package varstore provides variable management with priority-based resolution.
// Variables can come from CLI flags, files, environment, or script defaults.
package varstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Priority levels for variable resolution (highest to lowest):
// 1. CLI flags: --var key=value
// 2. Variable files: --var-file=values.yaml
// 3. Default config: ~/.starkite/config.yaml or ./config.yaml
// 4. Environment variables: STARKITE_VAR_key
// 5. Script defaults: var("key", "default")

// Vars manages variable storage and resolution.
type Vars struct {
	mu sync.RWMutex

	// Variables from CLI --var flags (highest priority)
	cliVars map[string]interface{}

	// Variables from --var-file files
	fileVars map[string]interface{}

	// Variables from default config files
	defaultVars map[string]interface{}

	// Variables from STARKITE_VAR_* environment variables
	envVars map[string]interface{}

	// Provider-specific defaults from config
	ProviderDefaults map[string]map[string]interface{}

	// Project configuration section
	ProjectConfig map[string]interface{}

	// Runtime defaults
	RuntimeDefaults map[string]interface{}

	// Active edition from config file
	ActiveEdition string
}

// New creates a new Vars instance.
func New() *Vars {
	return &Vars{
		cliVars:          make(map[string]interface{}),
		fileVars:         make(map[string]interface{}),
		defaultVars:      make(map[string]interface{}),
		envVars:          make(map[string]interface{}),
		ProviderDefaults: make(map[string]map[string]interface{}),
		ProjectConfig:    make(map[string]interface{}),
		RuntimeDefaults:  make(map[string]interface{}),
	}
}

// LoadDefaults loads variables from default config files.
// Checks ~/.starkite/config.yaml and ./config.yaml
func (v *Vars) LoadDefaults() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Try user config first
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userConfig := filepath.Join(homeDir, ".starkite", "config.yaml")
		if data, err := os.ReadFile(userConfig); err == nil {
			if err := v.parseConfigFile(data); err != nil {
				return fmt.Errorf("failed to parse %s: %w", userConfig, err)
			}
		}
	}

	// Try local config (overrides user config)
	if data, err := os.ReadFile("config.yaml"); err == nil {
		if err := v.parseConfigFile(data); err != nil {
			return fmt.Errorf("failed to parse config.yaml: %w", err)
		}
	}

	return nil
}

// parseConfigFile parses a YAML config file and populates appropriate sections.
func (v *Vars) parseConfigFile(data []byte) error {
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	for key, value := range config {
		switch key {
		case "project":
			if m, ok := value.(map[string]interface{}); ok {
				v.ProjectConfig = m
			}
		case "defaults":
			if m, ok := value.(map[string]interface{}); ok {
				v.RuntimeDefaults = m
			}
		case "providers":
			if m, ok := value.(map[string]interface{}); ok {
				for pname, pconfig := range m {
					if pc, ok := pconfig.(map[string]interface{}); ok {
						v.ProviderDefaults[pname] = pc
					}
				}
			}
		case "active_edition":
			if s, ok := value.(string); ok {
				v.ActiveEdition = s
			}
		default:
			// Flatten nested maps with dot notation
			v.flattenAndStore(key, value, v.defaultVars)
		}
	}

	return nil
}

// flattenAndStore flattens nested maps into dot-notation keys.
// Maps are preserved at the prefix key before recursing into children,
// so both var_dict("labels") and var("labels.app") work.
func (v *Vars) flattenAndStore(prefix string, value interface{}, store map[string]interface{}) {
	switch val := value.(type) {
	case map[string]interface{}:
		store[prefix] = value // preserve the unflattened map
		for k, v2 := range val {
			newKey := prefix + "." + k
			v.flattenAndStore(newKey, v2, store)
		}
	default:
		store[prefix] = value
	}
}

// LoadFromCLI parses CLI --var flags.
// Format: key=value
func (v *Vars) LoadFromCLI(vars []string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, kv := range vars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid var format: %s (expected key=value)", kv)
		}
		v.cliVars[parts[0]] = tryParseJSON(parts[1])
	}
	return nil
}

// LoadFromFile loads variables from a YAML file.
func (v *Vars) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read var file %s: %w", path, err)
	}

	var vars map[string]interface{}
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return fmt.Errorf("failed to parse var file %s: %w", path, err)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	for key, value := range vars {
		v.flattenAndStore(key, value, v.fileVars)
	}

	return nil
}

// LoadFromFiles loads variables from multiple YAML files.
func (v *Vars) LoadFromFiles(paths []string) error {
	for _, path := range paths {
		if err := v.LoadFromFile(path); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromEnv loads variables from STARKITE_VAR_* environment variables.
func (v *Vars) LoadFromEnv() {
	v.mu.Lock()
	defer v.mu.Unlock()

	const prefix = "STARKITE_VAR_"
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.HasPrefix(parts[0], prefix) {
			// STARKITE_VAR_DATABASE_HOST -> database.host
			key := strings.ToLower(strings.TrimPrefix(parts[0], prefix))
			key = strings.ReplaceAll(key, "_", ".")
			v.envVars[key] = parts[1]
		}
	}
}

// Get retrieves a variable value by key with priority resolution.
func (v *Vars) Get(key string) (interface{}, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Priority order (highest to lowest)
	if val, ok := v.cliVars[key]; ok {
		return val, true
	}
	if val, ok := v.fileVars[key]; ok {
		return val, true
	}
	if val, ok := v.defaultVars[key]; ok {
		return val, true
	}
	if val, ok := v.envVars[key]; ok {
		return val, true
	}

	return nil, false
}

// GetWithDefault retrieves a variable value with a default fallback.
func (v *Vars) GetWithDefault(key string, defaultValue interface{}) interface{} {
	if val, ok := v.Get(key); ok {
		return val
	}
	return defaultValue
}

// GetString retrieves a variable value as a string.
func (v *Vars) GetString(key string) string {
	if val, ok := v.Get(key); ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// MustGet retrieves a variable value or returns an error if not found.
func (v *Vars) MustGet(key string) (interface{}, error) {
	if val, ok := v.Get(key); ok {
		return val, nil
	}
	return nil, fmt.Errorf("variable not found: %s", key)
}

// Set sets a variable value at CLI priority (highest).
func (v *Vars) Set(key string, value interface{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cliVars[key] = value
}

// All returns all variables merged by priority.
func (v *Vars) All() map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	result := make(map[string]interface{})

	// Add in reverse priority order (lowest first, so higher priority overwrites)
	for k, val := range v.envVars {
		result[k] = val
	}
	for k, val := range v.defaultVars {
		result[k] = val
	}
	for k, val := range v.fileVars {
		result[k] = val
	}
	for k, val := range v.cliVars {
		result[k] = val
	}

	return result
}

// Keys returns sorted, deduplicated variable names across all priority tiers.
func (v *Vars) Keys() []string {
	all := v.All()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// tryParseJSON attempts to parse a string as JSON if it starts with [ or {.
// Returns the parsed value on success, or the original string on failure.
func tryParseJSON(s string) interface{} {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	if s[0] == '[' || s[0] == '{' {
		var parsed interface{}
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
	}
	return s
}

// GetProviderDefaults returns defaults for a specific provider.
func (v *Vars) GetProviderDefaults(provider string) map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if defaults, ok := v.ProviderDefaults[provider]; ok {
		result := make(map[string]interface{}, len(defaults))
		for k, val := range defaults {
			result[k] = val
		}
		return result
	}
	return nil
}
