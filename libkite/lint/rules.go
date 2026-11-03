package lint

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	CodeTopLevelMain     = "L001" // Explicit top-level main() call
	CodeUncheckedTry     = "L002" // Unchecked try_ Result
	CodeDeprecatedSymbol = "L003" // Deprecated function or module member
	CodeSandboxPolicy    = "L004" // Operation requiring explicit sandbox allowance
	CodeUnresolvedLoad   = "L005" // Unresolved Starlark load() import
)

// DeprecatedSymbols maps qualified deprecated API names to modern alternatives.
var DeprecatedSymbols = map[string]string{
	"os.system":          "use os.exec instead (or os.sh().exec)",
	"fs.read_file":       "use fs.read or fs.path.read_text instead",
	"fs.write_file":      "use fs.write or fs.path.write_text instead",
	"http.request":       "use http.get, http.post, or http.url instead",
	"k8s.apply_file":     "use k8s.apply instead",
	"strings.split":      "use Starlark native string method s.split(sep) instead",
	"strings.has_prefix": "use Starlark native string method s.startswith(prefix) instead",
	"strings.has_suffix": "use Starlark native string method s.endswith(suffix) instead",
	"strings.trim_space": "use Starlark native string method s.strip() instead",
	"strings.join":       "use Starlark native string method sep.join(list) instead",
	"strings.replace":    "use Starlark native string method s.replace(old, new) instead",
	"strings.contains":   "use 'sub in s' operator or s.find() instead",
	"strings.to_lower":   "use Starlark native string method s.lower() instead",
	"strings.to_upper":   "use Starlark native string method s.upper() instead",
}

// SensitivePaths identifies filesystem locations outside standard user workspaces.
var SensitivePaths = []string{
	"/etc", "/usr", "/bin", "/sbin", "/lib", "/var/run", "/root",
	"C:\\Windows", "C:\\Program Files", "C:\\Program Files (x86)",
}

// IsSensitivePath reports whether a file path targets sensitive system locations.
func IsSensitivePath(p string) bool {
	clean := filepath.ToSlash(filepath.Clean(p))
	for _, sp := range SensitivePaths {
		spClean := filepath.ToSlash(filepath.Clean(sp))
		if strings.EqualFold(clean, spClean) || strings.HasPrefix(strings.ToLower(clean), strings.ToLower(spClean)+"/") {
			return true
		}
	}
	return false
}

// DeprecatedMessage returns the deprecation warning message for a member.
func DeprecatedMessage(owner, member string) (string, bool) {
	key := member
	if owner != "" {
		key = owner + "." + member
	}
	if alt, ok := DeprecatedSymbols[key]; ok {
		return fmt.Sprintf("%q is deprecated: %s", key, alt), true
	}
	return "", false
}
