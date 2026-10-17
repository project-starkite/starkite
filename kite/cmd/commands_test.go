package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRegister(t *testing.T) {
	root := &cobra.Command{Use: "kite"}
	// Safe and idempotent across all editions
	Register(root)
	Register(root)
}
