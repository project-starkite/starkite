# gzip_object_test.star - Tests for gzip object API

# ============================================================================
# gzip.text() - Source from string
# ============================================================================

def test_text_compress_decompress():
    """Test gzip.text() compress/decompress round-trip."""
    original = "Hello, World!"
    compressed = gzip.text(original).compress()
    assert(type(compressed) == "bytes", "compress should return bytes")
    decompressed = gzip.bytes(compressed).decompress()
    assert(type(decompressed) == "bytes", "decompress should return bytes")
    assert(str(decompressed) == original, "round-trip should preserve data")

def test_text_compress_level():
    """Test gzip.text().compress(level=N)."""
    text = "x" * 1000
    c1 = gzip.text(text).compress(level=1)
    c9 = gzip.text(text).compress(level=9)
    assert(len(c1) > 0, "level 1 should produce output")
    assert(len(c9) > 0, "level 9 should produce output")
    assert(len(c9) <= len(c1), "level 9 should be same or smaller than level 1")

def test_text_compress_kwarg_level():
    """Test gzip.text().compress(level=9) with kwarg."""
    text = "test data"
    compressed = gzip.text(text).compress(level=9)
    decompressed = gzip.bytes(compressed).decompress()
    assert(str(decompressed) == text, "kwarg level should work")

def test_text_source_data_property():
    """Test Source.data property."""
    src = gzip.text("hello")
    assert(type(src.data) == "bytes", "data property should be bytes")
    assert(str(src.data) == "hello", "data should match input")

def test_text_source_type():
    """Test Source type."""
    src = gzip.text("hello")
    assert(type(src) == "gzip.source", "type should be gzip.source")

def test_text_source_repr():
    """Test Source string representation."""
    src = gzip.text("hello")
    s = str(src)
    assert("gzip.source" in s, "repr should contain gzip.source")
    assert("5 bytes" in s, "repr should show byte count")

def test_text_source_truth():
    """Test Source truthiness."""
    if gzip.text("hello"):
        pass
    else:
        assert(False, "non-empty source should be truthy")

def test_text_empty():
    """Test gzip.text() with empty string."""
    compressed = gzip.text("").compress()
    decompressed = gzip.bytes(compressed).decompress()
    assert(str(decompressed) == "", "empty round-trip should work")

# ============================================================================
# gzip.bytes() - Source from bytes
# ============================================================================

def test_bytes_compress_decompress():
    """Test gzip.bytes() compress/decompress round-trip."""
    original = b"binary data \x00\x01\x02"
    compressed = gzip.bytes(original).compress()
    decompressed = gzip.bytes(compressed).decompress()
    assert(decompressed == original, "bytes round-trip should preserve data")

def test_bytes_accepts_string():
    """Test gzip.bytes() accepts string too."""
    compressed = gzip.bytes("hello").compress()
    decompressed = gzip.bytes(compressed).decompress()
    assert(str(decompressed) == "hello", "bytes() should accept string")

def test_bytes_large_data():
    """Test compression with larger data via object API."""
    large = "x" * 10000
    compressed = gzip.text(large).compress()
    assert(len(compressed) < len(large), "compressed should be smaller")
    decompressed = gzip.bytes(compressed).decompress()
    assert(str(decompressed) == large, "large data round-trip should work")

# ============================================================================
# gzip.file() - File-to-file operations
# ============================================================================

def test_file_compress():
    """Test gzip.file().compress() reads source and writes .gz file."""
    src = "/tmp/starkite_gzip_src.txt"
    gz = "/tmp/starkite_gzip_out.gz"
    write_text(src, "file content for gzip")
    gzip.file(gz).compress(src)
    # Verify by decompressing back
    gzip.file(gz).decompress("/tmp/starkite_gzip_restored.txt")
    content = read_text("/tmp/starkite_gzip_restored.txt")
    assert(content == "file content for gzip", "file compress/decompress round-trip should work")
    fs.path(src).remove()
    fs.path(gz).remove()
    fs.path("/tmp/starkite_gzip_restored.txt").remove()

def test_file_decompress_auto_name():
    """Test gzip.file().decompress() strips .gz for output."""
    src = "/tmp/starkite_gzip_auto.txt"
    gz = "/tmp/starkite_gzip_auto.txt.gz"
    write_text(src, "auto name test")
    gzip.file(gz).compress(src)
    fs.path(src).remove()
    # decompress without dest — should strip .gz
    gzip.file(gz).decompress()
    content = read_text("/tmp/starkite_gzip_auto.txt")
    assert(content == "auto name test", "auto-name decompress should work")
    fs.path(gz).remove()
    fs.path("/tmp/starkite_gzip_auto.txt").remove()

