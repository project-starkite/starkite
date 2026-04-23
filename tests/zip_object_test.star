# zip_object_test.star - Tests for zip file-oriented object API

# ============================================================================
# zip.file().write + zip.file().read — Round-trip
# ============================================================================

def test_write_and_read_single():
    """Test writing a single file and reading it back."""
    src = "/tmp/starkite_zip_src.txt"
    arc = "/tmp/starkite_zip_test1.zip"
    write_text(src, "hello zip")
    zip.file(arc).write(src)
    content = zip.file(arc).read("starkite_zip_src.txt")
    assert(str(content) == "hello zip", "should read back written content")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_write_with_name_override():
    """Test writing with custom entry name."""
    src = "/tmp/starkite_zip_src2.txt"
    arc = "/tmp/starkite_zip_test2.zip"
    write_text(src, "custom name content")
    zip.file(arc).write(src, name="custom.txt")
    content = zip.file(arc).read("custom.txt")
    assert(str(content) == "custom name content", "should use custom name")
    fs.path(src).remove()
    fs.path(arc).remove()

# ============================================================================
# zip.file().namelist
# ============================================================================

def test_namelist():
    """Test namelist returns all entry names."""
    src1 = "/tmp/starkite_zip_nl1.txt"
    src2 = "/tmp/starkite_zip_nl2.txt"
    arc = "/tmp/starkite_zip_nl.zip"
    write_text(src1, "one")
    write_text(src2, "two")
    zip.file(arc).write_all(files=[src1, src2])
    names = zip.file(arc).namelist()
    assert(len(names) == 2, "should have 2 entries, got %d" % len(names))
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(arc).remove()

def test_namelist_with_match():
    """Test namelist with match filter."""
    src1 = "/tmp/starkite_zip_nm1.txt"
    src2 = "/tmp/starkite_zip_nm2.go"
    arc = "/tmp/starkite_zip_nm.zip"
    write_text(src1, "one")
    write_text(src2, "two")
    zip.file(arc).write_all(files=[src1, src2])
    names = zip.file(arc).namelist(match="*.txt")
    assert(len(names) == 1, "should have 1 .txt entry, got %d" % len(names))
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(arc).remove()

# ============================================================================
# zip.file().read_all
# ============================================================================

def test_read_all():
    """Test read_all returns all entries."""
    src1 = "/tmp/starkite_zip_ra1.txt"
    src2 = "/tmp/starkite_zip_ra2.txt"
    arc = "/tmp/starkite_zip_ra.zip"
    write_text(src1, "alpha")
    write_text(src2, "beta")
    zip.file(arc).write_all(files=[src1, src2])
    all = zip.file(arc).read_all()
    assert(len(all) == 2, "should have 2 entries")
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(arc).remove()

def test_read_all_with_match():
    """Test read_all with match filter."""
    src1 = "/tmp/starkite_zip_ram1.txt"
    src2 = "/tmp/starkite_zip_ram2.go"
    arc = "/tmp/starkite_zip_ram.zip"
    write_text(src1, "one")
    write_text(src2, "two")
    zip.file(arc).write_all(files=[src1, src2])
    filtered = zip.file(arc).read_all(match="*.txt")
    assert(len(filtered) == 1, "should have 1 .txt entry")
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(arc).remove()

def test_read_all_with_files():
    """Test read_all with files filter."""
    src1 = "/tmp/starkite_zip_raf1.txt"
    src2 = "/tmp/starkite_zip_raf2.txt"
    src3 = "/tmp/starkite_zip_raf3.txt"
    arc = "/tmp/starkite_zip_raf.zip"
    write_text(src1, "one")
    write_text(src2, "two")
    write_text(src3, "three")
    zip.file(arc).write_all(files=[src1, src2, src3])
    # Read only two specific entries (entry names are full paths from write_all)
    filtered = zip.file(arc).read_all(files=[src1, src3])
    assert(len(filtered) == 2, "should have 2 selected entries")
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(src3).remove()
    fs.path(arc).remove()

# ============================================================================
# zip.file().write_all
# ============================================================================

