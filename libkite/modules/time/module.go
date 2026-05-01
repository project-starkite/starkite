// Package time provides time-related functions for starkite.
package time

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "time"

// Module implements time-related functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "time provides time functions: now, sleep, parse, format, duration"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				// Functions
				"now":      starlark.NewBuiltin("time.now", m.now),
				"sleep":    starlark.NewBuiltin("time.sleep", m.sleep),
				"parse":    starlark.NewBuiltin("time.parse", m.parse),
				"format":   starlark.NewBuiltin("time.format", m.format),
				"duration": starlark.NewBuiltin("time.duration", m.duration),
				"since":    starlark.NewBuiltin("time.since", m.since),
				"until":    starlark.NewBuiltin("time.until", m.until),

				// Format constants
				"RFC3339":     starlark.String(time.RFC3339),
				"RFC3339Nano": starlark.String(time.RFC3339Nano),
				"RFC1123":     starlark.String(time.RFC1123),
				"RFC1123Z":    starlark.String(time.RFC1123Z),
				"RFC822":      starlark.String(time.RFC822),
				"RFC822Z":     starlark.String(time.RFC822Z),
				"RFC850":      starlark.String(time.RFC850),
				"Kitchen":     starlark.String(time.Kitchen),
				"Stamp":       starlark.String(time.Stamp),
				"DateTime":    starlark.String(time.DateTime),
				"DateOnly":    starlark.String(time.DateOnly),
				"TimeOnly":    starlark.String(time.TimeOnly),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// Named format presets for t.string()
var formatPresets = map[string]string{
	"rfc3339":     time.RFC3339,
	"rfc3339nano": time.RFC3339Nano,
	"kitchen":     time.Kitchen,
	"datetime":    time.DateTime,
	"date":        time.DateOnly,
	"time":        time.TimeOnly,
	"stamp":       time.Stamp,
	"rfc822":      time.RFC822,
	"rfc1123":     time.RFC1123,
	"rfc850":      time.RFC850,
}

// strftime to Go layout replacer
var strftimeReplacer = strings.NewReplacer(
	"%%", "\x00", // placeholder for literal %
	"%Y", "2006",
	"%m", "01",
	"%d", "02",
	"%H", "15",
	"%I", "03",
	"%M", "04",
	"%S", "05",
	"%p", "PM",
	"%a", "Mon",
	"%A", "Monday",
	"%b", "Jan",
	"%B", "January",
	"%Z", "MST",
	"%z", "-0700",
)

// resolveFormat converts a format string (preset name, strftime, or Go layout) to a Go layout.
func resolveFormat(format string) string {
	if format == "" {
		return time.RFC3339
	}
	if layout, ok := formatPresets[strings.ToLower(format)]; ok {
		return layout
	}
	if strings.Contains(format, "%") {
		result := strftimeReplacer.Replace(format)
		return strings.ReplaceAll(result, "\x00", "%")
	}
	return format
}

// TimeValue wraps a Go time.Time for Starlark
type TimeValue struct {
	t time.Time
}

var (
	_ starlark.Value      = (*TimeValue)(nil)
	_ starlark.HasAttrs   = (*TimeValue)(nil)
	_ starlark.Comparable = (*TimeValue)(nil)
	_ starlark.HasBinary  = (*TimeValue)(nil)
)

func (tv *TimeValue) String() string        { return tv.t.Format(time.RFC3339) }
func (tv *TimeValue) Type() string          { return "time" }
func (tv *TimeValue) Freeze()               {}
func (tv *TimeValue) Truth() starlark.Bool  { return starlark.True }
func (tv *TimeValue) Hash() (uint32, error) { return uint32(tv.t.UnixNano()), nil }

func (tv *TimeValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "year":
		return starlark.MakeInt(tv.t.Year()), nil
	case "month":
		return starlark.MakeInt(int(tv.t.Month())), nil
	case "day":
		return starlark.MakeInt(tv.t.Day()), nil
	case "hour":
		return starlark.MakeInt(tv.t.Hour()), nil
	case "minute":
		return starlark.MakeInt(tv.t.Minute()), nil
	case "second":
		return starlark.MakeInt(tv.t.Second()), nil
	case "weekday":
		return starlark.MakeInt(int(tv.t.Weekday())), nil
	case "unix":
		return starlark.MakeInt64(tv.t.Unix()), nil
	case "unix_nano":
		return starlark.MakeInt64(tv.t.UnixNano()), nil
	case "string":
		return starlark.NewBuiltin("time.string", tv.stringMethod), nil
	case "add":
		return starlark.NewBuiltin("time.add", tv.addMethod), nil
	case "sub":
		return starlark.NewBuiltin("time.sub", tv.subMethod), nil
	default:
		return nil, nil
	}
}

