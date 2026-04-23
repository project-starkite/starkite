# template_test.star - Tests for template module

def test_render_simple():
    """Test template.render with simple substitution."""
    tmpl = "Hello, {{.Name}}!"
    data = {"Name": "World"}
    result = template.render(tmpl, data)
    assert(result == "Hello, World!", "should substitute variable")

def test_render_multiple_vars():
    """Test template.render with multiple variables."""
    tmpl = "{{.First}} and {{.Second}}"
    data = {"First": "one", "Second": "two"}
    result = template.render(tmpl, data)
    assert(result == "one and two", "should substitute all variables")

def test_render_nested():
    """Test template.render with nested data."""
    tmpl = "User: {{.User.Name}}"
    data = {"User": {"Name": "Alice"}}
    result = template.render(tmpl, data)
    assert("Alice" in result, "should access nested name")

def test_render_with_range():
    """Test template.render with range."""
    tmpl = "{{range .Items}}{{.}} {{end}}"
    data = {"Items": ["a", "b", "c"]}
    result = template.render(tmpl, data)
    assert("a" in result, "should include a")
    assert("b" in result, "should include b")
    assert("c" in result, "should include c")

def test_render_with_if():
    """Test template.render with conditional."""
    tmpl = "{{if .Enabled}}ON{{else}}OFF{{end}}"

    result1 = template.render(tmpl, {"Enabled": True})
    assert(result1 == "ON", "should be ON when enabled")

    result2 = template.render(tmpl, {"Enabled": False})
    assert(result2 == "OFF", "should be OFF when disabled")

def test_render_empty_template():
    """Test template.render with empty template."""
    result = template.render("", {})
    assert(result == "", "empty template gives empty result")

def test_render_no_vars():
    """Test template.render with no variables."""
    result = template.render("plain text", {})
    assert(result == "plain text", "plain text unchanged")

def test_render_numeric():
    """Test template.render with numeric values."""
    tmpl = "Count: {{.Count}}"
    data = {"Count": 42}
    result = template.render(tmpl, data)
    assert("42" in result, "should render number")

def test_render_multiline():
    """Test template.render with multiline template."""
    tmpl = """Line 1: {{.A}}
Line 2: {{.B}}"""
    data = {"A": "one", "B": "two"}
    result = template.render(tmpl, data)
    assert("one" in result, "should have line 1")
    assert("two" in result, "should have line 2")

def test_render_any_data():
    """Test legacy template.render accepts any value, not just dict."""
    result = template.render("{{.}}", "hello")
    assert(result == "hello", "should render string data")

    result = template.render("{{.}}", 42)
    assert("42" in result, "should render int data")

    result = template.render("{{range .}}{{.}} {{end}}", ["a", "b", "c"])
    assert("a" in result and "b" in result, "should render list data")

# ============================================================================
# Object API: template.text()
# ============================================================================

def test_text_simple():
    """Test template.text() creates a Template object."""
    t = template.text("Hello, {{.Name}}!")
    assert(type(t) == "template", "should return template type")
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "should render correctly")

def test_text_source_property():
    """Test Template.source property."""
    src = "Hello, {{.Name}}!"
    t = template.text(src)
    assert(t.source == src, "source should match original text")

def test_text_render_any_data():
    """Test Template.render accepts any value."""
    t = template.text("{{.}}")
    assert(t.render("hello") == "hello", "should render string")
    assert("42" in t.render(42), "should render int")

def test_text_render_nested():
    """Test Template.render with nested data."""
    t = template.text("{{.User.Name}}")
    result = t.render({"User": {"Name": "Alice"}})
    assert("Alice" in result, "should access nested data")

def test_text_render_range():
    """Test Template.render with range."""
    t = template.text("{{range .}}{{.}} {{end}}")
    result = t.render(["a", "b", "c"])
    assert("a" in result and "b" in result and "c" in result, "should range over list")

def test_text_render_conditional():
    """Test Template.render with conditional."""
    t = template.text("{{if .Enabled}}ON{{else}}OFF{{end}}")
    assert(t.render({"Enabled": True}) == "ON", "should be ON")
    assert(t.render({"Enabled": False}) == "OFF", "should be OFF")

def test_text_reusable():
    """Test that a Template can be rendered multiple times with different data."""
    t = template.text("Hello, {{.Name}}!")
    assert(t.render({"Name": "Alice"}) == "Hello, Alice!", "first render")
    assert(t.render({"Name": "Bob"}) == "Hello, Bob!", "second render")

# ============================================================================
# Object API: template.file()
# ============================================================================

def test_file_render():
    """Test template.file() reads and renders a template from disk."""
    path = "/tmp/starkite_tmpl_test.txt"
    write_text(path, "Hello, {{.Name}}!")
    t = template.file(path)
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "should render file template")
    assert(t.source == "Hello, {{.Name}}!", "source should match file content")
    fs.path(path).remove()

