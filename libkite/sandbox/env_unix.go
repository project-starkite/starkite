//go:build !windows

package sandbox

import (
	"runtime"
)

func lookupEnv(m map[string]string, key string) (string, bool) {
	val, ok := m[key]
	return val, ok
}

func applyOSBaseline(env map[string]string, hostMap map[string]string) {
	if pathVal, ok := hostMap["PATH"]; ok && pathVal != "" {
		env["PATH"] = pathVal
	} else if runtime.GOOS == "darwin" {
		env["PATH"] = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"
	} else {
		env["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}

	env["HOME"] = "/tmp"
	env["TMPDIR"] = "/tmp"
	env["XDG_CACHE_HOME"] = "/tmp/.cache"
	env["XDG_CONFIG_HOME"] = "/tmp/.config"

	// Preserve Windows variables if present in test mocks
	for _, wk := range []string{"SYSTEMROOT", "SystemRoot", "COMSPEC", "ComSpec", "WINDIR", "windir", "PATHEXT", "pathext"} {
		if val, ok := hostMap[wk]; ok && val != "" {
			env[wk] = val
		}
	}
}
