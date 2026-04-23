# time_test.star - Tests for time module

def test_now():
    """Test time.now returns current time."""
    t = time.now()
    assert(t != None, "should return time object")
    assert(t.year >= 2024, "year should be current")

def test_now_fields():
    """Test time.now has expected fields."""
    t = time.now()
    assert(t.year > 0, "should have year")
    assert(t.month >= 1 and t.month <= 12, "month in valid range")
    assert(t.day >= 1 and t.day <= 31, "day in valid range")
    assert(t.hour >= 0 and t.hour <= 23, "hour in valid range")
    assert(t.minute >= 0 and t.minute <= 59, "minute in valid range")
    assert(t.second >= 0 and t.second <= 59, "second in valid range")

def test_format_rfc3339():
    """Test time.format with RFC3339."""
    t = time.now()
    result = time.format(t, time.RFC3339)
    assert("T" in result, "RFC3339 has T separator")
    assert("-" in result, "RFC3339 has dashes")

def test_format_custom():
    """Test time.format with custom layout."""
    t = time.now()
    result = time.format(t, "2006-01-02")
    parts = result.split("-")
    assert(len(parts) == 3, "should have 3 date parts")

def test_parse():
    """Test time.parse."""
    t = time.parse(time.RFC3339, "2024-06-15T10:30:00Z")
    assert(t.year == 2024, "year should be 2024")
    assert(t.month == 6, "month should be 6")
    assert(t.day == 15, "day should be 15")

def test_duration():
    """Test time.duration."""
    d = time.duration("1h30m")
    assert(d != None, "duration should not be None")

def test_duration_seconds():
    """Test time.duration with seconds."""
    d = time.duration("30s")
    assert(d != None, "duration should not be None")

def test_since():
    """Test time.since."""
    t1 = time.now()
    time.sleep("10ms")
    elapsed = time.since(t1)
    assert(elapsed != None, "should return duration")

def test_until():
    """Test time.until (time in past gives negative)."""
    t = time.now()
    time.sleep("10ms")
    # t is now in the past, so until(t) should be negative or zero
    remaining = time.until(t)
    assert(remaining != None, "should return duration")

def test_sleep():
    """Test time.sleep (short duration)."""
    t1 = time.now()
    time.sleep("10ms")
    t2 = time.now()
    elapsed = time.since(t1)
    assert(elapsed != None, "should have elapsed time")

def test_constants():
    """Test time format constants exist."""
    assert(time.RFC3339 != "", "RFC3339 should be defined")
    assert(time.RFC822 != "", "RFC822 should be defined")
    assert(time.Kitchen != "", "Kitchen should be defined")
    assert(time.DateOnly != "", "DateOnly should be defined")
    assert(time.TimeOnly != "", "TimeOnly should be defined")

# --- t.string() tests ---

def test_string_default():
    """Test t.string() defaults to RFC3339."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string()
    assert(result == "2026-03-11T14:30:00Z", "default should be RFC3339, got: " + result)

def test_string_preset_kitchen():
    """Test t.string with kitchen preset."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("kitchen")
    assert(result == "2:30PM", "kitchen preset, got: " + result)

def test_string_preset_date():
    """Test t.string with date preset."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("date")
    assert(result == "2026-03-11", "date preset, got: " + result)

def test_string_preset_datetime():
    """Test t.string with datetime preset."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("datetime")
    assert(result == "2026-03-11 14:30:00", "datetime preset, got: " + result)

def test_string_preset_time():
    """Test t.string with time preset."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("time")
    assert(result == "14:30:00", "time preset, got: " + result)

def test_string_preset_case_insensitive():
    """Test that preset names are case-insensitive."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    assert(t.string("Kitchen") == t.string("kitchen"), "presets should be case-insensitive")
    assert(t.string("RFC3339") == t.string("rfc3339"), "presets should be case-insensitive")

def test_string_strftime():
    """Test t.string with strftime format."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("%Y/%m/%d %H:%M")
    assert(result == "2026/03/11 14:30", "strftime format, got: " + result)

def test_string_strftime_percent():
    """Test t.string with literal percent in strftime."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("%Y%%")
    assert(result == "2026%", "literal percent, got: " + result)

def test_string_go_layout():
    """Test t.string with Go reference layout."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    result = t.string("2006-01-02")
    assert(result == "2026-03-11", "Go layout, got: " + result)

# --- t.add() / t.sub() tests ---

def test_add_duration():
    """Test t.add with duration string."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    future = t.add("5m30s")
    assert(future.minute == 35, "should be 35 minutes, got: " + str(future.minute))
    assert(future.second == 30, "should be 30 seconds, got: " + str(future.second))

