package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/basekite/version"
	"github.com/spf13/cobra"
)

func TestVersionCmdDefault(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	oldShort, oldJSON := versionShort, versionJSON
	versionShort, versionJSON = false, false
	defer func() {
		versionShort, versionJSON = oldShort, oldJSON
	}()

	versionCmd.Run(cmd, nil)
	out := buf.String()

	if !strings.Contains(out, "commit:") {
		t.Errorf("expected 'commit:' in output, got: %s", out)
	}
	if !strings.Contains(out, "built:") {
		t.Errorf("expected 'built:' in output, got: %s", out)
	}
	if !strings.Contains(out, "go:") {
		t.Errorf("expected 'go:' in output, got: %s", out)
	}
	if !strings.Contains(out, "os/arch:") {
		t.Errorf("expected 'os/arch:' in output, got: %s", out)
	}
}

func TestVersionCmdShort(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	oldShort, oldJSON := versionShort, versionJSON
	versionShort, versionJSON = true, false
	defer func() {
		versionShort, versionJSON = oldShort, oldJSON
	}()

	versionCmd.Run(cmd, nil)
	out := strings.TrimSpace(buf.String())

	if out != version.Version {
		t.Errorf("expected %q, got %q", version.Version, out)
	}
}

func TestVersionCmdJSON(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	oldShort, oldJSON := versionShort, versionJSON
	versionShort, versionJSON = false, true
	defer func() {
		versionShort, versionJSON = oldShort, oldJSON
	}()

	versionCmd.Run(cmd, nil)

	var data map[string]string
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("failed to parse version json: %v", err)
	}

	if data["version"] != version.Version {
		t.Errorf("json version = %q, want %q", data["version"], version.Version)
	}
	if _, ok := data["commit"]; !ok {
		t.Errorf("json missing 'commit' key")
	}
	if _, ok := data["go"]; !ok {
		t.Errorf("json missing 'go' key")
	}
}