def test_file_compress_with_level():
    """Test gzip.file().compress() with level kwarg."""
    src = "/tmp/starkite_gzip_level.txt"
    gz = "/tmp/starkite_gzip_level.gz"
    write_text(src, "level test data " * 100)
    gzip.file(gz).compress(src, level=9)
    gzip.file(gz).decompress("/tmp/starkite_gzip_level_out.txt")
    content = read_text("/tmp/starkite_gzip_level_out.txt")
    assert(content == "level test data " * 100, "level compress should work")
    fs.path(src).remove()
    fs.path(gz).remove()
    fs.path("/tmp/starkite_gzip_level_out.txt").remove()

def test_file_type():
    """Test gzip.file type name."""
    f = gzip.file("/tmp/test.gz")
    assert(type(f) == "gzip.file", "type should be gzip.file")

def test_file_repr():
    """Test gzip.file string representation."""
    f = gzip.file("/tmp/test.gz")
    s = str(f)
    assert("gzip.file" in s, "repr should contain gzip.file")
    assert("/tmp/test.gz" in s, "repr should contain path")

# ============================================================================
# Source compress/decompress with dest parameter
# ============================================================================

def test_text_compress_to_file():
    """Test gzip.text().compress(dest) writes to file."""
    dest = "/tmp/starkite_gzip_text_dest.gz"
    gzip.text("hello from text").compress(dest=dest)
    gzip.file(dest).decompress("/tmp/starkite_gzip_text_dest_out.txt")
    content = read_text("/tmp/starkite_gzip_text_dest_out.txt")
    assert(content == "hello from text", "text compress to file should work")
    fs.path(dest).remove()
    fs.path("/tmp/starkite_gzip_text_dest_out.txt").remove()

def test_bytes_decompress_to_file():
    """Test gzip.bytes().decompress(dest) writes to file."""
    compressed = gzip.text("decompress to file").compress()
    dest = "/tmp/starkite_gzip_bytes_dest.txt"
    gzip.bytes(compressed).decompress(dest=dest)
    content = read_text(dest)
    assert(content == "decompress to file", "bytes decompress to file should work")
    fs.path(dest).remove()

# ============================================================================
# try_ variants on Source
# ============================================================================

def test_try_compress_success():
    """Test Source.try_compress() on valid data."""
    result = gzip.text("hello").try_compress()
    assert(result.ok, "try_compress should succeed")
    assert(len(result.value) > 0, "should have compressed data")

def test_try_decompress_success():
    """Test Source.try_decompress() on valid compressed data."""
    compressed = gzip.text("hello").compress()
    result = gzip.bytes(compressed).try_decompress()
    assert(result.ok, "try_decompress should succeed")
    assert(str(result.value) == "hello", "decompressed value should match")

def test_try_decompress_invalid():
    """Test Source.try_decompress() on invalid data."""
    result = gzip.text("not gzip data").try_decompress()
    assert(not result.ok, "try_decompress should fail on invalid data")
    assert(result.error != "", "should have error message")

# ============================================================================
# try_ variants on GzipFile
# ============================================================================

def test_try_file_compress_success():
    """Test gzip.file().try_compress() on valid source."""
    src = "/tmp/starkite_gzip_try_c.txt"
    gz = "/tmp/starkite_gzip_try_c.gz"
    write_text(src, "try compress")
    result = gzip.file(gz).try_compress(src)
    assert(result.ok, "try_compress should succeed")
    fs.path(src).remove()
    fs.path(gz).remove()

def test_try_file_decompress_failure():
    """Test gzip.file().try_decompress() on invalid data."""
    gz = "/tmp/starkite_gzip_try_bad.gz"
    write_text(gz, "not gzip data")
    result = gzip.file(gz).try_decompress("/tmp/starkite_gzip_try_bad_out.txt")
    assert(not result.ok, "try_decompress should fail on invalid data")
    fs.path(gz).remove()

# ============================================================================
# try_ variants on module (TryModule)
# ============================================================================

def test_try_file_factory():
    """Test gzip.try_file() returns Result."""
    result = gzip.try_file("/tmp/test.gz")
    assert(result.ok, "try_file should succeed (just stores path)")
    assert(type(result.value) == "gzip.file", "should return gzip.file")

def test_try_text_success():
    """Test gzip.try_text() always succeeds."""
    result = gzip.try_text("hello")
    assert(result.ok, "try_text should succeed")
    assert(type(result.value) == "gzip.source", "should return source")

def test_try_bytes_success():
    """Test gzip.try_bytes() always succeeds."""
    result = gzip.try_bytes(b"hello")
    assert(result.ok, "try_bytes should succeed")
    assert(type(result.value) == "gzip.source", "should return source")

# ============================================================================
# Chaining patterns
# ============================================================================

def test_chain_text_compress_bytes_decompress():
    """Test chaining: gzip.text(...).compress() -> gzip.bytes(...).decompress()."""
    original = "chained workflow"
    result = gzip.bytes(gzip.text(original).compress()).decompress()
    assert(str(result) == original, "chained compress/decompress should work")

def test_compress_level_invalid():
    """Test invalid compression level."""
    result = gzip.text("test").try_compress(level=99)
    assert(not result.ok, "invalid level should fail")
    assert("invalid compression level" in result.error, "should mention invalid level")
