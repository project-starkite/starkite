package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

func newTestModule(t *testing.T) (*Module, *bytes.Buffer) {
	t.Helper()
	m := New()
	_, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// Replace default logger with one writing to a buffer
	buf := &bytes.Buffer{}
	m.defaultLogger = newLogger(buf, "info", "text", "stderr", m.levelVar)
	return m, buf
}

func newTestLogger(t *testing.T, level, format string) (*Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	return newLogger(buf, level, format, "stderr", nil), buf
}

// --- Module interface tests ---

func TestModuleName(t *testing.T) {
	m := New()
	if m.Name() != "log" {
		t.Errorf("expected module name 'log', got %q", m.Name())
	}
}

func TestModuleDescription(t *testing.T) {
	m := New()
	if m.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestModuleAliases(t *testing.T) {
	m := New()
	if m.Aliases() != nil {
		t.Error("expected nil aliases")
	}
}

func TestModuleFactoryMethod(t *testing.T) {
	m := New()
	if m.FactoryMethod() != "" {
		t.Error("expected empty factory method")
	}
}

func TestModuleLoad(t *testing.T) {
	m := New()
	dict, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dict["log"]; !ok {
		t.Error("expected 'log' key in loaded dict")
	}
}

// --- Logger value interface ---

func TestLoggerString(t *testing.T) {
	l, _ := newTestLogger(t, "info", "text")
	s := l.String()
	if !strings.Contains(s, "info") || !strings.Contains(s, "text") {
		t.Errorf("unexpected String(): %s", s)
	}
}

func TestLoggerType(t *testing.T) {
	l, _ := newTestLogger(t, "info", "text")
	if l.Type() != "logger" {
		t.Errorf("expected type 'logger', got %q", l.Type())
	}
}

func TestLoggerTruth(t *testing.T) {
	l, _ := newTestLogger(t, "info", "text")
	if l.Truth() != starlark.True {
		t.Error("expected Truth() == True")
	}
}

func TestLoggerHash(t *testing.T) {
	l, _ := newTestLogger(t, "info", "text")
	_, err := l.Hash()
	if err == nil {
		t.Error("expected error from Hash()")
	}
}

// --- Logger properties ---

func TestLoggerProperties(t *testing.T) {
	l, _ := newTestLogger(t, "debug", "json")
	l.output = "stdout"

	tests := []struct {
		attr string
		want string
	}{
		{"level", "debug"},
		{"format", "json"},
		{"output", "stdout"},
	}
	for _, tt := range tests {
		v, err := l.Attr(tt.attr)
		if err != nil {
			t.Errorf("Attr(%q): %v", tt.attr, err)
			continue
		}
		s, ok := starlark.AsString(v)
		if !ok {
			t.Errorf("Attr(%q): not a string", tt.attr)
			continue
		}
		if s != tt.want {
			t.Errorf("Attr(%q) = %q, want %q", tt.attr, s, tt.want)
		}
	}
}

// --- Logger AttrNames ---

func TestLoggerAttrNames(t *testing.T) {
	l, _ := newTestLogger(t, "info", "text")
	names := l.AttrNames()
	expected := []string{"attrs", "debug", "error", "format", "group", "info", "level", "output", "warn"}
	if len(names) != len(expected) {
		t.Fatalf("AttrNames() returned %d items, want %d: %v", len(names), len(expected), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("AttrNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

// --- Logger log methods ---

func TestLoggerInfo(t *testing.T) {
	l, buf := newTestLogger(t, "info", "text")
	l.logger.Info("hello world")
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "INFO") {
		t.Errorf("expected 'INFO' in output, got: %s", buf.String())
	}
}

func TestLoggerDebug(t *testing.T) {
	l, buf := newTestLogger(t, "debug", "text")
	l.logger.Debug("debug msg")
	if !strings.Contains(buf.String(), "debug msg") {
		t.Errorf("expected 'debug msg' in output, got: %s", buf.String())
	}
}

func TestLoggerWarn(t *testing.T) {
	l, buf := newTestLogger(t, "info", "text")
	l.logger.Warn("warning")
	if !strings.Contains(buf.String(), "warning") {
		t.Errorf("expected 'warning' in output, got: %s", buf.String())
	}
}

func TestLoggerError(t *testing.T) {
	l, buf := newTestLogger(t, "info", "text")
	l.logger.Error("oops")
	if !strings.Contains(buf.String(), "oops") {
		t.Errorf("expected 'oops' in output, got: %s", buf.String())
	}
}

// --- Level filtering ---

func TestLevelFiltering(t *testing.T) {
	l, buf := newTestLogger(t, "warn", "text")
	l.logger.Info("should not appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("info message should be filtered at warn level")
	}
	l.logger.Warn("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("warn message should not be filtered at warn level")
	}
}

// --- LevelVar dynamic level change ---

func TestLevelVarDynamic(t *testing.T) {
	buf := &bytes.Buffer{}
	levelVar := &slog.LevelVar{}
	l := newLogger(buf, "info", "text", "stderr", levelVar)

	l.logger.Debug("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Error("debug should be hidden at info level")
	}

	levelVar.Set(slog.LevelDebug)
	l.logger.Debug("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Error("debug should be visible after level change")
	}
}

// --- attrs() ---

func TestLoggerAttrs(t *testing.T) {
	l, buf := newTestLogger(t, "info", "text")
	derived := &Logger{
		logger: l.logger.With("request_id", "abc"),
		level:  l.level,
		format: l.format,
		output: l.output,
	}
	derived.logger.Info("test")
	output := buf.String()
	if !strings.Contains(output, "request_id") || !strings.Contains(output, "abc") {
		t.Errorf("expected persistent attrs in output, got: %s", output)
	}
}

// --- group() with JSON ---

func TestLoggerGroupJSON(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")
	grouped := &Logger{
		logger: l.logger.WithGroup("db"),
		level:  l.level,
		format: l.format,
		output: l.output,
	}
	grouped.logger.Info("connected", "host", "pg.local")
	output := buf.String()

	var entry map[string]any
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, output)
	}
	db, ok := entry["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'db' group in JSON, got: %v", entry)
	}
	if db["host"] != "pg.local" {
		t.Errorf("expected db.host='pg.local', got: %v", db["host"])
	}
}

// --- JSON format produces valid JSON ---

func TestJSONFormat(t *testing.T) {
	l, buf := newTestLogger(t, "info", "json")
	l.logger.Info("test", "key", "value")
	output := buf.String()

	var entry map[string]any
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}
	if entry["msg"] != "test" {
		t.Errorf("expected msg='test', got: %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key='value', got: %v", entry["key"])
	}
}

