package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/project-starkite/starkite/basekite/varstore"
	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch <target>",
	Short: "Watch and re-execute script on file changes",
	Long: `Watch a script file and automatically re-execute it when the file changes.

This is useful during development to get immediate feedback on script changes.

Examples:
  # Watch a script
  kite watch ./deploy.star

  # Watch with custom output format
  kite watch ./manifest.star --output=yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: watchScript,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func watchScript(cmd *cobra.Command, args []string) error {
	// Resolve the run target (path reference or installed namespace/name).
	entry, isModule, err := resolveRunTarget(args[0])
	if err != nil {
		return err
	}
	if isModule {
		if err := resolveModuleDeps(filepath.Dir(entry)); err != nil {
			return fmt.Errorf("failed to resolve dependencies: %w", err)
		}
	} else {
		if err := resolveLooseDeps(entry); err != nil {
			return fmt.Errorf("failed to resolve dependencies: %w", err)
		}
	}
	absPath, err := filepath.Abs(entry)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	varStore, err := loadVarStore()
	if err != nil {
		return err
	}

	activeProfile, err := ResolveActiveProfile()
	if err != nil {
		return err
	}
	policy := activeProfile.PolicySummary(envFlags, envInherit, args[0])
	perms := activeProfile.CompilePermissions()
	checker, _ := libkite.NewPermissionChecker(perms)

	preloaded := GetActivePreloadedClosure()
	if preloaded == nil && absPath != "" {
		if closure, err := libkite.ResolveStaticLoadClosure(absPath, filepath.Dir(absPath), nil); err == nil {
			preloaded = closure
			SetActivePreloadedClosure(preloaded)
		}
	}

	if err := libkite.ValidateClosureManifests(preloaded, policy, checker, filepath.Dir(absPath)); err != nil {
		return err
	}

	if handled, err := MaybeHandoffToSandboxWithScript(cmd.Context(), absPath); handled || err != nil {
		return err
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

	fmt.Printf("Watching %s for changes...\n", absPath)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Run script initially
	ctx := cmd.Context()
	runWatchedScript(ctx, absPath, perms, isModule, varStore)

	// Debounce timer
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time
	debounceDelay := 200 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return nil
		case <-debounceCh:
			debounceCh = nil
			fmt.Println("\n--- File changed, re-executing ---")
			runWatchedScript(ctx, absPath, perms, isModule, varStore)
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Check if this is the script file
			if filepath.Clean(event.Name) != filepath.Clean(absPath) {
				continue
			}

			// React to write, create, or rename events (for atomic-save editors)
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				if event.Op&fsnotify.Rename != 0 {
					_ = watcher.Add(dir)
				}
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(debounceDelay)
				debounceCh = debounceTimer.C
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("Watch error: %v\n", err)
		}
	}
}

func runWatchedScript(ctx context.Context, scriptPath string, perms *libkite.PermissionConfig, isModule bool, varStore *varstore.Vars) {
	preloaded := GetActivePreloadedClosure()
	if closure, cerr := libkite.ResolveStaticLoadClosure(scriptPath, filepath.Dir(scriptPath), nil); cerr == nil && closure != nil {
		preloaded = closure
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		if preloaded != nil {
			if cached, ok := preloaded.Files[filepath.Clean(scriptPath)]; ok {
				content = cached
				err = nil
			}
		}
		if err != nil {
			fmt.Printf("Error reading script: %v\n", err)
			return
		}
	}

	if varStore == nil {
		vs, err := loadVarStore()
		if err != nil {
			fmt.Printf("Error loading variables: %v\n", err)
			return
		}
		varStore = vs
	}

	activeProf, err := ResolveActiveProfile()
	if err != nil {
		activeProf = sandbox.Profile{}
	}
	envMap, _, _ := ResolveRuntimeEnv(&activeProf)

	// Create module config
	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
		Env:      envMap,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:        scriptPath,
		OutputFormat:      outputFormat,
		Debug:             debugMode,
		DryRun:            dryRun,
		VarStore:          varStore,
		Permissions:       perms,
		SandboxPolicy:     activeProf.PolicySummary(envFlags, envInherit, scriptPath),
		Registry:          registry,
		Env:               envMap,
		EntryPoint:        "main",
		RequireEntryPoint: isModule,
		Preloaded:         preloaded,
	}

	// Create and run the runtime
	rt, err := libkite.New(cfg)
	if err != nil {
		fmt.Printf("Error creating runtime: %v\n", err)
		return
	}
	defer rt.Close()
	defer rt.Cleanup()

	execCtx, cancel := execContext(timeout)
	defer cancel()

	// Execute the script
	startTime := time.Now()
	if err := rt.Execute(execCtx, string(content)); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("\n--- Completed in %s ---\n", elapsed)
}
