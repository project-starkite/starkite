# strings_test.star - Tests for strings module

def test_ljust():
    """Test strings.ljust pads on the right."""
    assert(strings.ljust("hi", 5) == "hi   ", "should pad with spaces")
    assert(strings.ljust("hi", 5, ".") == "hi...", "should pad with fillchar")
    assert(strings.ljust("hello", 3) == "hello", "already wider stays unchanged")

def test_rjust():
    """Test strings.rjust pads on the left."""
    assert(strings.rjust("hi", 5) == "   hi", "should pad with spaces")
    assert(strings.rjust("hi", 5, ".") == "...hi", "should pad with fillchar")
    assert(strings.rjust("hello", 3) == "hello", "already wider stays unchanged")

def test_center():
    """Test strings.center pads both sides."""
    assert(strings.center("hi", 6) == "  hi  ", "should center with spaces")
    assert(strings.center("hi", 7) == "  hi   ", "odd padding puts extra on right")
    assert(strings.center("hi", 6, "-") == "--hi--", "should center with fillchar")
    assert(strings.center("hello", 3) == "hello", "already wider stays unchanged")

def test_cut():
    """Test strings.cut splits at first separator."""
    before, after, found = strings.cut("hello=world", "=")
    assert(found, "should find separator")
    assert(before == "hello", "before should be 'hello'")
    assert(after == "world", "after should be 'world'")

def test_cut_not_found():
    """Test strings.cut when separator is missing."""
    before, after, found = strings.cut("hello", "=")
    assert(not found, "should not find separator")
    assert(before == "hello", "before should be original string")
    assert(after == "", "after should be empty")

def test_equal():
    """Test strings.equal for case-insensitive comparison."""
    assert(strings.equal("Hello", "hello"), "should match case-insensitively")
    assert(strings.equal("ABC", "abc"), "should match case-insensitively")
    assert(not strings.equal("hello", "world"), "different strings should not match")

def test_has_any():
    """Test strings.has_any checks for any char."""
    assert(strings.has_any("hello", "aeiou"), "should find vowels")
    assert(not strings.has_any("rhythm", "aeiou"), "should not find vowels")

def test_quote():
    """Test strings.quote adds Go-style quoting."""
    result = strings.quote("hello\tworld")
    assert(result == '"hello\\tworld"', "should quote with escapes")

def test_unquote():
    """Test strings.unquote removes Go-style quoting."""
    result = strings.unquote('"hello\\tworld"')
    assert(result == "hello\tworld", "should unquote with escapes")

def test_quote_unquote_roundtrip():
    """Test quote/unquote roundtrip."""
    original = "line1\nline2\ttab"
    assert(strings.unquote(strings.quote(original)) == original, "roundtrip should preserve string")
