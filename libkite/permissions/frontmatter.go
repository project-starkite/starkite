package permissions

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// frontmatterScanLimit is the maximum number of leading lines scanned when
// looking for a `# permissions:` directive. Past this limit we assume the
// script header is over and stop.
const frontmatterScanLimit = 32

// ParseFrontmatterPermissions reads up to frontmatterScanLimit leading lines
// of a script file and returns the value of a `# permissions: <value>`
// directive if present. Returns ("", nil) when no directive is found and
// ("", err) only on I/O errors (a malformed directive isn't an error here;
// validation happens when LoadProfile runs on the value).
//
// Recognized forms (whitespace-tolerant after the leading comment marker):
//
//	# permissions: strict
//	# permissions:strict
//	#permissions: strict
//
// The shebang line (`#!...`) is skipped if present. Scanning stops at the
// first non-comment, non-blank line.
func ParseFrontmatterPermissions(scriptPath string) (string, error) {
	f, err := os.Open(scriptPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < frontmatterScanLimit && scanner.Scan(); i++ {
		line := strings.TrimRight(scanner.Text(), " \t\r")

		// Skip shebang on the first line.
		if i == 0 && strings.HasPrefix(line, "#!") {
			continue
		}

		// Blank lines are allowed within the header.
		if line == "" {
			continue
		}

		// Stop at the first non-comment line.
		if !strings.HasPrefix(line, "#") {
			break
		}

		// Strip the leading '#' and any space, then look for "permissions:".
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		const key = "permissions:"
		if !strings.HasPrefix(body, key) {
			continue
		}
		value := strings.TrimSpace(body[len(key):])
		return value, nil
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return "", err
	}
	return "", nil
}