func (tv *TimeValue) AttrNames() []string {
	return []string{"year", "month", "day", "hour", "minute", "second", "weekday", "unix", "unix_nano", "string", "add", "sub"}
}

// CompareSameType implements starlark.Comparable.
func (tv *TimeValue) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	other := y.(*TimeValue)
	cmp := tv.t.Compare(other.t)
	switch op {
	case syntax.EQL:
		return cmp == 0, nil
	case syntax.NEQ:
		return cmp != 0, nil
	case syntax.LT:
		return cmp < 0, nil
	case syntax.LE:
		return cmp <= 0, nil
	case syntax.GT:
		return cmp > 0, nil
	case syntax.GE:
		return cmp >= 0, nil
	}
	return false, fmt.Errorf("unsupported comparison: %s", op)
}

// Binary implements starlark.HasBinary for + and - operators.
func (tv *TimeValue) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	switch op {
	case syntax.PLUS:
		// time + duration → time
		if side == starlark.Left {
			if dv, ok := y.(*DurationValue); ok {
				return &TimeValue{t: tv.t.Add(dv.d)}, nil
			}
		}
		// duration + time → time (handled from DurationValue side)
	case syntax.MINUS:
		if side == starlark.Left {
			switch other := y.(type) {
			case *DurationValue:
				// time - duration → time
				return &TimeValue{t: tv.t.Add(-other.d)}, nil
			case *TimeValue:
				// time - time → duration
				return &DurationValue{d: tv.t.Sub(other.t)}, nil
			}
		}
	}
	return nil, nil
}

// stringMethod implements t.string(format="")
func (tv *TimeValue) stringMethod(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var format string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "format?", &format); err != nil {
		return nil, err
	}
	layout := resolveFormat(format)
	return starlark.String(tv.t.Format(layout)), nil
}

// addMethod implements t.add(duration_string) → TimeValue
func (tv *TimeValue) addMethod(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var durStr string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "duration", &durStr); err != nil {
		return nil, err
	}
	d, err := time.ParseDuration(durStr)
	if err != nil {
		return nil, err
	}
	return &TimeValue{t: tv.t.Add(d)}, nil
}

// subMethod implements t.sub(other_time) → DurationValue
func (tv *TimeValue) subMethod(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sub: expected 1 argument (time), got %d", len(args))
	}
	other, ok := args[0].(*TimeValue)
	if !ok {
		return nil, fmt.Errorf("sub: argument must be a time value, got %s", args[0].Type())
	}
	return &DurationValue{d: tv.t.Sub(other.t)}, nil
}

// DurationValue wraps a Go time.Duration for Starlark
type DurationValue struct {
	d time.Duration
}

var (
	_ starlark.Value      = (*DurationValue)(nil)
	_ starlark.HasAttrs   = (*DurationValue)(nil)
	_ starlark.Comparable = (*DurationValue)(nil)
	_ starlark.HasBinary  = (*DurationValue)(nil)
)

func (dv *DurationValue) String() string        { return dv.d.String() }
func (dv *DurationValue) Type() string          { return "duration" }
func (dv *DurationValue) Freeze()               {}
func (dv *DurationValue) Truth() starlark.Bool  { return starlark.Bool(dv.d != 0) }
func (dv *DurationValue) Hash() (uint32, error) { return uint32(dv.d), nil }

func (dv *DurationValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "seconds":
		return starlark.Float(dv.d.Seconds()), nil
	case "milliseconds":
		return starlark.MakeInt64(dv.d.Milliseconds()), nil
	case "nanoseconds":
		return starlark.MakeInt64(dv.d.Nanoseconds()), nil
	case "minutes":
		return starlark.Float(dv.d.Minutes()), nil
	case "hours":
		return starlark.Float(dv.d.Hours()), nil
	case "string":
		return starlark.NewBuiltin("duration.string", dv.stringMethod), nil
	default:
		return nil, nil
	}
}

