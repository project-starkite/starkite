package permissions

import "testing"

// FuzzParseInline feeds arbitrary strings to the inline parser and asserts
// no panics. Successful parses must produce a config whose allow/deny entries
// are non-empty strings.
func FuzzParseInline(f *testing.F) {
	seeds := []string{
		"allow:fs.read",
		"deny:os.exec",
		"allow:fs.read;deny:os.exec",
		"allow:fs.read deny:os.exec",
		"allow: fs.read , http.client ",
		"allow:",
		"allow:,",
		"allow",
		"",
		"allow:fs.read;deny:",
		"allow:fs.read(*)",
		"allow:fs.read(read_file:*),deny:fs.write",
		"allow:fs.read;allow:os.env",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		cfg, err := parseInline(input)
		if err != nil {
			return // many random inputs are expected to fail
		}
		for i, r := range cfg.Allow {
			if r == "" {
				t.Errorf("parseInline(%q).Allow[%d] is empty", input, i)
			}
		}
		for i, r := range cfg.Deny {
			if r == "" {
				t.Errorf("parseInline(%q).Deny[%d] is empty", input, i)
			}
		}
	})
}

// FuzzLoadProfile runs LoadProfile against arbitrary values to verify no
// panics across the resolution paths (built-in, inline, file, named).
func FuzzLoadProfile(f *testing.F) {
	seeds := []string{
		"allow-all",
		"strict",
		"deny-all",
		"allow:fs.read",
		"./team.yaml",
		"team.yaml#dev",
		"team",
		"",
		"#fragment",
		"some.name",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = LoadProfile(input)
	})
}
