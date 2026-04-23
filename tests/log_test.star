# log_test.star - Tests for log module

# --- function existence ---

def test_log_module_functions():
    """Test log module exports all expected functions."""
    funcs = dir(log)
    assert_contains(funcs, "logger")
    assert_contains(funcs, "info")
    assert_contains(funcs, "debug")
    assert_contains(funcs, "warn")
    assert_contains(funcs, "error")
    assert_contains(funcs, "set_level")
    assert_contains(funcs, "set_format")
    assert_contains(funcs, "set_output")

# --- module-level functions ---

def test_log_info_returns_none():
    """log.info returns None."""
    assert_equal(log.info("test message"), None)

def test_log_with_attrs():
    """log.info accepts attrs kwarg."""
    log.info("test", attrs={"host": "localhost", "port": 8080})

def test_log_all_levels():
    """All log levels work."""
    log.set_level("debug")
    log.debug("debug msg", attrs={"k": "v"})
    log.info("info msg")
    log.warn("warn msg")
    log.error("error msg")
    log.set_level("info")

# --- set_level / set_format / set_output ---

def test_set_level():
    """set_level changes log level."""
    log.set_level("debug")
    log.debug("visible")
    log.set_level("info")

def test_set_format():
    """set_format switches between text and json."""
    log.set_format("json")
    log.info("json mode")
    log.set_format("text")

def test_set_output():
    """set_output switches between stderr and stdout."""
    log.set_output("stdout")
    log.info("to stdout")
    log.set_output("stderr")

# --- logger constructor ---

def test_logger_constructor():
    """log.logger() creates a Logger object."""
    l = log.logger(level="debug", format="text")
    assert_equal(type(l), "logger")
    assert_equal(l.level, "debug")
    assert_equal(l.format, "text")
    assert_equal(l.output, "stderr")

def test_logger_defaults():
    """log.logger() with no args uses defaults."""
    l = log.logger()
    assert_equal(l.level, "info")
    assert_equal(l.format, "text")
    assert_equal(l.output, "stderr")

def test_logger_output_stdout():
    """log.logger(output='stdout') sets output."""
    l = log.logger(output="stdout")
    assert_equal(l.output, "stdout")
    l.info("to stdout")

def test_logger_json():
    """log.logger(format='json') produces JSON output."""
    l = log.logger(format="json")
    assert_equal(l.format, "json")
    l.info("json test", attrs={"key": "val"})

# --- .attrs() ---

def test_logger_attrs():
    """attrs() returns a derived logger with persistent fields."""
    l = log.logger(level="debug")
    derived = l.attrs({"request_id": "abc", "user": "alice"})
    assert_equal(type(derived), "logger")
    assert_equal(derived.level, "debug")
    derived.info("test")

def test_logger_attrs_with_call():
    """Derived logger includes persistent and per-call attrs."""
    req_log = log.logger().attrs({"request_id": "abc"})
    req_log.warn("slow", attrs={"duration_ms": 1500})

# --- .group() ---

def test_logger_group():
    """group() returns a derived logger with nested field namespace."""
    l = log.logger(format="json")
    grouped = l.group("db")
    assert_equal(type(grouped), "logger")
    grouped.info("connected", attrs={"host": "pg.local"})

# --- chaining ---

def test_logger_chaining():
    """attrs/group/attrs chaining works."""
    l = log.logger().attrs({"service": "api"}).group("http").attrs({"method": "GET"})
    l.info("request", attrs={"path": "/health"})

# --- logger methods ---

def test_logger_all_levels():
    """All log levels work on constructed logger."""
    l = log.logger(level="debug")
    l.debug("d", attrs={"k": "v"})
    l.info("i")
    l.warn("w")
    l.error("e")
