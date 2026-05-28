// Package version provides version information for starkite.
package version

import "fmt"

var (
	// Version is the starkite version, set at build time
	Version = "0.0.1"

	// GitCommit is the git commit hash, set at build time
	GitCommit = "unknown"

	// BuildDate is the build date, set at build time
	BuildDate = "unknown"

	// BuildTime is an alias for BuildDate (for backward compatibility)
	BuildTime = BuildDate

	// Edition is the edition name, set by cloud binary or build-time ldflags
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

// String returns the version string including edition.
func String() string {
	return fmt.Sprintf("%s (%s) (commit: %s)", Version, EditionName(), GitCommit)
}
