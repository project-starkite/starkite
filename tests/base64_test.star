# base64_test.star - Tests for base64 module (builder/source pattern)

# ===========================================================================
# text() factory + encode/decode
# ===========================================================================

def test_text_encode():
    """Test base64.text().encode()."""
    result = base64.text("hello").encode()
    assert(result == "aGVsbG8=", "known base64 of 'hello', got '%s'" % result)

def test_text_encode_empty():
    """Test base64.text("").encode()."""
    result = base64.text("").encode()
    assert(result == "", "empty string encodes to empty")

def test_text_decode():
    """Test base64.text().decode()."""
    result = base64.text("aGVsbG8=").decode()
    assert(result == b"hello", "should decode to b'hello', got '%s'" % result)

def test_text_decode_empty():
    """Test base64.text("").decode()."""
    result = base64.text("").decode()
    assert(result == b"", "empty string decodes to empty bytes")

def test_text_roundtrip():
    """Test encode/decode roundtrip."""
    original = "Hello, World! 123 !@#$%"
    encoded = base64.text(original).encode()
    decoded = base64.text(encoded).decode()
    assert(decoded == bytes(original), "roundtrip should preserve content")

def test_text_roundtrip_special_chars():
    """Test roundtrip with special characters."""
    original = "hello\nworld\ttab"
    encoded = base64.text(original).encode()
    decoded = base64.text(encoded).decode()
    assert(decoded == bytes(original), "should handle special characters")

def test_text_encode_long_string():
    """Test encoding longer string."""
    original = "a" * 100
    encoded = base64.text(original).encode()
    decoded = base64.text(encoded).decode()
    assert(decoded == bytes(original), "should handle long strings")

def test_text_known_values():
    """Test known base64 values."""
    assert(base64.text("Man").encode() == "TWFu", "known encoding")
    assert(base64.text("Ma").encode() == "TWE=", "known encoding with padding")
    assert(base64.text("M").encode() == "TQ==", "known encoding with double padding")

def test_text_encode_url():
    """Test base64.text().encode_url()."""
    result = base64.text("hello").encode_url()
    assert(result == "aGVsbG8=", "url encode of 'hello', got '%s'" % result)

def test_text_decode_url():
    """Test base64.text().decode_url()."""
    result = base64.text("aGVsbG8=").decode_url()
    assert(result == b"hello", "url decode should return bytes")

def test_text_data_property():
    """Test base64.text().data property."""
    src = base64.text("hello")
    assert(src.data == b"hello", "data should be bytes of input")

# ===========================================================================
# bytes() factory
# ===========================================================================

def test_bytes_encode():
    """Test base64.bytes(b"...").encode()."""
    result = base64.bytes(b"hello").encode()
    assert(result == "aGVsbG8=", "should encode bytes, got '%s'" % result)

def test_bytes_encode_url():
    """Test base64.bytes(b"...").encode_url()."""
    result = base64.bytes(b"hello").encode_url()
    assert(result == "aGVsbG8=", "should encode bytes with url encoding, got '%s'" % result)

def test_bytes_with_string():
    """Test base64.bytes() accepts string too."""
    result = base64.bytes("hello").encode()
    assert(result == "aGVsbG8=", "should accept string input")

def test_bytes_roundtrip():
    """Test encode bytes then decode roundtrip."""
    encoded = base64.bytes(b"binary\x00data").encode()
    decoded = base64.text(encoded).decode()
    assert(decoded == b"binary\x00data", "roundtrip with bytes should preserve content")

# ===========================================================================
# try_ variants (object-level)
# ===========================================================================

def test_try_decode_success():
    """base64.text().try_decode() with valid input -> ok=True."""
    result = base64.text("aGVsbG8=").try_decode()
    assert(result.ok, "try_decode valid input should succeed")
    assert(result.value == b"hello", "decoded value should be b'hello', got '%s'" % result.value)

def test_try_decode_failure():
    """base64.text().try_decode() with invalid input -> ok=False."""
    result = base64.text("!!!not-valid-base64!!!").try_decode()
    assert(not result.ok, "try_decode invalid input should fail")
    assert(result.error != "", "should have non-empty error")

def test_try_decode_url_success():
    """base64.text().try_decode_url() with valid input -> ok=True."""
    encoded = base64.text("test").encode_url()
    result = base64.text(encoded).try_decode_url()
    assert(result.ok, "try_decode_url valid input should succeed")
    assert(result.value == b"test", "decoded value should be b'test'")

def test_try_decode_url_failure():
    """base64.text().try_decode_url() with invalid input -> ok=False."""
    result = base64.text("!!!invalid!!!").try_decode_url()
    assert(not result.ok, "try_decode_url invalid input should fail")

def test_try_encode_success():
    """base64.text().try_encode() always succeeds."""
    result = base64.text("hello").try_encode()
    assert(result.ok, "try_encode should always succeed")
    assert(result.value == "aGVsbG8=", "should encode correctly")

# ===========================================================================
# try_ variants (module-level wraps factories)
# ===========================================================================

def test_try_text():
    """base64.try_text() wraps text factory."""
    result = base64.try_text("hello")
    assert(result.ok, "try_text should succeed")
    # value is a base64.source object
    assert(result.value.encode() == "aGVsbG8=", "source should encode correctly")

def test_try_bytes():
    """base64.try_bytes() wraps bytes factory."""
    result = base64.try_bytes(b"hello")
    assert(result.ok, "try_bytes should succeed")
    assert(result.value.encode() == "aGVsbG8=", "source should encode correctly")

def test_try_file():
    """base64.try_file() wraps file factory."""
    result = base64.try_file("/tmp/test.txt")
    assert(result.ok, "try_file should succeed (creates file object)")
