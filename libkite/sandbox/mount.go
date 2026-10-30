package sandbox

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var mountStat = os.Stat

// ParseMountFlag parses a CLI --sandbox-mount specification string into a Mount struct.
//
// Supported formats:
//   - tmpfs:<destination>[:<mode>]
//   - <source>:<destination>[:<mode>]
//   - <source>[:<mode>] (identity mount)
//
// Relative paths are resolved against cwd; "~" and "$HOME" are expanded against home.
func ParseMountFlag(spec, cwd, home string) (Mount, error) {
	if spec == "" {
		return Mount{}, fmt.Errorf("empty mount specification")
	}

	if strings.HasPrefix(spec, "tmpfs:") {
		rem := spec[len("tmpfs:"):]
		if rem == "" {
			return Mount{}, fmt.Errorf("empty tmpfs destination in mount spec %q", spec)
		}
		tokens, err := splitMountTokens(rem)
		if err != nil {
			return Mount{}, err
		}
		if len(tokens) == 0 || tokens[0] == "" {
			return Mount{}, fmt.Errorf("empty tmpfs destination in mount spec %q", spec)
		}
		if len(tokens) > 2 {
			return Mount{}, fmt.Errorf("invalid mount specification %q: too many colons", spec)
		}
		dstRaw := tokens[0]
		dst := expandPath(dstRaw, cwd, home)
		if dst == "" {
			return Mount{}, fmt.Errorf("destination is required for tmpfs mount")
		}
		if !isMountAbs(dst) {
			return Mount{}, fmt.Errorf("destination %q must be an absolute path", dstRaw)
		}
		if strings.ContainsAny(stripDriveLetter(dst), ":\"\n\r\x00") {
			return Mount{}, fmt.Errorf("destination %q contains invalid characters", dst)
		}

		mode := MountRW
		if len(tokens) > 1 {
			switch tokens[1] {
			case string(MountRW):
				mode = MountRW
			case string(MountRO):
				mode = MountRO
			default:
				return Mount{}, fmt.Errorf("invalid mount mode %q (want %q or %q)", tokens[1], MountRO, MountRW)
			}
		}
		return Mount{
			Destination: cleanDestPath(dst),
			Type:        MountTmpfs,
			Mode:        mode,
		}, nil
	}

	tokens, err := splitMountTokens(spec)
	if err != nil {
		return Mount{}, err
	}

	var srcRaw, dstRaw string
	mode := MountRO

	switch len(tokens) {
	case 1:
		srcRaw = tokens[0]
		dstRaw = tokens[0]
	case 2:
		if tokens[1] == string(MountRO) || tokens[1] == string(MountRW) {
			srcRaw = tokens[0]
			dstRaw = tokens[0]
			mode = MountMode(tokens[1])
		} else {
			srcRaw = tokens[0]
			dstRaw = tokens[1]
		}
	case 3:
		srcRaw = tokens[0]
		dstRaw = tokens[1]
		switch tokens[2] {
		case string(MountRO):
			mode = MountRO
		case string(MountRW):
			mode = MountRW
		default:
			return Mount{}, fmt.Errorf("invalid mount mode %q (want %q or %q)", tokens[2], MountRO, MountRW)
		}
	default:
		return Mount{}, fmt.Errorf("invalid mount specification %q: too many colons", spec)
	}

	if srcRaw == "" {
		return Mount{}, fmt.Errorf("source is required for bind mount")
	}
	if dstRaw == "" {
		return Mount{}, fmt.Errorf("destination is required for bind mount")
	}

	src := expandPath(srcRaw, cwd, home)
	if cwd != "" && !isMountAbs(src) && !strings.HasPrefix(srcRaw, "~") && !strings.HasPrefix(srcRaw, "$") {
		src = filepath.Join(cwd, src)
	}
	var dst string
	if dstRaw == srcRaw {
		dst = src
	} else {
		dst = expandPath(dstRaw, cwd, home)
	}

	if !isMountAbs(src) {
		return Mount{}, fmt.Errorf("source %q must be an absolute path", srcRaw)
	}
	if !isMountAbs(dst) {
		return Mount{}, fmt.Errorf("destination %q must be an absolute path", dstRaw)
	}

	if strings.ContainsAny(stripDriveLetter(src), ":\"\n\r\x00") {
		return Mount{}, fmt.Errorf("source %q contains invalid characters", src)
	}
	if strings.ContainsAny(stripDriveLetter(dst), ":\"\n\r\x00") {
		return Mount{}, fmt.Errorf("destination %q contains invalid characters", dst)
	}

	if _, err := mountStat(src); err != nil {
		return Mount{}, fmt.Errorf("bind source %q does not exist on the host", src)
	}

	return Mount{
		Source:      filepath.Clean(src),
		Destination: cleanDestPath(dst),
		Type:        MountBind,
		Mode:        mode,
	}, nil
}

func splitMountTokens(spec string) ([]string, error) {
	var tokens []string
	var cur strings.Builder

	for i := 0; i < len(spec); i++ {
		ch := spec[i]
		if ch == ':' {
			isDriveLetter := false
			if cur.Len() == 1 && isLetter(spec[i-1]) && (i+1 == len(spec) || spec[i+1] == '/' || spec[i+1] == '\\') {
				isDriveLetter = true
			}
			if isDriveLetter {
				cur.WriteByte(ch)
				continue
			}
			tokens = append(tokens, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	tokens = append(tokens, cur.String())
	return tokens, nil
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isMountAbs(p string) bool {
	if isHostAbs(p) || isContainerAbs(p) {
		return true
	}
	if len(p) >= 2 && isLetter(p[0]) && p[1] == ':' {
		if len(p) == 2 || p[2] == '\\' || p[2] == '/' {
			return true
		}
	}
	return false
}

func cleanDestPath(dst string) string {
	if strings.HasPrefix(dst, "/") && !strings.Contains(dst, "\\") {
		return path.Clean(dst)
	}
	return filepath.Clean(dst)
}