def test_file_missing():
    """Test template.file() fails on missing file."""
    result = template.try_file("/nonexistent/tmpl.txt")
    assert(not result.ok, "should fail on missing file")
    assert("no such file" in result.error, "error should mention missing file")

# ============================================================================
# Object API: template.bytes()
# ============================================================================

def test_bytes_render():
    """Test template.bytes() creates template from bytes."""
    t = template.bytes(b"Hello, {{.Name}}!")
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "should render from bytes")

def test_bytes_accepts_string():
    """Test template.bytes() also accepts string."""
    t = template.bytes("Hello, {{.Name}}!")
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "should render string passed to bytes()")

# ============================================================================
# Custom delimiters
# ============================================================================

def test_text_custom_delims():
    """Test template.text() with custom delimiters."""
    t = template.text("Hello, <% .Name %>!", delims=("<%", "%>"))
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "should render with custom delims")

def test_text_custom_delims_no_conflict():
    """Test custom delimiters avoid conflicts with default {{ }}."""
    src = 'config: {{ "value" }}\nname: <% .Name %>'
    t = template.text(src, delims=("<%", "%>"))
    result = t.render({"Name": "test"})
    assert('{{ "value" }}' in result, "default delims should be literal")
    assert("name: test" in result, "custom delims should work")

def test_file_custom_delims():
    """Test template.file() with custom delimiters."""
    path = "/tmp/starkite_tmpl_delims.txt"
    write_text(path, "Hello, <% .Name %>!")
    t = template.file(path, delims=("<%", "%>"))
    result = t.render({"Name": "World"})
    assert(result == "Hello, World!", "file template with custom delims")
    fs.path(path).remove()

# ============================================================================
# missing_key option
# ============================================================================

def test_render_missing_key_default():
    """Test missing_key='default' shows <no value>."""
    t = template.text("Hello, {{.Name}}!")
    result = t.render({})
    assert("<no value>" in result, "default should show <no value> for missing key")

def test_render_missing_key_zero():
    """Test missing_key='zero' uses zero value (nil for map, shows <no value>)."""
    t = template.text("Hello, {{.Name}}!")
    # For map[string]interface{}, zero value is nil which renders as <no value>
    # This is standard Go text/template behavior
    result = t.render({}, missing_key="zero")
    assert("<no value>" in result, "zero on map should show <no value> (nil interface)")

def test_render_missing_key_error():
    """Test missing_key='error' causes error."""
    t = template.text("Hello, {{.Name}}!")
    result = t.try_render({}, missing_key="error")
    assert(not result.ok, "error mode should fail on missing key")

def test_render_missing_key_invalid():
    """Test invalid missing_key value is rejected."""
    t = template.text("{{.X}}")
    result = t.try_render({}, missing_key="bogus")
    assert(not result.ok, "should reject invalid missing_key")
    assert("must be" in result.error, "should describe valid values")

# ============================================================================
# try_ variants
# ============================================================================

def test_try_text_parse_error():
    """Test template.try_text() catches parse errors."""
    result = template.try_text("{{if}}")
    assert(not result.ok, "should fail on bad template syntax")

def test_try_text_success():
    """Test template.try_text() returns Template on success."""
    result = template.try_text("Hello, {{.Name}}!")
    assert(result.ok, "should succeed on valid template")
    rendered = result.value.render({"Name": "World"})
    assert(rendered == "Hello, World!", "should render from try_ result")

def test_try_render_success():
    """Test Template.try_render() on valid data."""
    t = template.text("Hello, {{.Name}}!")
    result = t.try_render({"Name": "World"})
    assert(result.ok, "try_render should succeed")
    assert(result.value == "Hello, World!", "try_render value should match")

def test_try_render_error():
    """Test Template.try_render() on error."""
    t = template.text("{{.Name}}")
    result = t.try_render({}, missing_key="error")
    assert(not result.ok, "try_render should capture error")
    assert(result.error != "", "try_render should have error message")

def test_try_file_success():
    """Test template.try_file() on existing file."""
    path = "/tmp/starkite_tmpl_try.txt"
    write_text(path, "{{.X}}")
    result = template.try_file(path)
    assert(result.ok, "try_file should succeed")
    assert(result.value.render({"X": "yes"}) == "yes", "should render")
    fs.path(path).remove()

def test_try_bytes_parse_error():
    """Test template.try_bytes() catches parse errors."""
    result = template.try_bytes(b"{{if}}")
    assert(not result.ok, "should fail on bad syntax in bytes")

# ============================================================================
# Template repr and type
# ============================================================================

def test_template_repr():
    """Test string representation."""
    t = template.text("Hello")
    s = str(t)
    assert("template(" in s, "repr should contain 'template('")

def test_template_type():
    """Test type()."""
    t = template.text("Hello")
    assert(type(t) == "template", "type should be 'template'")
