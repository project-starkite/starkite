package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Logger is a Starlark value wrapping a *slog.Logger.
type Logger struct {
	logger *slog.Logger
	level  string // "debug"|"info"|"warn"|"error"
	format string // "text"|"json"
	output string // "stderr"|"stdout"
}

var (
	_ starlark.Value    = (*Logger)(nil)
	_ starlark.HasAttrs = (*Logger)(nil)
)

func (l *Logger) String() string {
	return fmt.Sprintf("logger(level=%s, format=%s, output=%s)", l.level, l.format, l.output)
}
func (l *Logger) Type() string          { return "logger" }
func (l *Logger) Freeze()               {}
func (l *Logger) Truth() starlark.Bool  { return starlark.True }
func (l *Logger) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: logger") }

// Attr implements starlark.HasAttrs.
func (l *Logger) Attr(name string) (starlark.Value, error) {
	switch name {
	case "level":
		return starlark.String(l.level), nil
	case "format":
		return starlark.String(l.format), nil
	case "output":
		return starlark.String(l.output), nil
	}
	if method := l.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (l *Logger) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "debug":
		return starlark.NewBuiltin("logger.debug", l.debugMethod)
	case "info":
		return starlark.NewBuiltin("logger.info", l.infoMethod)
	case "warn":
		return starlark.NewBuiltin("logger.warn", l.warnMethod)
	case "error":
		return starlark.NewBuiltin("logger.error", l.errorMethod)
	case "attrs":
		return starlark.NewBuiltin("logger.attrs", l.attrsMethod)
	case "group":
		return starlark.NewBuiltin("logger.group", l.groupMethod)
	}
	return nil
}

// AttrNames implements starlark.HasAttrs.
func (l *Logger) AttrNames() []string {
	names := []string{"level", "format", "output", "debug", "info", "warn", "error", "attrs", "group"}
	sort.Strings(names)
	return names
}

func (l *Logger) debugMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	l.logger.Debug(msg, slogAttrs...)
	return starlark.None, nil
}

func (l *Logger) infoMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	l.logger.Info(msg, slogAttrs...)
	return starlark.None, nil
}

func (l *Logger) warnMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	l.logger.Warn(msg, slogAttrs...)
	return starlark.None, nil
}

func (l *Logger) errorMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	msg, slogAttrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	l.logger.Error(msg, slogAttrs...)
	return starlark.None, nil
}

func (l *Logger) attrsMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Attrs starlark.Value `name:"attrs" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	dict, ok := p.Attrs.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("logger.attrs: argument must be a dict")
	}
	slogAttrs, err := dictToSlogAttrs(dict)
	if err != nil {
		return nil, err
	}
	return &Logger{
		logger: l.logger.With(slogAttrs...),
		level:  l.level,
		format: l.format,
		output: l.output,
	}, nil
}

func (l *Logger) groupMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name string `name:"name" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return &Logger{
		logger: l.logger.WithGroup(p.Name),
		level:  l.level,
		format: l.format,
		output: l.output,
	}, nil
}

// parseLogArgs extracts msg (positional) and attrs (kwarg dict) from log function arguments.
func parseLogArgs(args starlark.Tuple, kwargs []starlark.Tuple) (string, []any, error) {
	var p struct {
		Msg   string         `name:"msg" position:"0" required:"true"`
		Attrs starlark.Value `name:"attrs"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return "", nil, err
	}
	var slogAttrs []any
	if p.Attrs != nil {
		dict, ok := p.Attrs.(*starlark.Dict)
		if !ok {
			return "", nil, fmt.Errorf("attrs must be a dict")
		}
		var err error
		slogAttrs, err = dictToSlogAttrs(dict)
		if err != nil {
			return "", nil, err
		}
	}
	return p.Msg, slogAttrs, nil
}

// dictToSlogAttrs converts a Starlark dict to slog key-value pairs.
func dictToSlogAttrs(d *starlark.Dict) ([]any, error) {
	attrs := make([]any, 0, d.Len()*2)
	for _, item := range d.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("attrs keys must be strings")
		}
		var goVal any
		if err := startype.Starlark(item[1]).Go(&goVal); err != nil {
			goVal = item[1].String()
		}
		attrs = append(attrs, key, goVal)
	}
	return attrs, nil
}

// newLogger creates a Logger with the given configuration.
// If levelVar is non-nil, the handler uses it for dynamic level changes (default logger).
// Otherwise a fixed level is used (constructed loggers).
func newLogger(w io.Writer, level, format, output string, levelVar *slog.LevelVar) *Logger {
	slogLevel := parseSlogLevel(level)

	var handler slog.Handler
	if levelVar != nil {
		levelVar.Set(slogLevel)
		opts := &slog.HandlerOptions{Level: levelVar}
		if format == "json" {
			handler = slog.NewJSONHandler(w, opts)
		} else {
			handler = slog.NewTextHandler(w, opts)
		}
	} else {
		opts := &slog.HandlerOptions{Level: slogLevel}
		if format == "json" {
			handler = slog.NewJSONHandler(w, opts)
		} else {
			handler = slog.NewTextHandler(w, opts)
		}
	}

	return &Logger{
		logger: slog.New(handler),
		level:  strings.ToLower(level),
		format: format,
		output: output,
	}
}

// parseSlogLevel converts a string level name to slog.Level.
func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// resolveWriter maps "stderr"/"stdout" to the corresponding os writer.
func resolveWriter(output string) (io.Writer, error) {
	switch strings.ToLower(output) {
	case "stderr", "":
		return os.Stderr, nil
	case "stdout":
		return os.Stdout, nil
	default:
		return nil, fmt.Errorf("unknown output: %s (use 'stderr' or 'stdout')", output)
	}
}
