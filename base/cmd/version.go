package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/base/version"
)

var (
	versionShort bool
	versionJSON  bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information including commit hash, build time, and Go version.`,
	Run: func(cmd *cobra.Command, args []string) {
		if versionShort {
			fmt.Println(version.Version)
			return
		}

		if versionJSON {
			info := map[string]string{
				"version": version.Version,
				"edition": version.EditionName(),
				"commit":  version.GitCommit,
				"built":   version.BuildTime,
				"go":      runtime.Version(),
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("kite version %s (%s)\n", version.Version, version.EditionName())
		fmt.Printf("  edition: %s\n", version.EditionName())
		fmt.Printf("  commit:  %s\n", version.GitCommit)
		fmt.Printf("  built:   %s\n", version.BuildTime)
		fmt.Printf("  go:      %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionShort, "short", false, "Print version number only")
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Print version as JSON")
	rootCmd.AddCommand(versionCmd)
}
