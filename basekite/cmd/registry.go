package cmd

import (
	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
	"github.com/spf13/cobra"
)

// NewRegistry is the function used by all script-executing commands to create
// a module registry. It defaults to the base loader which registers all built-in
// modules. Edition binaries (e.g. cloud) override this before calling Execute()
// to inject additional modules.
var NewRegistry func(config *libkite.ModuleConfig) *libkite.Registry = loader.NewDefaultRegistry

// RegisterEditionCommands is called at the start of Execute() to allow edition
// binaries to register additional cobra commands on the root command.
// Nil by default (base edition has no extra commands).
var RegisterEditionCommands func(root *cobra.Command)
