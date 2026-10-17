package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRegister(t *testing.T) {
	root := &cobra.Command{Use: "kite"}
	// Safe and idempotent
	Register(root)
	Register(root)
}
