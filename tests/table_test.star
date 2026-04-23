# table_test.star - Tests for table module

def test_table_new():
    """Test table.new creates a table object."""
    t = table.new(["A", "B"])
    assert(t != None, "table.new should return table object")

def test_table_add_row():
    """Test table.add_row."""
    t = table.new(["NAME", "VALUE"])
    t.add_row("foo", "1")
    t.add_row("bar", "2")
    assert(t.row_count == 2, "should have 2 rows")

def test_table_render():
    """Test table.render returns string."""
    t = table.new(["A", "B"])
    t.add_row("1", "2")
    result = t.render()
    assert("A" in result, "should contain header A")
    assert("B" in result, "should contain header B")
    assert("1" in result, "should contain value 1")
    assert("2" in result, "should contain value 2")

def test_table_render_basic():
    """Test table render with basic data."""
    t = table.new(["NAME", "VALUE"])
    t.add_row("foo", "1")
    t.add_row("bar", "2")
    result = t.render()
    assert("NAME" in result, "should contain NAME header")
    assert("foo" in result, "should contain foo")
    assert("bar" in result, "should contain bar")

def test_table_render_empty_rows():
    """Test table render with empty rows."""
    t = table.new(["A", "B", "C"])
    result = t.render()
    assert("A" in result, "should contain header A")
    assert("B" in result, "should contain header B")
    assert("C" in result, "should contain header C")

def test_table_render_single_row():
    """Test table render with single row."""
    t = table.new(["HEADER"])
    t.add_row("value")
    result = t.render()
    assert("HEADER" in result, "should contain header")
    assert("value" in result, "should contain value")

def test_table_render_many_columns():
    """Test table render with many columns."""
    t = table.new(["A", "B", "C", "D", "E"])
    t.add_row("1", "2", "3", "4", "5")
    t.add_row("a", "b", "c", "d", "e")
    result = t.render()
    assert("A" in result, "should contain A")
    assert("E" in result, "should contain E")
    assert("1" in result, "should contain 1")
    assert("e" in result, "should contain e")

def test_table_render_long_values():
    """Test table render with long values."""
    t = table.new(["NAME", "DESCRIPTION"])
    t.add_row("short", "This is a longer description")
    result = t.render()
    assert("short" in result, "should contain short")
    assert("longer" in result, "should contain longer")
