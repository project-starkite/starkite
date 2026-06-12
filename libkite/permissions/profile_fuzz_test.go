package permissions

import "testing"

// FuzzResolve asserts Resolve never panics for arbitrary --permissions values,
// with and without config-defined profiles (including an alias).
func FuzzResolve(f *testing.F) {
	seeds := []string{
		"", "deny-all", "allow-fs", "allow-all", "default", "team",
		"allow:fs.read", "./profile.yaml", "a#b", "ALLOW-FS", "allow fs",
		"\x00", "….", "default:allow-all",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	defined := map[string]ProfileSpec{
		"default": {Alias: ProfileAllowFS},
		"team":    {Allow: []string{"fs.read"}},
	}
	f.Fuzz(func(t *testing.T, value string) {
		_, _ = Resolve(value, defined)
		_, _ = Resolve(value, nil)
	})
}
