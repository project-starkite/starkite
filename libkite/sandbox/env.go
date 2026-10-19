package sandbox

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// HostEnvToMap converts a slice of "KEY=VALUE" environment strings to a map.
func HostEnvToMap(environ []string) map[string]string {
	res := make(map[string]string, len(environ))
	for _, e := range environ {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			res[k] = v
		}
	}
	return res
}

// SanitizedBaselineEnv constructs the zero-trust hermetic baseline environment
// table from the provided host environment. Ambient credentials, cloud tokens,
// and host user directories are scrubbed, retaining only minimal platform essentials.
func SanitizedBaselineEnv(hostEnv []string) map[string]string {
	hostMap := HostEnvToMap(hostEnv)
	env := make(map[string]string)

	// Apply OS-specific baseline environment variables (PATH, scratch/temp, system essentials)
	applyOSBaseline(env, hostMap)

	// Locale and Terminal
	if val, ok := lookupEnv(hostMap, "LANG"); ok && val != "" {
		env["LANG"] = val
	} else {
		env["LANG"] = "C.UTF-8"
	}
	if val, ok := lookupEnv(hostMap, "LC_ALL"); ok && val != "" {
		env["LC_ALL"] = val
	} else {
		env["LC_ALL"] = "C.UTF-8"
	}
	if val, ok := lookupEnv(hostMap, "TERM"); ok && val != "" {
		env["TERM"] = val
	} else {
		env["TERM"] = "xterm-256color"
	}
	if val, ok := lookupEnv(hostMap, "TZ"); ok && val != "" {
		env["TZ"] = val
	}

	// Control environment variable for recursion prevention
	if val, ok := hostMap[InsideEnvVar]; ok && val != "" {
		env[InsideEnvVar] = val
	}

	return env
}

// ParseCLIEnvFlags parses repeated --env / -e flags into a map of variables.
// Bare flags (--env KEY or -e KEY) forward the variable from hostMap (or "" if unset).
// Explicit flags (--env KEY=VAL or -e KEY=VAL) set KEY to VAL literally.
func ParseCLIEnvFlags(flags []string, hostMap map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	for _, flag := range flags {
		key, val, hasEq := strings.Cut(flag, "=")
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || strings.ContainsAny(trimmedKey, " \t\n\r") || trimmedKey != key {
			return nil, fmt.Errorf("invalid environment variable key %q in --env flag", key)
		}
		if !hasEq {
			// Bare forwarding
			if v, ok := lookupEnv(hostMap, key); ok {
				result[key] = v
			} else {
				result[key] = ""
			}
		} else {
			// Explicit assignment
			result[key] = val
		}
	}
	return result, nil
}

// SynthesizeEnvMap merges Tier 1 (Baseline), Tier 2 (Profile config), and Tier 3 (CLI flags).
func (p *Profile) SynthesizeEnvMap(hostEnv []string, cliEnvFlags []string, cliEnvInherit bool) (map[string]string, error) {
	hostMap := HostEnvToMap(hostEnv)
	var envMap map[string]string

	// Tier 1: Baseline
	if (p != nil && p.Env.Inherit) || cliEnvInherit {
		envMap = maps.Clone(hostMap)
	} else {
		envMap = SanitizedBaselineEnv(hostEnv)
	}

	// Tier 2: Profile env directives
	if p != nil && len(p.Env.Vars) > 0 {
		for k, vPtr := range p.Env.Vars {
			if vPtr == nil {
				// Bare key: forward from host if present, else ""
				if hv, ok := lookupEnv(hostMap, k); ok {
					envMap[k] = hv
				} else {
					envMap[k] = ""
				}
			} else {
				// String value: expand $VAR and ${VAR} against host environment
				expanded := os.Expand(*vPtr, func(name string) string {
					v, _ := lookupEnv(hostMap, name)
					return v
				})
				envMap[k] = expanded
			}
		}
	}

	// Tier 3: CLI flags
	cliMap, err := ParseCLIEnvFlags(cliEnvFlags, hostMap)
	if err != nil {
		return nil, err
	}
	for k, v := range cliMap {
		envMap[k] = v
	}

	return envMap, nil
}

// SynthesizeEnv returns a deterministically sorted slice of "KEY=VALUE" pairs.
func (p *Profile) SynthesizeEnv(hostEnv []string, cliEnvFlags []string, cliEnvInherit bool) ([]string, error) {
	envMap, err := p.SynthesizeEnvMap(hostEnv, cliEnvFlags, cliEnvInherit)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, len(envMap))
	for k, v := range envMap {
		res = append(res, fmt.Sprintf("%s=%s", k, v))
	}
	slices.Sort(res)
	return res, nil
}
