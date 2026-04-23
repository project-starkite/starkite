// Package log provides structured logging functions for starkite using slog.
package log

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "log"

// Module implements structured logging functions backed by slog.
type Module struct {
	once          sync.Once
	module        starlark.Value
	config        *starbase.ModuleConfig
	defaultLogger *Logger
	levelVar      *slog.LevelVar
	mu            sync.RWMutex
	format        string // current format for rebuilds
	output        string // current output for rebuilds
}

func New() *Module {
	return &Module{
		format: "text",
		output: "stderr",
	}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "log provides structured logging: debug, info, warn, error, set_level, set_format, set_output, logger"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.levelVar = &slog.LevelVar{}

		level := "info"
		if config.Debug {
			level = "debug"
		}

		m.defaultLogger = newLogger(os.Stderr, level, m.format, m.output, m.levelVar)

		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"debug":      starlark.NewBuiltin("log.debug", m.debug),
				"info":       starlark.NewBuiltin("log.info", m.info),
				"warn":       starlark.NewBuiltin("log.warn", m.warn),
				"error":      starlark.NewBuiltin("log.error", m.logError),
				"set_level":  starlark.NewBuiltin("log.set_level", m.setLevel),
				"set_format": starlark.NewBuiltin("log.set_format", m.setFormat),
				"set_output": starlark.NewBuiltin("log.set_output", m.setOutput),
				"logger":     starlark.NewBuiltin("log.logger", m.loggerFactory),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// Module-level log functions delegate to the default logger.

func (m *Module) debug(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	l := m.defaultLogger
	m.mu.RUnlock()
	l.logger.Debug(msg, slogAttrs...)
	return starlark.None, nil
}

func (m *Module) info(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	l := m.defaultLogger
	m.mu.RUnlock()
	l.logger.Info(msg, slogAttrs...)
	return starlark.None, nil
}

func (m *Module) warn(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	l := m.defaultLogger
	m.mu.RUnlock()
	l.logger.Warn(msg, slogAttrs...)
	return starlark.None, nil
}

func (m *Module) logError(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	l := m.defaultLogger
	m.mu.RUnlock()
	l.logger.Error(msg, slogAttrs...)
	return starlark.None, nil
}

func (m *Module) setLevel(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Level string `name:"level" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	switch p.Level {
	case "debug", "DEBUG", "info", "INFO", "warn", "WARN", "warning", "WARNING", "error", "ERROR":
	default:
		return nil, fmt.Errorf("unknown log level: %s", p.Level)
	}

	m.levelVar.Set(parseSlogLevel(p.Level))

	m.mu.Lock()
	m.defaultLogger.level = p.Level
	m.mu.Unlock()

	return starlark.None, nil
}

func (m *Module) setFormat(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Format string `name:"format" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	switch p.Format {
	case "text", "json":
	default:
		return nil, fmt.Errorf("unknown log format: %s (use 'text' or 'json')", p.Format)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.format = p.Format
	w, _ := resolveWriter(m.output)
	m.defaultLogger = newLogger(w, m.defaultLogger.level, m.format, m.output, m.levelVar)

	return starlark.None, nil
}

func (m *Module) setOutput(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Output string `name:"output" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	w, err := resolveWriter(p.Output)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.output = p.Output
	m.defaultLogger = newLogger(w, m.defaultLogger.level, m.format, m.output, m.levelVar)

	return starlark.None, nil
}

func (m *Module) loggerFactory(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Level  string `name:"level"`
		Format string `name:"format"`
		Output string `name:"output"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Level == "" {
		p.Level = "info"
	}
	if p.Format == "" {
		p.Format = "text"
	}
	if p.Output == "" {
		p.Output = "stderr"
	}

	switch p.Level {
	case "debug", "info", "warn", "warning", "error":
	default:
		return nil, fmt.Errorf("unknown log level: %s", p.Level)
	}
	switch p.Format {
	case "text", "json":
	default:
		return nil, fmt.Errorf("unknown log format: %s (use 'text' or 'json')", p.Format)
	}

	w, err := resolveWriter(p.Output)
	if err != nil {
		return nil, err
	}

	return newLogger(w, p.Level, p.Format, p.Output, nil), nil
}
