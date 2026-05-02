package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/base/varstore"
	"github.com/project-starkite/starkite/libkite"
)

var watchCmd = &cobra.Command{
	Use:   "watch <script.star>",
	Short: "Watch and re-execute script on file changes",
	Long: `Watch a script file and automatically re-execute it when the file changes.

This is useful during development to get immediate feedback on script changes.

Examples:
  # Watch a script
  kite watch deploy.star

  # Watch with custom output format
  kite watch manifest.star --output=yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: watchScript,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func watchScript(cmd *cobra.Command, args []string) error {
	scriptPath := args[0]

	// Resolve absolute path
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("script file not found: %s", absPath)
	}

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	// Watch the directory containing the script
	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	perms, err := resolvePermissionsForScript(absPath)
	if err != nil {
		return err
	}

	fmt.Printf("Watching %s for changes...\n", scriptPath)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Run script initially
	runWatchedScript(absPath, perms)

	// Debounce timer
	var debounceTimer *time.Timer
	debounceDelay := 200 * time.Millisecond

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Check if this is the script file
			if filepath.Clean(event.Name) != filepath.Clean(absPath) {
				continue
			}

			// Only react to write events
			if event.Op&fsnotify.Write != fsnotify.Write {
				continue
			}

			// Debounce rapid events
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				fmt.Println("\n--- File changed, re-executing ---")
				runWatchedScript(absPath, perms)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("Watch error: %v\n", err)
		}
	}
}

func runWatchedScript(scriptPath string, perms *libkite.PermissionConfig) {
	// Read script content
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Printf("Error reading script: %v\n", err)
		return
	}

	// Create and populate variable store
	varStore := varstore.New()
	varStore.LoadFromEnv()
	_ = varStore.LoadFromCLI(variables)

	// Create module config
	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:   scriptPath,
		OutputFormat: outputFormat,
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		Permissions:  perms,
		Registry:     registry,
	}

	// Create and run the runtime
	rt, err := libkite.New(cfg)
	if err != nil {
		fmt.Printf("Error creating runtime: %v\n", err)
		return
	}
	defer rt.Cleanup()

	ctx, cancel := execContext(timeout)
	defer cancel()

	// Execute the script
	startTime := time.Now()
	if err := rt.Execute(ctx, string(content)); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("\n--- Completed in %s ---\n", elapsed)
}