def test_add_negative():
    """Test t.add with negative duration."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    past = t.add("-1h")
    assert(past.hour == 13, "should be 13:30, got hour: " + str(past.hour))

def test_sub_times():
    """Test t.sub returns duration between times."""
    t1 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    t2 = time.parse(time.RFC3339, "2026-03-11T14:35:30Z")
    elapsed = t2.sub(t1)
    assert(elapsed.seconds == 330.0, "should be 330 seconds, got: " + str(elapsed.seconds))
    assert(elapsed.minutes == 5.5, "should be 5.5 minutes, got: " + str(elapsed.minutes))

# --- DurationValue attributes ---

def test_duration_minutes():
    """Test duration.minutes attribute."""
    d = time.duration("1h30m")
    assert(d.minutes == 90.0, "should be 90 minutes, got: " + str(d.minutes))

def test_duration_hours():
    """Test duration.hours attribute."""
    d = time.duration("1h30m")
    assert(d.hours == 1.5, "should be 1.5 hours, got: " + str(d.hours))

def test_duration_string_method():
    """Test d.string() method."""
    d = time.duration("1h30m")
    result = d.string()
    assert(result == "1h30m0s", "should be '1h30m0s', got: " + result)

# --- Comparison tests ---

def test_time_comparison_equal():
    """Test time equality."""
    t1 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    t2 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    assert(t1 == t2, "same times should be equal")
    assert(not (t1 != t2), "same times should not be unequal")

def test_time_comparison_less():
    """Test time less-than."""
    t1 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    t2 = time.parse(time.RFC3339, "2026-03-11T14:31:00Z")
    assert(t1 < t2, "earlier time should be less")
    assert(t1 <= t2, "earlier time should be less-or-equal")
    assert(not (t1 > t2), "earlier time should not be greater")

def test_time_comparison_greater():
    """Test time greater-than."""
    t1 = time.parse(time.RFC3339, "2026-03-11T14:31:00Z")
    t2 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    assert(t1 > t2, "later time should be greater")
    assert(t1 >= t2, "later time should be greater-or-equal")

def test_duration_comparison_equal():
    """Test duration equality."""
    d1 = time.duration("1h")
    d2 = time.duration("60m")
    assert(d1 == d2, "1h should equal 60m")

def test_duration_comparison_less():
    """Test duration less-than."""
    d1 = time.duration("30m")
    d2 = time.duration("1h")
    assert(d1 < d2, "30m should be less than 1h")
    assert(d1 <= d2, "30m should be less-or-equal to 1h")

def test_duration_comparison_greater():
    """Test duration greater-than."""
    d1 = time.duration("2h")
    d2 = time.duration("30m")
    assert(d1 > d2, "2h should be greater than 30m")
    assert(d1 >= d2, "2h should be greater-or-equal to 30m")

# --- Binary operator tests ---

def test_duration_add():
    """Test duration + duration."""
    d1 = time.duration("1h")
    d2 = time.duration("30m")
    result = d1 + d2
    assert(result.minutes == 90.0, "1h + 30m should be 90 minutes, got: " + str(result.minutes))

def test_duration_sub():
    """Test duration - duration."""
    d1 = time.duration("1h")
    d2 = time.duration("30m")
    result = d1 - d2
    assert(result.minutes == 30.0, "1h - 30m should be 30 minutes, got: " + str(result.minutes))

def test_duration_mul():
    """Test duration * int."""
    d = time.duration("1h")
    result = d * 2
    assert(result.hours == 2.0, "1h * 2 should be 2 hours, got: " + str(result.hours))

def test_duration_mul_commutative():
    """Test int * duration (commutative)."""
    d = time.duration("1h")
    result = 2 * d
    assert(result.hours == 2.0, "2 * 1h should be 2 hours, got: " + str(result.hours))

def test_time_plus_duration():
    """Test time + duration operator."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    d = time.duration("5m")
    result = t + d
    assert(result.minute == 35, "14:30 + 5m should be 14:35, got minute: " + str(result.minute))

def test_time_minus_duration():
    """Test time - duration operator."""
    t = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    d = time.duration("1h")
    result = t - d
    assert(result.hour == 13, "14:30 - 1h should be 13:30, got hour: " + str(result.hour))

def test_time_minus_time():
    """Test time - time operator."""
    t1 = time.parse(time.RFC3339, "2026-03-11T14:30:00Z")
    t2 = time.parse(time.RFC3339, "2026-03-11T15:00:00Z")
    result = t2 - t1
    assert(result.minutes == 30.0, "15:00 - 14:30 should be 30 minutes, got: " + str(result.minutes))
