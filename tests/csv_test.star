# csv_test.star - Tests for csv module (builder/file pattern)

def test_file_read_basic():
    """Test csv.file(path).read() basic parsing."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_basic.csv").string
    write_text(p, "name,value\nfoo,1\nbar,2")
    rows = csv.file(p).read()
    assert(len(rows) == 3, "should have 3 rows")
    assert(rows[0][0] == "name", "first header should be 'name'")
    assert(rows[1][0] == "foo", "first data row should be 'foo'")
    assert(rows[2][1] == "2", "second data row value should be '2'")
    fs.path(p).remove()

def test_file_read_header():
    """Test csv.file(path).read(header=True) returns list of dicts."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_header.csv").string
    write_text(p, "name,value\nfoo,1\nbar,2")
    records = csv.file(p).read(header=True)
    assert(len(records) == 2, "should have 2 records")
    assert(records[0]["name"] == "foo", "first record name should be 'foo'")
    assert(records[0]["value"] == "1", "first record value should be '1'")
    assert(records[1]["name"] == "bar", "second record name should be 'bar'")
    fs.path(p).remove()

def test_file_read_skip():
    """Test csv.file(path).read(skip=1) skips rows."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_skip.csv").string
    write_text(p, "name,value\nfoo,1\nbar,2")
    rows = csv.file(p).read(skip=1)
    assert(len(rows) == 2, "should have 2 rows after skip")
    assert(rows[0][0] == "foo", "first row should be 'foo'")
    fs.path(p).remove()

def test_file_read_custom_delimiter():
    """Test csv.file(path).read(sep=';') with custom delimiter."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_delim.csv").string
    write_text(p, "name;value\nfoo;1")
    rows = csv.file(p).read(sep=";")
    assert(rows[0][0] == "name", "should parse semicolon delimiter")
    assert(rows[1][1] == "1", "should parse value correctly")
    fs.path(p).remove()

def test_file_read_comment():
    """Test csv.file(path).read(comment='#') skips comment lines."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_comment.csv").string
    write_text(p, "name,value\n#comment\nfoo,1")
    rows = csv.file(p).read(comment="#")
    assert(len(rows) == 2, "should skip comment line")
    fs.path(p).remove()

def test_file_path_property():
    """Test csv.file(path).path returns the path."""
    f = csv.file("/some/path.csv")
    assert(f.path == "/some/path.csv", "path property should return the path")

def test_from_write_file_list_of_lists():
    """Test csv.source(data).write_file(path) with list of lists."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_write_lol.csv").string
    csv.source([["a", "b"], ["1", "2"]]).write_file(p)
    content = read_text(p)
    assert("a,b" in content, "should contain header row")
    assert("1,2" in content, "should contain data row")
    fs.path(p).remove()

def test_from_write_file_list_of_dicts():
    """Test csv.source(data).write_file(path) with list of dicts."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_write_lod.csv").string
    csv.source([{"name": "foo", "value": "1"}]).write_file(p)
    content = read_text(p)
    assert("name" in content, "should contain header")
    assert("foo" in content, "should contain data")
    fs.path(p).remove()

def test_from_write_file_custom_delimiter():
    """Test csv.source(data).write_file(path, sep=';')."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_write_sep.csv").string
    csv.source([["a", "b"], ["1", "2"]]).write_file(p, sep=";")
    content = read_text(p)
    assert("a;b" in content, "should use semicolon delimiter")
    fs.path(p).remove()

def test_from_write_file_explicit_headers():
    """Test csv.source(data).write_file(path, headers=[...])."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_write_headers.csv").string
    csv.source([{"name": "foo", "extra": "x"}]).write_file(p, headers=["name"])
    content = read_text(p)
    lines = content.strip().split("\n")
    assert(lines[0] == "name", "should only have specified header")
    assert(lines[1] == "foo", "should have data row")
    fs.path(p).remove()

def test_from_data_property():
    """Test csv.source(data).data returns the data."""
    data = [["a", "b"]]
    w = csv.source(data)
    assert(w.data == data, "data property should return the original data")

def test_roundtrip():
    """Test csv round-trip: write then read."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_roundtrip.csv").string
    original = [["name", "age", "city"], ["Alice", "30", "NYC"], ["Bob", "25", "LA"]]
    csv.source(original).write_file(p)
    parsed = csv.file(p).read()
    assert(parsed == original, "round-trip should preserve data")
    fs.path(p).remove()

def test_roundtrip_dict():
    """Test csv round-trip with dicts."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_roundtrip_dict.csv").string
    csv.source([{"name": "Alice", "age": "30"}, {"name": "Bob", "age": "25"}]).write_file(p)
    records = csv.file(p).read(header=True)
    assert(len(records) == 2, "should have 2 records")
    assert(records[0]["name"] == "Alice", "first record name")
    assert(records[1]["age"] == "25", "second record age")
    fs.path(p).remove()

# ===========================================================================
# try_ variants
# ===========================================================================

def test_try_file_success():
    """csv.try_file with valid path -> ok=True."""
    result = csv.try_file("/tmp/test.csv")
    assert(result.ok, "try_file should succeed")
    assert(result.value.path == "/tmp/test.csv", "should have correct path")

def test_try_source_success():
    """csv.try_source with valid list -> ok=True."""
    result = csv.try_source([["a", "b"]])
    assert(result.ok, "try_source should succeed")

def test_try_source_bad_type():
    """csv.try_source with non-list -> ok=False."""
    result = csv.try_source("not a list")
    assert(not result.ok, "try_source with string arg should fail")
    assert(result.error != "", "should have non-empty error")

def test_try_read_success():
    """csv.file(path).try_read() with valid file -> ok=True."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_try_read.csv").string
    write_text(p, "a,b\n1,2")
    result = csv.file(p).try_read()
    assert(result.ok, "try_read should succeed")
    assert(len(result.value) == 2, "should have 2 rows")
    fs.path(p).remove()

def test_try_read_missing_file():
    """csv.file(path).try_read() on missing file -> ok=False."""
    p = (fs.path(temp_dir()) / "nonexistent_csv_file_xyz.csv").string
    result = csv.file(p).try_read()
    assert(not result.ok, "try_read on missing file should fail")
    assert(result.error != "", "should have non-empty error")

def test_try_write_file_success():
    """csv.source(data).try_write_file(path) -> ok=True."""
    p = (fs.path(temp_dir()) / "starkite_csv_test_try_write.csv").string
    result = csv.source([["a", "b"]]).try_write_file(p)
    assert(result.ok, "try_write_file should succeed")
    fs.path(p).remove()
