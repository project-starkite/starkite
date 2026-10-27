//go:build windows

package sandbox

import (
	"os"
	"path/filepath"
)

var platformNetworkReadPaths = func() []string {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	return []string{
		filepath.Join(sysRoot, `System32\drivers\etc\hosts`),
		filepath.Join(sysRoot, `System32\drivers\etc\services`),
		filepath.Join(sysRoot, `System32\drivers\etc\networks`),
		filepath.Join(sysRoot, `System32\drivers\etc\protocol`),
	}
}()