func (dv *DurationValue) AttrNames() []string {
	return []string{"seconds", "milliseconds", "nanoseconds", "minutes", "hours", "string"}
}

// CompareSameType implements starlark.Comparable.
func (dv *DurationValue) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	other := y.(*DurationValue)
	switch op {
	case syntax.EQL:
		return dv.d == other.d, nil
	case syntax.NEQ:
		return dv.d != other.d, nil
	case syntax.LT:
		return dv.d < other.d, nil
	case syntax.LE:
		return dv.d <= other.d, nil
	case syntax.GT:
		return dv.d > other.d, nil
	case syntax.GE:
		return dv.d >= other.d, nil
	}
	return false, fmt.Errorf("unsupported comparison: %s", op)
}

// Binary implements starlark.HasBinary for +, -, * operators.
func (dv *DurationValue) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	switch op {
	case syntax.PLUS:
		if other, ok := y.(*DurationValue); ok {
			return &DurationValue{d: dv.d + other.d}, nil
		}
		// duration + time → time
		if side == starlark.Left {
			if tv, ok := y.(*TimeValue); ok {
				return &TimeValue{t: tv.t.Add(dv.d)}, nil
			}
		}
	case syntax.MINUS:
		if side == starlark.Left {
			if other, ok := y.(*DurationValue); ok {
				return &DurationValue{d: dv.d - other.d}, nil
			}
		}
	case syntax.STAR:
		// duration * int or int * duration
		if n, err := starlark.AsInt32(y); err == nil {
			return &DurationValue{d: dv.d * time.Duration(n)}, nil
		}
	}
	return nil, nil
}

// stringMethod implements d.string()
func (dv *DurationValue) stringMethod(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("string takes no arguments")
	}
	return starlark.String(dv.d.String()), nil
}

func (m *Module) now(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("now takes no arguments")
	}
	return &TimeValue{t: time.Now()}, nil
}

func (m *Module) sleep(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Duration string `name:"duration" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	d, err := time.ParseDuration(p.Duration)
	if err != nil {
		return nil, err
	}
	time.Sleep(d)
	return starlark.None, nil
}

func (m *Module) parse(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Layout string `name:"layout" position:"0" required:"true"`
		Value  string `name:"value" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	t, err := time.Parse(p.Layout, p.Value)
	if err != nil {
		return nil, err
	}
	return &TimeValue{t: t}, nil
}

func (m *Module) format(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// format requires a TimeValue which startype can't handle directly
	// Extract it manually from args
	if len(args) < 2 {
		return nil, fmt.Errorf("format: expected 2 arguments (time, layout), got %d", len(args))
	}
	tv, ok := args[0].(*TimeValue)
	if !ok {
		return nil, fmt.Errorf("format: first argument must be a time value, got %s", args[0].Type())
	}
	layout, ok := starlark.AsString(args[1])
	if !ok {
		return nil, fmt.Errorf("format: second argument must be a string, got %s", args[1].Type())
	}
	return starlark.String(tv.t.Format(layout)), nil
}

func (m *Module) duration(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Value string `name:"value" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	d, err := time.ParseDuration(p.Value)
	if err != nil {
		return nil, err
	}
	return &DurationValue{d: d}, nil
}

func (m *Module) since(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// since requires a TimeValue which startype can't handle directly
	if len(args) != 1 {
		return nil, fmt.Errorf("since: expected 1 argument (time), got %d", len(args))
	}
	tv, ok := args[0].(*TimeValue)
	if !ok {
		return nil, fmt.Errorf("since: argument must be a time value, got %s", args[0].Type())
	}
	return &DurationValue{d: time.Since(tv.t)}, nil
}

func (m *Module) until(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// until requires a TimeValue which startype can't handle directly
	if len(args) != 1 {
		return nil, fmt.Errorf("until: expected 1 argument (time), got %d", len(args))
	}
	tv, ok := args[0].(*TimeValue)
	if !ok {
		return nil, fmt.Errorf("until: argument must be a time value, got %s", args[0].Type())
	}
	return &DurationValue{d: time.Until(tv.t)}, nil
}
