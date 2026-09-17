//go:build windows

package osmod

import "os"

func defaultShell() (string, string) {
	comspec := os.Getenv("COMSPEC")
	if comspec != "" {
		return comspec, "/c"
	}
	return "cmd.exe", "/c"
}
