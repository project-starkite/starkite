---
title: "time"
description: "Time, duration, formatting, and arithmetic"
weight: 15
---

The `time` module provides time values, durations, formatting, parsing, and arithmetic.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `time.now()` | `TimeValue` | Get the current time |
| `time.sleep(duration)` | `None` | Sleep for the given duration (e.g. `"5s"`, `"100ms"`) |
| `time.parse(layout, value)` | `TimeValue` | Parse a time string using the given layout |
| `time.format(time, layout)` | `string` | Format a `TimeValue` using the given layout |
| `time.duration(value)` | `DurationValue` | Parse a duration string (e.g. `"1h30m"`, `"500ms"`) |
| `time.since(time)` | `DurationValue` | Duration elapsed since `time` |
| `time.until(time)` | `DurationValue` | Duration remaining until `time` |

## TimeValue

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `year` | `int` | Year |
| `month` | `int` | Month (1-12) |
| `day` | `int` | Day of the month |
| `hour` | `int` | Hour (0-23) |
| `minute` | `int` | Minute (0-59) |
| `second` | `int` | Second (0-59) |
| `weekday` | `int` | Day of the week (0 = Sunday) |
| `unix` | `int` | Unix timestamp in seconds |
| `unix_nano` | `int` | Unix timestamp in nanoseconds |

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `string(format="")` | `string` | Format as string; default is RFC 3339. Accepts format presets, strftime, or Go layouts |
| `add(duration)` | `TimeValue` | Add a duration, returning a new time |
| `sub(time)` | `DurationValue` | Subtract another time, returning a duration |

## Operators

| Expression | Result | Description |
|------------|--------|-------------|
| `time + duration` | `TimeValue` | Add duration to time |
| `time - duration` | `TimeValue` | Subtract duration from time |
| `time - time` | `DurationValue` | Difference between two times |
| `duration + duration` | `DurationValue` | Sum of durations |
| `duration - duration` | `DurationValue` | Difference of durations |
| `duration * int` | `DurationValue` | Scale a duration |

### Comparison Operators

`TimeValue` and `DurationValue` support `==`, `!=`, `<`, `<=`, `>`, `>=`.

## Format Presets

The `string()` method and `time.format()` accept these preset names:

| Preset | Example Output |
|--------|---------------|
| `rfc3339` (default) | `2025-01-15T14:30:00Z` |
| `kitchen` | `3:04PM` |
| `datetime` | `2025-01-15 14:30:00` |
| `date` | `2025-01-15` |
| `time` | `14:30:00` |
| `stamp` | `Jan 15 14:30:00` |
| `rfc822` | `15 Jan 25 14:30 UTC` |
| `rfc1123` | `Wed, 15 Jan 2025 14:30:00 UTC` |

You can also use **strftime** patterns (e.g. `%Y-%m-%d %H:%M:%S`) or **Go reference layouts** (e.g. `2006-01-02`).

## Constants

| Constant | Description |
|----------|-------------|
| `time.RFC3339` | RFC 3339 layout |
| `time.RFC3339Nano` | RFC 3339 with nanoseconds |
| `time.RFC1123` | RFC 1123 layout |
| `time.RFC822` | RFC 822 layout |
| `time.Kitchen` | Kitchen time layout |
| `time.DateTime` | Date and time layout |
| `time.DateOnly` | Date-only layout |
| `time.TimeOnly` | Time-only layout |

## Examples

### Current time

```python
now = time.now()
print(now.string())           # 2025-01-15T14:30:00Z
print(now.string("kitchen"))  # 3:04PM
print(now.year, now.month, now.day)
```

### Parsing and formatting

```python
t = time.parse(time.RFC3339, "2025-06-15T10:00:00Z")
print(time.format(t, "date"))  # 2025-06-15

# strftime style
print(t.string("%Y/%m/%d"))  # 2025/06/15
```

### Duration arithmetic

```python
d = time.duration("1h30m")
start = time.now()
deadline = start + d

remaining = time.until(deadline)
print(remaining)
```

### Measuring elapsed time

```python
start = time.now()
exec("make build")
elapsed = time.since(start)
print("Build took:", elapsed)
```

### Sleeping

```python
time.sleep("2s")
print("Done waiting")
```

### Time comparisons

```python
t1 = time.parse(time.RFC3339, "2025-01-01T00:00:00Z")
t2 = time.now()
if t2 > t1:
    print("We are past 2025!")
```

> **Note:**
All `time` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