// --- dictToSlogAttrs ---

func TestDictToSlogAttrs(t *testing.T) {
	d := starlark.NewDict(2)
	d.SetKey(starlark.String("port"), starlark.MakeInt(8080))
	d.SetKey(starlark.String("host"), starlark.String("localhost"))

	attrs, err := dictToSlogAttrs(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 4 { // key, val, key, val
		t.Fatalf("expected 4 items, got %d", len(attrs))
	}
}

func TestDictToSlogAttrsNonStringKey(t *testing.T) {
	d := starlark.NewDict(1)
	d.SetKey(starlark.MakeInt(42), starlark.String("val"))

	_, err := dictToSlogAttrs(d)
	if err == nil {
		t.Error("expected error for non-string key")
	}
}

// --- parseLogArgs ---

func TestParseLogArgs(t *testing.T) {
	args := starlark.Tuple{starlark.String("hello")}
	msg, attrs, err := parseLogArgs(args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "hello" {
		t.Errorf("expected msg='hello', got %q", msg)
	}
	if len(attrs) != 0 {
		t.Errorf("expected no attrs, got %v", attrs)
	}
}

func TestParseLogArgsWithDict(t *testing.T) {
	d := starlark.NewDict(1)
	d.SetKey(starlark.String("k"), starlark.String("v"))

	args := starlark.Tuple{starlark.String("msg")}
	kwargs := []starlark.Tuple{
		{starlark.String("attrs"), d},
	}
	msg, attrs, err := parseLogArgs(args, kwargs)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "msg" {
		t.Errorf("expected msg='msg', got %q", msg)
	}
	if len(attrs) != 2 {
		t.Errorf("expected 2 attr items, got %d", len(attrs))
	}
}

// --- parseSlogLevel ---

func TestParseSlogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		got := parseSlogLevel(tt.input)
		if got != tt.want {
			t.Errorf("parseSlogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- resolveWriter ---

func TestResolveWriter(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"stderr", false},
		{"stdout", false},
		{"", false},
		{"file", true},
	}
	for _, tt := range tests {
		_, err := resolveWriter(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("resolveWriter(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
	}
}

// --- Module debug flag ---

func TestModuleDebugFlag(t *testing.T) {
	m := New()
	_, err := m.Load(&starbase.ModuleConfig{Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.defaultLogger.level != "debug" {
		t.Errorf("expected debug level with Debug flag, got %q", m.defaultLogger.level)
	}
}
