# regexp_test.star - Tests for regexp module

# --- match (bool) - unchanged return type ---

def test_match_simple():
    """Test regexp.match with simple pattern."""
    assert(regexp.match("hello", "hello world"), "should match substring")
    assert(not regexp.match("hello", "goodbye world"), "should not match")

def test_match_digits():
    """Test regexp.match with digit pattern."""
    assert(regexp.match(r"\d+", "abc123def"), "should match digits")
    assert(not regexp.match(r"\d+", "abcdef"), "should not match without digits")

def test_match_start():
    """Test regexp.match with start anchor."""
    assert(regexp.match("^hello", "hello world"), "should match at start")
    assert(not regexp.match("^hello", "say hello"), "should not match in middle")

def test_match_end():
    """Test regexp.match with end anchor."""
    assert(regexp.match("world$", "hello world"), "should match at end")
    assert(not regexp.match("world$", "world hello"), "should not match at start")

# --- find (now returns Match | None) ---

def test_find():
    """Test regexp.find returns Match object."""
    result = regexp.find(r"\d+", "abc123def456")
    assert(result != None, "should find a match")
    assert(result.text == "123", "match text should be first match")
    assert(result.start == 3, "start should be 3")
    assert(result.end == 6, "end should be 6")

def test_find_no_match():
    """Test regexp.find with no match."""
    result = regexp.find(r"\d+", "abcdef")
    assert(result == None, "should return None when no match")

def test_find_with_groups():
    """Test regexp.find with capture groups."""
    m = regexp.find(r"(\d+)-(\w+)", "id:123-abc")
    assert(m.text == "123-abc", "full match text")
    assert(m.group(n=0) == "123-abc", "group 0 is full match")
    assert(m.group(n=1) == "123", "group 1")
    assert(m.group(n=2) == "abc", "group 2")

def test_find_named_groups():
    """Test regexp.find with named capture groups."""
    m = regexp.find(r"(?P<user>\w+)@(?P<host>\w+)", "user@host")
    assert(m.group(name="user") == "user", "named group 'user'")
    assert(m.group(name="host") == "host", "named group 'host'")

def test_find_empty_match():
    """Test that empty match is returned (not None)."""
    m = regexp.find("a*", "b")
    assert(m != None, "empty match should not be None")
    assert(m.text == "", "empty match text should be empty string")

# --- find_all (now returns list[Match]) ---

def test_find_all():
    """Test regexp.find_all returns list of Match."""
    results = regexp.find_all(r"\d+", "a1b2c3")
    assert(len(results) == 3, "should find all matches")
    assert(results[0].text == "1", "first match text")
    assert(results[1].text == "2", "second match text")
    assert(results[2].text == "3", "third match text")

def test_find_all_no_match():
    """Test regexp.find_all with no matches."""
    results = regexp.find_all(r"\d+", "abc")
    assert(len(results) == 0, "should return empty list")

def test_find_all_with_n():
    """Test regexp.find_all with limit."""
    results = regexp.find_all(r"\d+", "a1b2c3", n=2)
    assert(len(results) == 2, "should limit to 2 matches")

# --- replace (string) - unchanged return type ---

def test_replace_all():
    """Test regexp.replace (replaces all occurrences)."""
    result = regexp.replace(r"\d", "a1b2c3", "X")
    assert(result == "aXbXcX", "should replace all digits")

def test_replace_groups():
    """Test regexp.replace with capture groups."""
    result = regexp.replace(r"(\w+)@(\w+)", "user@host", "$1 at $2")
    assert(result == "user at host", "should use capture groups")

# --- split (list[string]) - unchanged return type ---

def test_split():
    """Test regexp.split."""
    parts = regexp.split(r"\s+", "a  b   c")
    assert(len(parts) == 3, "should split on whitespace")
    assert(parts[0] == "a", "first part")
    assert(parts[1] == "b", "second part")
    assert(parts[2] == "c", "third part")

def test_split_no_match():
    """Test regexp.split with no matching delimiter."""
    parts = regexp.split(r"\d", "abc")
    assert(len(parts) == 1, "should return original as single element")
    assert(parts[0] == "abc", "should be original string")

def test_split_with_n():
    """Test regexp.split with limit."""
    parts = regexp.split(r",", "a,b,c,d", n=2)
    assert(len(parts) == 2, "should limit splits")
    assert(parts[0] == "a", "first part")
    assert(parts[1] == "b,c,d", "remaining unsplit")

# --- existing pattern tests ---

def test_word_boundary():
    """Test regexp with word boundary."""
    assert(regexp.match(r"\bword\b", "a word here"), "should match whole word")
    assert(not regexp.match(r"\bword\b", "awordhere"), "should not match within word")

def test_case_insensitive():
    """Test regexp with case insensitive flag inline."""
    assert(regexp.match("(?i)hello", "HELLO"), "should match case insensitive")

def test_special_chars():
    """Test regexp with escaped special chars."""
    assert(regexp.match(r"\.", "a.b"), "should match literal dot")
    assert(regexp.match(r"\$\d+", "$100"), "should match dollar sign and digits")

# --- Match attributes ---

def test_match_attrs():
    """Test Match text, start, end attributes."""
    m = regexp.find(r"\d+", "abc123")
    assert(m.text == "123", "text")
    assert(m.start == 3, "start")
    assert(m.end == 6, "end")

def test_match_groups():
    """Test Match groups tuple."""
    m = regexp.find(r"(\d+)-(\w+)", "id:123-abc")
    groups = m.groups
    assert(len(groups) == 3, "groups includes full match + 2 captures")
    assert(groups[0] == "123-abc", "groups[0] is full match")
    assert(groups[1] == "123", "groups[1] is first capture")
    assert(groups[2] == "abc", "groups[2] is second capture")

