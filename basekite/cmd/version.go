package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/project-starkite/starkite/basekite/version"
	"github.com/spf13/cobra"
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
		out := cmd.OutOrStdout()
		if versionShort {
			fmt.Fprintln(out, version.Version)
			return
		}

		// "all" is the default full binary, not an edition; only the lean
		// distributions (base/cloud/ai) are annotated.
		edition := version.EditionName()
		lean := edition != "all"

		if versionJSON {
			info := map[string]string{
				"version": version.Version,
				"commit":  version.GitCommit,
				"built":   version.BuildTime,
				"go":      runtime.Version(),
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			}
			if lean {
				info["edition"] = edition
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Fprintln(out, string(data))
			return
		}

		if lean {
			fmt.Fprintf(out, "kite version %s (%s)\n", version.Version, edition)
			fmt.Fprintf(out, "  edition: %s\n", edition)
		} else {
			fmt.Fprintf(out, "kite version %s\n", version.Version)
		}
		fmt.Fprintf(out, "  commit:  %s\n", version.GitCommit)
		fmt.Fprintf(out, "  built:   %s\n", version.BuildTime)
		fmt.Fprintf(out, "  go:      %s\n", runtime.Version())
		fmt.Fprintf(out, "  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionShort, "short", false, "Print version number only")
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Print version as JSON")
	rootCmd.AddCommand(versionCmd)
}
