# hash_test.star - Tests for hash module (builder/source pattern)

# ============================================================================
# text factory tests
# ============================================================================

def test_text_md5():
    result = hash.text("hello").md5()
    assert(len(result) == 32, "MD5 should be 32 hex chars")
    assert(result == "5d41402abc4b2a76b9719d911017c592", "known MD5 of 'hello'")

def test_text_md5_empty():
    result = hash.text("").md5()
    assert(len(result) == 32, "MD5 should be 32 hex chars")
    assert(result == "d41d8cd98f00b204e9800998ecf8427e", "known MD5 of empty string")

def test_text_sha1():
    result = hash.text("hello").sha1()
    assert(len(result) == 40, "SHA1 should be 40 hex chars")
    assert(result == "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", "known SHA1 of 'hello'")

def test_text_sha256():
    result = hash.text("hello").sha256()
    assert(len(result) == 64, "SHA256 should be 64 hex chars")
    assert(result == "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "known SHA256 of 'hello'")

def test_text_sha256_empty():
    result = hash.text("").sha256()
    assert(len(result) == 64, "SHA256 should be 64 hex chars")

def test_text_sha512():
    result = hash.text("hello").sha512()
    assert(len(result) == 128, "SHA512 should be 128 hex chars")

# ============================================================================
# bytes factory tests
# ============================================================================

def test_bytes_string():
    result = hash.bytes("hello").md5()
    assert(result == "5d41402abc4b2a76b9719d911017c592", "bytes with string should work like text")

def test_bytes_md5():
    result = hash.bytes(b"\x00\x01\x02").md5()
    assert(len(result) == 32, "should return 32 hex chars")

# ============================================================================
# file factory tests
# ============================================================================

def test_file_sha256():
    path = "/tmp/crsh_hash_test.txt"
    write_text(path, "hello")
    result = hash.file(path).sha256()
    assert(result == "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "file hash should match")
    fs.path(path).remove()

def test_file_md5():
    path = "/tmp/crsh_hash_test_md5.txt"
    write_text(path, "hello")
    result = hash.file(path).md5()
    assert(result == "5d41402abc4b2a76b9719d911017c592", "file MD5 should match")
    fs.path(path).remove()

# ============================================================================
# type/repr tests
# ============================================================================

def test_source_type():
    assert(type(hash.text("hello")) == "hash.source", "text should produce hash.source")
    assert(type(hash.bytes("hello")) == "hash.source", "bytes should produce hash.source")

def test_file_type():
    assert(type(hash.file("/tmp/test")) == "hash.file", "file should produce hash.file")

def test_source_data_property():
    src = hash.text("hello")
    assert(src.data == b"hello", "data property should return bytes")

def test_file_path_property():
    f = hash.file("/tmp/test")
    assert(f.path == "/tmp/test", "path property should return path string")

# ============================================================================
# consistency tests
# ============================================================================

def test_different_inputs():
    h1 = hash.text("hello").sha256()
    h2 = hash.text("world").sha256()
    assert(h1 != h2, "different inputs should produce different hashes")

def test_same_input():
    h1 = hash.text("test").sha256()
    h2 = hash.text("test").sha256()
    assert(h1 == h2, "same input should produce same hash")
