//go:build !windows

package osmod

func defaultShell() (string, string) {
	return "/bin/sh", "-c"
}
