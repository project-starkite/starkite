// Package iomod provides interactive I/O functions for starkite.
// Named iomod to avoid conflict with Go's io package.
package iomod

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"golang.org/x/term"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "io"

// Module implements interactive I/O functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "io provides interactive I/O: confirm, prompt"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"confirm": starlark.NewBuiltin("io.confirm", m.confirm),
			"prompt":  starlark.NewBuiltin("io.prompt", m.prompt),
		}

		m.module = starbase.NewTryModule(string(ModuleName), members)
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return nil // No global aliases for io
}

func (m *Module) FactoryMethod() string { return "" }

// confirm prompts the user for confirmation.
func (m *Module) confirm(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Msg     string `name:"msg" position:"0" required:"true"`
		Default bool   `name:"default" position:"1"`
	}
	p.Default = false
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "io", "confirm", p.Msg); err != nil {
		return nil, err
	}

	// Build prompt string with default indicator
	prompt := p.Msg
	if p.Default {
		prompt += " [Y/n]: "
	} else {
		prompt += " [y/N]: "
	}

	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	input = strings.TrimSpace(strings.ToLower(input))

	// Empty input returns default
	if input == "" {
		return starlark.Bool(p.Default), nil
	}

	// Check for affirmative responses
	if input == "y" || input == "yes" {
		return starlark.True, nil
	}

	// Check for negative responses
	if input == "n" || input == "no" {
		return starlark.False, nil
	}

	// Invalid input returns default
	return starlark.Bool(p.Default), nil
}

// prompt prompts the user for input.
func (m *Module) prompt(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Msg     string `name:"msg" position:"0" required:"true"`
		Default string `name:"default" position:"1"`
		Secret  bool   `name:"secret" position:"2"`
	}
	p.Default = ""
	p.Secret = false
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "io", "prompt", p.Msg); err != nil {
		return nil, err
	}

	fmt.Print(p.Msg)

	var input string
	var err error

	if p.Secret {
		// Read password without echo
		fd := int(os.Stdin.Fd())
		password, err := term.ReadPassword(fd)
		if err != nil {
			return nil, err
		}
		fmt.Println() // Print newline after password input
		input = string(password)
	} else {
		reader := bufio.NewReader(os.Stdin)
		input, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		input = strings.TrimSuffix(input, "\n")
		input = strings.TrimSuffix(input, "\r")
	}

	if input == "" && p.Default != "" {
		return starlark.String(p.Default), nil
	}

	return starlark.String(input), nil
}