def test_match_group_by_index():
    """Test group() by index."""
    m = regexp.find(r"(\d+)-(\w+)", "id:123-abc")
    assert(m.group() == "123-abc", "group() default n=0")
    assert(m.group(n=1) == "123", "group(n=1)")
    assert(m.group(n=2) == "abc", "group(n=2)")

def test_match_group_by_name():
    """Test group() by name."""
    m = regexp.find(r"(?P<year>\d{4})-(?P<month>\d{2})", "date: 2026-03")
    assert(m.group(name="year") == "2026", "named group 'year'")
    assert(m.group(name="month") == "03", "named group 'month'")

def test_match_unmatched_optional_group():
    """Test unmatched optional group returns None."""
    m = regexp.find(r"a(b)?c", "ac")
    assert(m != None, "should match")
    assert(m.text == "ac", "full match")
    assert(m.group(n=1) == None, "unmatched optional group is None")
    groups = m.groups
    assert(groups[1] == None, "unmatched group in tuple is None")

# --- compile() / Pattern ---

def test_compile_basic():
    """Test regexp.compile returns Pattern."""
    p = regexp.compile(r"\d+")
    assert(p.pattern == r"\d+", "pattern attr")

def test_compile_group_info():
    """Test Pattern group_count and group_names."""
    p = regexp.compile(r"(?P<year>\d{4})-(?P<month>\d{2})")
    assert(p.group_count == 2, "group_count")
    names = p.group_names
    assert(len(names) == 2, "two named groups")
    assert(names[0] == "year", "first name")
    assert(names[1] == "month", "second name")

def test_pattern_match():
    """Test Pattern.match method."""
    p = regexp.compile(r"\d+")
    assert(p.match("abc123"), "should match")
    assert(not p.match("abc"), "should not match")

def test_pattern_find():
    """Test Pattern.find method."""
    p = regexp.compile(r"(?P<year>\d{4})-(?P<month>\d{2})")
    m = p.find("date: 2026-03")
    assert(m != None, "should find match")
    assert(m.group(name="year") == "2026", "year group")
    assert(m.group(name="month") == "03", "month group")

def test_pattern_find_all():
    """Test Pattern.find_all method."""
    p = regexp.compile(r"\d+")
    results = p.find_all("a1b2c3")
    assert(len(results) == 3, "should find 3")
    assert(results[0].text == "1", "first")

def test_pattern_replace():
    """Test Pattern.replace method."""
    p = regexp.compile(r"\d")
    result = p.replace("a1b2c3", "X")
    assert(result == "aXbXcX", "should replace all")

def test_pattern_split():
    """Test Pattern.split method."""
    p = regexp.compile(r",")
    parts = p.split("a,b,c")
    assert(len(parts) == 3, "should split into 3")
    assert(parts[1] == "b", "middle element")

# --- flags kwarg ---

def test_flags_case_insensitive():
    """Test flags='i' for case insensitive matching."""
    assert(regexp.match("hello", "HELLO", flags="i"), "case insensitive match")
    m = regexp.find("hello", "HELLO WORLD", flags="i")
    assert(m != None, "case insensitive find")
    assert(m.text == "HELLO", "matched text preserves original case")

def test_flags_multiline():
    """Test flags='m' for multiline matching."""
    m = regexp.find("^line2", "line1\nline2", flags="m")
    assert(m != None, "multiline should match ^line2")
    assert(m.text == "line2", "matched text")

def test_flags_dotall():
    """Test flags='s' for dotall (dot matches newline)."""
    m = regexp.find("a.b", "a\nb", flags="s")
    assert(m != None, "dotall should match across newline")
    assert(m.text == "a\nb", "matched text")

def test_flags_ungreedy():
    """Test flags='U' for ungreedy matching."""
    m = regexp.find("a.+b", "aXXb aYb", flags="U")
    assert(m != None, "ungreedy match")
    # With ungreedy, .+ matches as few chars as possible
    # but since there's only one possible match starting at position 0, it depends on the engine
    # In Go, (?U)a.+b on "aXXb aYb" matches "aXXb" (shortest from first a to first b)
    assert(m.text == "aXXb", "ungreedy match text")

def test_flags_invalid():
    """Test that invalid flags produce an error."""
    r = regexp.try_match("hello", "hello", flags="z")
    assert(not r.ok, "invalid flag should error")

def test_compile_with_flags():
    """Test regexp.compile with flags kwarg."""
    p = regexp.compile("hello", flags="i")
    assert(p.match("HELLO"), "compiled with case insensitive flag")

# --- try_ support ---

def test_try_compile_invalid():
    """Test try_compile with invalid pattern."""
    r = regexp.try_compile("[invalid")
    assert(not r.ok, "should fail")
    assert("error parsing regexp" in r.error, "should have parse error")

def test_try_compile_valid():
    """Test try_compile with valid pattern."""
    r = regexp.try_compile(r"\d+")
    assert(r.ok, "should succeed")

def test_try_find():
    """Test try_find at module level."""
    r = regexp.try_find(r"\d+", "abc123")
    assert(r.ok, "should succeed")

def test_try_match():
    """Test try_match at module level."""
    r = regexp.try_match("[invalid", "test")
    assert(not r.ok, "invalid pattern should error")

def test_pattern_try_find():
    """Test try_find on Pattern object."""
    p = regexp.compile(r"\d+")
    r = p.try_find("abc123")
    assert(r.ok, "should succeed")

# --- str() and print compatibility ---

def test_match_str():
    """Test that str(match) returns the matched text."""
    m = regexp.find(r"\d+", "abc123")
    assert(str(m) == "123", "str(match) should be the matched text")
