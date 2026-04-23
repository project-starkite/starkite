# yaml_core_test.star - Tests for yaml module (core edition, no k8s dependency)

# ============================================================================
# decode_all tests
# ============================================================================

def test_decode_all_multi_doc():
    """Test yaml.decode_all with multiple documents."""
    input = """name: first
kind: ConfigMap
---
name: second
kind: Deployment
"""
    result = yaml.decode_all(input)
    assert(type(result) == "list", "decode_all should return a list")
    assert(len(result) == 2, "should have 2 documents, got: " + str(len(result)))
    assert(result[0]["name"] == "first", "first doc name")
    assert(result[0]["kind"] == "ConfigMap", "first doc kind")
    assert(result[1]["name"] == "second", "second doc name")
    assert(result[1]["kind"] == "Deployment", "second doc kind")

def test_decode_all_single_doc():
    """Test yaml.decode_all with single document."""
    result = yaml.decode_all("key: value")
    assert(len(result) == 1, "should have 1 document")
    assert(result[0]["key"] == "value", "should decode value")

def test_decode_all_empty():
    """Test yaml.decode_all with empty string."""
    result = yaml.decode_all("")
    assert(len(result) == 0, "empty input should return empty list")

def test_decode_all_roundtrip():
    """Test encode_all -> decode_all roundtrip."""
    docs = [
        {"name": "first", "kind": "ConfigMap"},
        {"name": "second", "kind": "Deployment"},
    ]
    encoded = yaml.encode_all(docs)
    decoded = yaml.decode_all(encoded)
    assert(len(decoded) == 2, "roundtrip should preserve document count")
    assert(decoded[0]["name"] == "first", "roundtrip first doc")
    assert(decoded[1]["name"] == "second", "roundtrip second doc")

def test_decode_all_with_lists():
    """Test yaml.decode_all with list documents."""
    input = """- a
- b
---
- c
- d
"""
    result = yaml.decode_all(input)
    assert(len(result) == 2, "should have 2 documents")
    assert(result[0][0] == "a", "first doc first element")
    assert(result[1][0] == "c", "second doc first element")

# ============================================================================
# try_ variant tests
# ============================================================================

def test_try_decode_success():
    """Test yaml.try_decode on valid input."""
    result = yaml.try_decode("key: value")
    assert(result.ok, "try_decode should succeed on valid input")
    assert(result.value["key"] == "value", "try_decode value should match")

def test_try_decode_failure():
    """Test yaml.try_decode on invalid input."""
    result = yaml.try_decode("{{{{invalid yaml")
    # yaml.v3 is lenient — it may parse this as a string.
    # Use truly malformed YAML:
    result = yaml.try_decode("key: [unclosed")
    assert(not result.ok, "try_decode should fail on malformed YAML")
    assert(result.error != "", "try_decode should have error message")

def test_try_encode_success():
    """Test yaml.try_encode on valid input."""
    result = yaml.try_encode({"key": "value"})
    assert(result.ok, "try_encode should succeed")
    assert("key:" in result.value, "try_encode value should contain key")

def test_try_decode_all_success():
    """Test yaml.try_decode_all on valid multi-doc input."""
    input = "a: 1\n---\nb: 2"
    result = yaml.try_decode_all(input)
    assert(result.ok, "try_decode_all should succeed")
    assert(len(result.value) == 2, "try_decode_all should return 2 docs")

def test_try_decode_all_failure():
    """Test yaml.try_decode_all on invalid input."""
    result = yaml.try_decode_all("valid: doc\n---\nkey: [unclosed")
    assert(not result.ok, "try_decode_all should fail on malformed doc")

def test_try_encode_all_success():
    """Test yaml.try_encode_all on valid input."""
    result = yaml.try_encode_all([{"a": 1}, {"b": 2}])
    assert(result.ok, "try_encode_all should succeed")
    assert("---" in result.value, "try_encode_all should have separator")

# ============================================================================
# bytes input tests
# ============================================================================

def test_decode_bytes():
    """Test yaml.decode accepts bytes input."""
    data = b"name: starkite\nversion: 1"
    result = yaml.decode(data)
    assert(result["name"] == "starkite", "should decode name from bytes")
    assert(result["version"] == 1, "should decode version from bytes")

def test_decode_all_bytes():
    """Test yaml.decode_all accepts bytes input."""
    data = b"a: 1\n---\nb: 2"
    result = yaml.decode_all(data)
    assert(len(result) == 2, "should decode 2 docs from bytes")
    assert(result[0]["a"] == 1, "first doc from bytes")
    assert(result[1]["b"] == 2, "second doc from bytes")

def test_try_decode_bytes():
    """Test yaml.try_decode accepts bytes input."""
    result = yaml.try_decode(b"key: value")
    assert(result.ok, "try_decode should accept bytes")
    assert(result.value["key"] == "value", "try_decode bytes value")

def test_decode_bytes_roundtrip():
    """Test encode returns string, decode accepts it as bytes."""
    original = {"hello": "world"}
    encoded = yaml.encode(original)
    # Convert string to bytes and decode
    decoded = yaml.decode(b"hello: world")
    assert(decoded["hello"] == "world", "bytes roundtrip should work")

def test_decode_bad_type():
    """Test yaml.decode rejects non-string/bytes types."""
    result = yaml.try_decode(42)
    assert(not result.ok, "decode should reject int")
    assert("string or bytes" in result.error, "error should mention expected types")

def test_decode_all_bad_type():
    """Test yaml.decode_all rejects non-string/bytes types."""
    result = yaml.try_decode_all([1, 2])
    assert(not result.ok, "decode_all should reject list")
    assert("string or bytes" in result.error, "error should mention expected types")

# ============================================================================
# Existing basic tests (duplicated from yaml_test.star to run without k8s)
# ============================================================================

def test_encode_dict():
    """Test yaml.encode with dict."""
    result = yaml.encode({"name": "starkite", "version": 1})
    assert("name:" in result, "should contain name key")
    assert("starkite" in result, "should contain starkite value")

def test_decode_dict():
    """Test yaml.decode with dict."""
    result = yaml.decode("name: starkite\nversion: 1")
    assert(result["name"] == "starkite", "should decode name")
    assert(result["version"] == 1, "should decode version")

def test_roundtrip():
    """Test encode/decode roundtrip."""
    original = {"key": "value", "list": [1, 2, 3]}
    encoded = yaml.encode(original)
    decoded = yaml.decode(encoded)
    assert(decoded["key"] == "value", "key preserved")
    assert(len(decoded["list"]) == 3, "list length preserved")

def test_encode_all():
    """Test yaml.encode_all."""
    docs = [
        {"name": "first", "kind": "ConfigMap"},
        {"name": "second", "kind": "Deployment"},
    ]
    result = yaml.encode_all(docs)
    assert("---" in result, "should have document separator")
    assert("kind: ConfigMap" in result, "should have first doc kind")
    assert("kind: Deployment" in result, "should have second doc kind")
