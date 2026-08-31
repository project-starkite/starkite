# zip_object_test.star - Tests for zip file-oriented object API

# ============================================================================
# zip.file().write + zip.file().read — Round-trip
# ============================================================================

def test_write_and_read_single():
    """Test writing a single file and reading it back."""
    src = (fs.path(temp_dir()) / "starkite_zip_src.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_test1.zip").string
    write_text(src, "hello zip")
    zip.file(arc).write(src)
    entry_name = fs.path(src).name
    content = zip.file(arc).read(entry_name)
    assert(str(content) == "hello zip", "should read back written content")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_write_with_name_override():
    """Test writing with custom entry name."""
    src = (fs.path(temp_dir()) / "starkite_zip_src2.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_test2.zip").string
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_nl1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_nl2.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_nl.zip").string
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_nm1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_nm2.go").string
    arc = (fs.path(temp_dir()) / "starkite_zip_nm.zip").string
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_ra1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_ra2.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_ra.zip").string
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_ram1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_ram2.go").string
    arc = (fs.path(temp_dir()) / "starkite_zip_ram.zip").string
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_raf1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_raf2.txt").string
    src3 = (fs.path(temp_dir()) / "starkite_zip_raf3.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_raf.zip").string
    write_text(src1, "one")
    write_text(src2, "two")
    write_text(src3, "three")
    zip.file(arc).write_all(files=[src1, src2, src3])
    names = zip.file(arc).namelist()
    filtered = zip.file(arc).read_all(files=[names[0], names[2]])
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
    src1 = (fs.path(temp_dir()) / "starkite_zip_waf1.txt").string
    src2 = (fs.path(temp_dir()) / "starkite_zip_waf2.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_waf.zip").string
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
    base_d = (fs.path(temp_dir()) / "starkite_zip_bd").string
    sub_d = (fs.path(base_d) / "sub").string
    src = (fs.path(sub_d) / "file.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_bd.zip").string
    # Create directory structure
    fs.path(sub_d).mkdir(parents=True)
    write_text(src, "nested content")
    zip.file(arc).write_all(files=[src], base_dir=base_d)
    names = zip.file(arc).namelist()
    assert(len(names) == 1, "should have 1 entry")
    entry_name = names[0].replace("\\", "/")
    assert(entry_name == "sub/file.txt", "entry should be relative to base_dir, got: " + names[0])
    fs.path(arc).remove()
    fs.path(src).remove()
    fs.path(sub_d).remove()
    fs.path(base_d).remove()

# ============================================================================
# try_ variants
# ============================================================================

def test_try_read_success():
    """Test try_read on valid archive."""
    src = (fs.path(temp_dir()) / "starkite_zip_tr.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_tr.zip").string
    write_text(src, "try data")
    zip.file(arc).write(src)
    entry_name = fs.path(src).name
    result = zip.file(arc).try_read(entry_name)
    assert(result.ok, "try_read should succeed")
    assert(str(result.value) == "try data", "should have correct content")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_try_read_missing_entry():
    """Test try_read on missing entry."""
    src = (fs.path(temp_dir()) / "starkite_zip_trm.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_trm.zip").string
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
    src = (fs.path(temp_dir()) / "starkite_zip_twa.txt").string
    arc = (fs.path(temp_dir()) / "starkite_zip_twa.zip").string
    write_text(src, "try write all")
    result = zip.file(arc).try_write_all(files=[src])
    assert(result.ok, "try_write_all should succeed")
    fs.path(src).remove()
    fs.path(arc).remove()

def test_try_file_success():
    """Test zip.try_file() returns Result."""
    result = zip.try_file("test.zip")
    assert(result.ok, "try_file should succeed (just stores path)")
    assert(type(result.value) == "zip.archive", "should return archive")

# ============================================================================
# zip.file type
# ============================================================================

def test_archive_type():
    """Test archive type name."""
    a = zip.file("test.zip")
    assert(type(a) == "zip.archive", "type should be zip.archive")

def test_archive_repr():
    """Test archive string representation."""
    a = zip.file("test.zip")
    s = str(a)
    assert("zip.file" in s, "repr should contain zip.file")
    assert("test.zip" in s, "repr should contain path")

def test_archive_truth():
    """Test archive truthiness."""
    a = zip.file("test.zip")
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
    result = zip.file("test.zip").try_write_all(match="*.txt", files=["a.txt"])
    assert(not result.ok, "should reject match+files together")