def test_write_all_with_files():
    """Test write_all with files list."""
    src1 = "/tmp/starkite_zip_waf1.txt"
    src2 = "/tmp/starkite_zip_waf2.txt"
    arc = "/tmp/starkite_zip_waf.zip"
    write_text(src1, "file1")
    write_text(src2, "file2")
    zip.file(arc).write_all(files=[src1, src2])
    names = zip.file(arc).namelist()
    assert(len(names) == 2, "should have 2 entries")
    fs.path(src1).remove()
    fs.path(src2).remove()
    fs.path(arc).remove()

def test_write_all_with_base_dir():
    """Test write_all with base_dir strips prefix."""
    src = "/tmp/starkite_zip_bd/sub/file.txt"
    arc = "/tmp/starkite_zip_bd.zip"
    # Create directory structure
    fs.path("/tmp/starkite_zip_bd/sub").mkdir(parents=True)
    write_text(src, "nested content")
    zip.file(arc).write_all(files=[src], base_dir="/tmp/starkite_zip_bd")
    names = zip.file(arc).namelist()
    assert(len(names) == 1, "should have 1 entry")
    assert(names[0] == "sub/file.txt", "entry should be relative to base_dir, got: " + names[0])
    fs.path(arc).remove()
    fs.path(src).remove()
    fs.path("/tmp/starkite_zip_bd/sub").remove()
    fs.path("/tmp/starkite_zip_bd").remove()

# ============================================================================
# try_ variants
# ============================================================================

def test_try_read_success():
    """Test try_read on valid archive."""
    src = "/tmp/starkite_zip_tr.txt"
    arc = "/tmp/starkite_zip_tr.zip"
    write_text(src, "try data")
    zip.file(arc).write(src)
    result = zip.file(arc).try_read("starkite_zip_tr.txt")
    assert(result.ok, "try_read should succeed")
    assert(str(result.value) == "try data", "should have correct content")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_try_read_missing_entry():
    """Test try_read on missing entry."""
    src = "/tmp/starkite_zip_trm.txt"
    arc = "/tmp/starkite_zip_trm.zip"
    write_text(src, "data")
    zip.file(arc).write(src)
    result = zip.file(arc).try_read("nonexistent.txt")
    assert(not result.ok, "try_read should fail on missing entry")
    assert(result.error != "", "should have error message")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_try_namelist_missing_archive():
    """Test try_namelist on missing archive."""
    result = zip.file("/nonexistent/archive.zip").try_namelist()
    assert(not result.ok, "try_namelist should fail on missing archive")

def test_try_write_all_success():
    """Test try_write_all succeeds."""
    src = "/tmp/starkite_zip_twa.txt"
    arc = "/tmp/starkite_zip_twa.zip"
    write_text(src, "try write all")
    result = zip.file(arc).try_write_all(files=[src])
    assert(result.ok, "try_write_all should succeed")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_try_file_success():
    """Test zip.try_file() returns Result."""
    result = zip.try_file("/tmp/test.zip")
    assert(result.ok, "try_file should succeed (just stores path)")
    assert(type(result.value) == "zip.archive", "should return archive")

# ============================================================================
# zip.file type
# ============================================================================

def test_archive_type():
    """Test archive type name."""
    a = zip.file("/tmp/test.zip")
    assert(type(a) == "zip.archive", "type should be zip.archive")

def test_archive_repr():
    """Test archive string representation."""
    a = zip.file("/tmp/test.zip")
    s = str(a)
    assert("zip.file" in s, "repr should contain zip.file")
    assert("/tmp/test.zip" in s, "repr should contain path")

def test_archive_truth():
    """Test archive truthiness."""
    a = zip.file("/tmp/test.zip")
    if a:
        pass
    else:
        assert(False, "archive with path should be truthy")

# ============================================================================
# Error cases
# ============================================================================

def test_read_missing_archive():
    """Test read on non-existent archive."""
    result = zip.file("/nonexistent.zip").try_read("a.txt")
    assert(not result.ok, "read on missing archive should fail")

def test_write_all_match_and_files_exclusive():
    """Test write_all rejects both match and files."""
    result = zip.file("/tmp/test.zip").try_write_all(match="*.txt", files=["a.txt"])
    assert(not result.ok, "should reject match+files together")
