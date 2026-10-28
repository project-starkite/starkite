//go:build windows

package sandbox

import (
	"os"
	"path/filepath"
)

var defaultExecSearchDirs = initDefaultExecSearchDirs()

func initDefaultExecSearchDirs() []string {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = os.Getenv("windir")
	}
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}

	dirs := []string{
		filepath.Join(sysRoot, "System32"),
		sysRoot,
		filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(sysRoot, "System32", "OpenSSH"),
	}

	if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
		dirs = append(dirs, filepath.Join(localApp, "Microsoft", "WinGet", "Links"))
	}
	if userProf := os.Getenv("USERPROFILE"); userProf != "" {
		dirs = append(dirs, filepath.Join(userProf, "scoop", "shims"))
	}

	return dirs
}
