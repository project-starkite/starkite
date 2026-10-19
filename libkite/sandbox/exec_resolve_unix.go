//go:build unix

package sandbox

var defaultExecSearchDirs = []string{
	"/opt/homebrew/bin",
	"/opt/homebrew/sbin",
	"/usr/local/bin",
	"/usr/local/sbin",
	"/usr/bin",
	"/bin",
	"/usr/sbin",
	"/sbin",
}
