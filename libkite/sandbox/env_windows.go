//go:build windows

package sandbox

import (
	"os"
	"strings"
)

func lookupEnv(m map[string]string, key string) (string, bool) {
	if val, ok := m[key]; ok {
		return val, true
	}
	lowerKey := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lowerKey {
			return v, true
		}
	}
	return "", false
}

func applyOSBaseline(env map[string]string, hostMap map[string]string) {
	if pathVal, ok := lookupEnv(hostMap, "PATH"); ok && pathVal != "" {
		env["PATH"] = pathVal
	} else {
		env["PATH"] = `C:\Windows\system32;C:\Windows;C:\Windows\System32\Wbem`
	}

	tempDir := os.TempDir()
	if val, ok := lookupEnv(hostMap, "TEMP"); ok && val != "" {
		tempDir = val
	} else if val, ok := lookupEnv(hostMap, "TMP"); ok && val != "" {
		tempDir = val
	}
	env["TEMP"] = tempDir
	env["TMP"] = tempDir
	env["HOME"] = tempDir

	if val, ok := lookupEnv(hostMap, "USERPROFILE"); ok && val != "" {
		env["USERPROFILE"] = val
	} else {
		env["USERPROFILE"] = tempDir
	}

	if val, ok := lookupEnv(hostMap, "LOCALAPPDATA"); ok && val != "" {
		env["LOCALAPPDATA"] = val
	}
	if val, ok := lookupEnv(hostMap, "APPDATA"); ok && val != "" {
		env["APPDATA"] = val
	}

	if val, ok := lookupEnv(hostMap, "SystemDrive"); ok && val != "" {
		env["SystemDrive"] = val
	} else {
		env["SystemDrive"] = "C:"
	}

	for _, wk := range []string{"SYSTEMROOT", "SystemRoot", "COMSPEC", "ComSpec", "WINDIR", "windir", "PATHEXT", "pathext"} {
		if val, ok := lookupEnv(hostMap, wk); ok && val != "" {
			env[wk] = val
		}
	}

	if env["SYSTEMROOT"] == "" && env["SystemRoot"] == "" {
		env["SystemRoot"] = `C:\Windows`
		env["SYSTEMROOT"] = `C:\Windows`
	}
	if env["WINDIR"] == "" && env["windir"] == "" {
		env["windir"] = `C:\Windows`
		env["WINDIR"] = `C:\Windows`
	}
	if env["COMSPEC"] == "" && env["ComSpec"] == "" {
		env["ComSpec"] = `C:\Windows\system32\cmd.exe`
		env["COMSPEC"] = `C:\Windows\system32\cmd.exe`
	}
	if env["PATHEXT"] == "" && env["pathext"] == "" {
		env["PATHEXT"] = ".COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH"
	}
}
