// Package version provides version information for starkite.
package version

import "fmt"

var (
	// Version is the starkite version, set at build time.
	Version = "0.0.1"

	// GitCommit is the git commit hash, set at build time.
	GitCommit = "unknown"

	// BuildTime is the build timestamp, set at build time. The initializer must
	// be a string constant so the linker's -X flag can override it.
	BuildTime = "unknown"

	// Edition is the edition name, set by the edition's main or build-time ldflags.
	Edition = ""
)

// EditionName returns the edition name, defaulting to "base".
func EditionName() string {
	if Edition == "" || Edition == "base" {
		return "base"
	}
	return Edition
}

// IsBaseEdition returns true if this is the base edition.
func IsBaseEdition() bool {
	return Edition == "" || Edition == "base"
}

// String returns the version string. The all-in-one edition is the default and
// is not annotated; lean editions append their name.
func String() string {
	if edition := EditionName(); edition != "all" {
		return fmt.Sprintf("%s (%s) (commit: %s)", Version, edition, GitCommit)
	}
	return fmt.Sprintf("%s (commit: %s)", Version, GitCommit)
}
