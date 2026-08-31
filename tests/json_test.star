# json_test.star - Tests for json module (builder/file pattern)

def test_file_decode_dict():
    """Test json.file(path).decode() with dict."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_dict.json").string
    write_text(p, '{"name":"crsh","count":5}')
    result = json.file(p).decode()
    assert(result["name"] == "crsh", "should decode name")
    assert(result["count"] == 5, "should decode count")
    fs.path(p).remove()

def test_file_decode_list():
    """Test json.file(path).decode() with list."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_list.json").string
    write_text(p, "[1,2,3]")
    result = json.file(p).decode()
    assert(len(result) == 3, "should have 3 elements")
    assert(result[0] == 1, "first element")
    assert(result[2] == 3, "third element")
    fs.path(p).remove()

def test_file_decode_string():
    """Test json.file(path).decode() with string."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_str.json").string
    write_text(p, '"hello"')
    result = json.file(p).decode()
    assert(result == "hello", "should decode string")
    fs.path(p).remove()

def test_file_decode_number():
    """Test json.file(path).decode() with number."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_num.json").string
    write_text(p, "42")
    result = json.file(p).decode()
    assert(result == 42, "should decode int")
    fs.path(p).remove()

def test_file_decode_bool():
    """Test json.file(path).decode() with boolean."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_bool.json").string
    write_text(p, "true")
    result = json.file(p).decode()
    assert(result == True, "should decode true")
    fs.path(p).remove()

def test_file_decode_null():
    """Test json.file(path).decode() with null."""
    p = (fs.path(temp_dir()) / "starkite_json_test_decode_null.json").string
    write_text(p, "null")
    result = json.file(p).decode()
    assert(result == None, "should decode null as None")
    fs.path(p).remove()

def test_file_path_property():
    """Test json.file(path).path returns the path."""
    f = json.file("/some/config.json")
    assert(f.path == "/some/config.json", "path property should return the path")

def test_from_write_file_dict():
    """Test json.source(data).write_file(path) with dict."""
    p = (fs.path(temp_dir()) / "starkite_json_test_write_dict.json").string
    json.source({"name": "crsh"}).write_file(p)
    content = read_text(p)
    assert('"name"' in content, "should contain name key")
    assert('"crsh"' in content, "should contain crsh value")
    fs.path(p).remove()

def test_from_write_file_list():
    """Test json.source(data).write_file(path) with list."""
    p = (fs.path(temp_dir()) / "starkite_json_test_write_list.json").string
    json.source([1, 2, 3]).write_file(p)
    content = read_text(p)
    assert("[1,2,3]" in content, "should contain encoded list")
    fs.path(p).remove()

def test_from_write_file_indent():
    """Test json.source(data).write_file(path, indent='  ') for pretty printing."""
    p = (fs.path(temp_dir()) / "starkite_json_test_write_indent.json").string
    json.source({"a": 1}).write_file(p, indent="  ")
    content = read_text(p)
    assert("\n" in content, "should have newlines")
    assert("  " in content, "should have indentation")
    fs.path(p).remove()

def test_from_data_property():
    """Test json.source(data).data returns the data."""
    data = {"key": "value"}
    w = json.source(data)
    assert(w.data == data, "data property should return the original data")

def test_roundtrip():
    """Test json round-trip: write then decode."""
    p = (fs.path(temp_dir()) / "starkite_json_test_roundtrip.json").string
    original = {"list": [1, 2, 3], "nested": {"key": "value"}, "flag": True}
    json.source(original).write_file(p)
    decoded = json.file(p).decode()
    assert(decoded["list"][0] == 1, "list element preserved")
    assert(decoded["nested"]["key"] == "value", "nested value preserved")
    assert(decoded["flag"] == True, "boolean preserved")
    fs.path(p).remove()

def test_roundtrip_types():
    """Test json round-trip with all types."""
    p = (fs.path(temp_dir()) / "starkite_json_test_roundtrip_types.json").string
    json.source(42).write_file(p)
    assert(json.file(p).decode() == 42, "int roundtrip")
    fs.path(p).remove()

    json.source(True).write_file(p)
    assert(json.file(p).decode() == True, "bool roundtrip")
    fs.path(p).remove()

    json.source("hello").write_file(p)
    assert(json.file(p).decode() == "hello", "string roundtrip")
    fs.path(p).remove()

# ===========================================================================
# try_ variants
# ===========================================================================

def test_try_file_success():
    """json.try_file with valid path -> ok=True."""
    result = json.try_file("/some/path.json")
    assert(result.ok, "try_file should succeed")
    assert(result.value.path == "/some/path.json", "should have correct path")

def test_try_source_success():
    """json.try_source with valid data -> ok=True."""
    result = json.try_source({"key": "value"})
    assert(result.ok, "try_source should succeed")

def test_try_decode_success():
    """json.file(path).try_decode() with valid file -> ok=True."""
    p = (fs.path(temp_dir()) / "starkite_json_test_try_decode.json").string
    write_text(p, '{"key":"value"}')
    result = json.file(p).try_decode()
    assert(result.ok, "try_decode should succeed")
    assert(result.value["key"] == "value", "decoded key should be 'value'")
    fs.path(p).remove()

def test_try_decode_missing_file():
    """json.file(path).try_decode() on missing file -> ok=False."""
    p = (fs.path(temp_dir()) / "nonexistent_json_file_xyz.json").string
    result = json.file(p).try_decode()
    assert(not result.ok, "try_decode on missing file should fail")
    assert(result.error != "", "should have non-empty error")

def test_try_write_file_success():
    """json.source(data).try_write_file(path) -> ok=True."""
    p = (fs.path(temp_dir()) / "starkite_json_test_try_write.json").string
    result = json.source({"key": "value"}).try_write_file(p)
    assert(result.ok, "try_write_file should succeed")
    fs.path(p).remove()
