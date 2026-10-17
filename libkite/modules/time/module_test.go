package time

import (
	"context"
	"testing"
	gotime "time"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func TestModuleMetadata(t *testing.T) {
	m := New()
	if m.Name() != ModuleName {
		t.Fatalf("expected name %s, got %s", ModuleName, m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	if m.FactoryMethod() != "" {
		t.Fatalf("expected empty factory method, got %q", m.FactoryMethod())
	}
}

func TestNowParseFormat(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// now
	val, err := m.now(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("now error: %v", err)
	}
	tv, ok := val.(*TimeValue)
	if !ok {
		t.Fatalf("expected *TimeValue, got %T", val)
	}
	if tv.t.IsZero() {
		t.Fatal("expected non-zero time")
	}

	// parse 1 arg (RFC3339)
	val, err = m.parse(thread, nil, starlark.Tuple{starlark.String("2026-04-17T12:00:00Z")}, nil)
	if err != nil {
		t.Fatalf("parse(1-arg) error: %v", err)
	}
	tv = val.(*TimeValue)
	if tv.t.Year() != 2026 || tv.t.Month() != 4 || tv.t.Day() != 17 {
		t.Fatalf("parsed date mismatch: %v", tv.t)
	}

	// parse with preset layout (both orders)
	val, err = m.parse(thread, nil, starlark.Tuple{starlark.String("date"), starlark.String("2026-04-17")}, nil)
	if err != nil || val.(*TimeValue).t.Year() != 2026 {
		t.Fatalf("parse(date, val) error: %v, val: %v", err, val)
	}
	val, err = m.parse(thread, nil, starlark.Tuple{starlark.String("2026-04-17"), starlark.String("date")}, nil)
	if err != nil || val.(*TimeValue).t.Year() != 2026 {
		t.Fatalf("parse(val, date) error: %v, val: %v", err, val)
	}

	// parse with strftime pattern
	val, err = m.parse(thread, nil, starlark.Tuple{starlark.String("%Y-%m-%d"), starlark.String("2026-04-17")}, nil)
	if err != nil || val.(*TimeValue).t.Year() != 2026 {
		t.Fatalf("parse(strftime, val) error: %v, val: %v", err, val)
	}

	// format with preset and strftime
	val, err = m.format(thread, nil, starlark.Tuple{tv, starlark.String("date")}, nil)
	if err != nil || val != starlark.String("2026-04-17") {
		t.Fatalf("format(tv, 'date') = %v, err=%v", val, err)
	}

	val, err = m.format(thread, nil, starlark.Tuple{tv, starlark.String("%Y/%m/%d")}, nil)
	if err != nil || val != starlark.String("2026/04/17") {
		t.Fatalf("format(tv, '%%Y/%%m/%%d') = %v, err=%v", val, err)
	}
}

func TestTimeValueAttrsAndOps(t *testing.T) {
	refTime := gotime.Date(2026, 4, 17, 14, 30, 45, 123456789, gotime.UTC)
	tv := &TimeValue{t: refTime}

	// Check fields
	checks := map[string]int{
		"year":       2026,
		"month":      4,
		"day":        17,
		"hour":       14,
		"minute":     30,
		"second":     45,
		"nanosecond": 123456789,
	}
	for field, want := range checks {
		val, err := tv.Attr(field)
		if err != nil {
			t.Fatalf("Attr(%q) error: %v", field, err)
		}
		num, _ := val.(starlark.Int).Int64()
		if int(num) != want {
			t.Fatalf("Attr(%q) = %d, want %d", field, num, want)
		}
	}

	// utc
	utcVal, err := tv.Attr("utc")
	if err != nil || utcVal == nil {
		t.Fatalf("Attr('utc') error: %v", err)
	}

	// string and try_string
	thread := &starlark.Thread{Name: "test"}
	strMethod, _ := tv.Attr("string")
	res, err := starlark.Call(thread, strMethod.(*starlark.Builtin), starlark.Tuple{starlark.String("date")}, nil)
	if err != nil || res != starlark.String("2026-04-17") {
		t.Fatalf("t.string('date') = %v, err=%v", res, err)
	}

	tryStrMethod, _ := tv.Attr("try_string")
	res, err = starlark.Call(thread, tryStrMethod.(*starlark.Builtin), starlark.Tuple{starlark.String("date")}, nil)
	if err != nil || !bool(res.(*libkite.Result).Truth()) {
		t.Fatalf("t.try_string('date') = %v, err=%v", res, err)
	}

	// add with string and DurationValue
	durVal := &DurationValue{d: 2 * gotime.Hour}
	addMethod, _ := tv.Attr("add")
	res1, err := starlark.Call(thread, addMethod.(*starlark.Builtin), starlark.Tuple{starlark.String("2h")}, nil)
	if err != nil {
		t.Fatalf("t.add('2h') error: %v", err)
	}
	res2, err := starlark.Call(thread, addMethod.(*starlark.Builtin), starlark.Tuple{durVal}, nil)
	if err != nil {
		t.Fatalf("t.add(durVal) error: %v", err)
	}
	eq, _ := starlark.Equal(res1, res2)
	if !eq {
		t.Fatal("t.add('2h') != t.add(durVal)")
	}

	// Binary + and -
	sum, err := tv.Binary(syntax.PLUS, durVal, starlark.Left)
	if err != nil || sum == nil {
		t.Fatalf("t + d error: %v", err)
	}
	diff, err := tv.Binary(syntax.MINUS, durVal, starlark.Left)
	if err != nil || diff == nil {
		t.Fatalf("t - d error: %v", err)
	}

	// Sub other time
	otherTime := &TimeValue{t: refTime.Add(30 * gotime.Minute)}
	timeDiff, err := otherTime.Binary(syntax.MINUS, tv, starlark.Left)
	if err != nil {
		t.Fatalf("t2 - t1 error: %v", err)
	}
	diffDuration, ok := timeDiff.(*DurationValue)
	if !ok || diffDuration.d != 30*gotime.Minute {
		t.Fatalf("time diff = %v, want 30m", timeDiff)
	}

	// Comparisons
	cmpLT, err := tv.CompareSameType(syntax.LT, otherTime, 0)
	if err != nil || !cmpLT {
		t.Fatalf("t1 < t2 = %v, err=%v", cmpLT, err)
	}
	cmpGT, err := otherTime.CompareSameType(syntax.GT, tv, 0)
	if err != nil || !cmpGT {
		t.Fatalf("t2 > t1 = %v, err=%v", cmpGT, err)
	}
}

func TestDurationValueAttrsAndOps(t *testing.T) {
	d := &DurationValue{d: 90 * gotime.Minute}

	// Fields
	if d.String() != "1h30m0s" {
		t.Fatalf("Duration string = %q, want '1h30m0s'", d.String())
	}
	hoursVal, _ := d.Attr("hours")
	if h, ok := hoursVal.(starlark.Float); !ok || float64(h) != 1.5 {
		t.Fatalf("hours = %v, want 1.5", hoursVal)
	}
	minsVal, _ := d.Attr("minutes")
	if m, ok := minsVal.(starlark.Float); !ok || float64(m) != 90.0 {
		t.Fatalf("minutes = %v, want 90.0", minsVal)
	}
	secsVal, _ := d.Attr("seconds")
	if s, ok := secsVal.(starlark.Float); !ok || float64(s) != 5400.0 {
		t.Fatalf("seconds = %v, want 5400.0", secsVal)
	}

	// string and try_string
	thread := &starlark.Thread{Name: "test"}
	strMethod, _ := d.Attr("string")
	res, err := starlark.Call(thread, strMethod.(*starlark.Builtin), nil, nil)
	if err != nil || res != starlark.String("1h30m0s") {
		t.Fatalf("d.string() = %v, err=%v", res, err)
	}

	tryStrMethod, _ := d.Attr("try_string")
	res, err = starlark.Call(thread, tryStrMethod.(*starlark.Builtin), nil, nil)
	if err != nil || !bool(res.(*libkite.Result).Truth()) {
		t.Fatalf("d.try_string() = %v, err=%v", res, err)
	}

	// Operations: d1 + d2, d1 - d2, d * 2
	d2 := &DurationValue{d: 30 * gotime.Minute}
	sumVal, err := d.Binary(syntax.PLUS, d2, starlark.Left)
	if err != nil || sumVal.(*DurationValue).d != 120*gotime.Minute {
		t.Fatalf("d1 + d2 = %v", sumVal)
	}
	diffVal, err := d.Binary(syntax.MINUS, d2, starlark.Left)
	if err != nil || diffVal.(*DurationValue).d != 60*gotime.Minute {
		t.Fatalf("d1 - d2 = %v", diffVal)
	}
	mulVal, err := d.Binary(syntax.STAR, starlark.MakeInt(2), starlark.Left)
	if err != nil || mulVal.(*DurationValue).d != 180*gotime.Minute {
		t.Fatalf("d * 2 = %v", mulVal)
	}

	// Module duration creation from string and int
	mod := New()
	_, _ = mod.Load(&libkite.ModuleConfig{})
	fromStr, err := mod.duration(thread, nil, starlark.Tuple{starlark.String("1h30m")}, nil)
	if err != nil || fromStr.(*DurationValue).d != 90*gotime.Minute {
		t.Fatalf("duration('1h30m') = %v, err=%v", fromStr, err)
	}
	fromInt, err := mod.duration(thread, nil, starlark.Tuple{starlark.MakeInt(90)}, nil)
	if err != nil || fromInt.(*DurationValue).d != 90*gotime.Second {
		t.Fatalf("duration(90) = %v, err=%v", fromInt, err)
	}
}

func TestSleepAndContextCancellation(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})

	// Fast sleep succeeds
	thread := &starlark.Thread{Name: "test"}
	libkite.SetThreadContext(thread, context.Background())
	val, err := m.sleep(thread, nil, starlark.Tuple{starlark.String("1ms")}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("sleep('1ms') error: %v", err)
	}

	// Numeric seconds sleep succeeds
	val, err = m.sleep(thread, nil, starlark.Tuple{starlark.Float(0.001)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("sleep(0.001) error: %v", err)
	}

	// Negative duration returns error
	_, err = m.sleep(thread, nil, starlark.Tuple{starlark.String("-5s")}, nil)
	if err == nil {
		t.Fatal("sleep('-5s') should return error")
	}

	// Cancelled context aborts sleep
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	cancelThread := &starlark.Thread{Name: "cancel"}
	libkite.SetThreadContext(cancelThread, ctx)
	_, err = m.sleep(cancelThread, nil, starlark.Tuple{starlark.String("10s")}, nil)
	if err == nil {
		t.Fatal("sleep with cancelled context should return error")
	}
}
