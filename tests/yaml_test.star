# yaml_test.star - Tests for yaml module (builder/file pattern)

def test_file_decode_dict():
    """Test yaml.file(path).decode() with dict."""
    path = "/tmp/starkite_yaml_test_decode_dict.yaml"
    write_text(path, "name: crsh\nversion: 1\n")
    result = yaml.file(path).decode()
    assert(result["name"] == "crsh", "should decode name")
    assert(result["version"] == 1, "should decode version")
    fs.path(path).remove()

def test_file_decode_list():
    """Test yaml.file(path).decode() with list."""
    path = "/tmp/starkite_yaml_test_decode_list.yaml"
    write_text(path, "- a\n- b\n- c\n")
    result = yaml.file(path).decode()
    assert(len(result) == 3, "should have 3 elements")
    assert(result[0] == "a", "first element")
    fs.path(path).remove()

def test_file_decode_nested():
    """Test yaml.file(path).decode() with nested structure."""
    path = "/tmp/starkite_yaml_test_decode_nested.yaml"
    write_text(path, "metadata:\n  name: test\n  labels:\n    app: crsh\n")
    result = yaml.file(path).decode()
    assert(result["metadata"]["name"] == "test", "should decode nested name")
    assert(result["metadata"]["labels"]["app"] == "crsh", "should decode nested label")
    fs.path(path).remove()

def test_file_decode_all():
    """Test yaml.file(path).decode_all() with multi-doc."""
    path = "/tmp/starkite_yaml_test_decode_all.yaml"
    write_text(path, "name: first\n---\nname: second\n")
    result = yaml.file(path).decode_all()
    assert(len(result) == 2, "should have 2 documents")
    assert(result[0]["name"] == "first", "first doc name")
    assert(result[1]["name"] == "second", "second doc name")
    fs.path(path).remove()

def test_file_path_property():
    """Test yaml.file(path).path returns the path."""
    f = yaml.file("/some/config.yaml")
    assert(f.path == "/some/config.yaml", "path property should return the path")

def test_from_write_file_single_doc():
    """Test yaml.source(data).write_file(path) with single dict."""
    path = "/tmp/starkite_yaml_test_write_single.yaml"
    yaml.source({"key": "value"}).write_file(path)
    content = read_text(path)
    assert("key: value" in content, "should contain key: value")
    fs.path(path).remove()

def test_from_write_file_multi_doc():
    """Test yaml.source(list).write_file(path) auto-detects multi-doc."""
    path = "/tmp/starkite_yaml_test_write_multi.yaml"
    yaml.source([{"name": "first"}, {"name": "second"}]).write_file(path)
    content = read_text(path)
    assert("---" in content, "should have document separator")
    assert("name: first" in content, "should have first doc")
    assert("name: second" in content, "should have second doc")
    fs.path(path).remove()

def test_from_data_property():
    """Test yaml.source(data).data returns the data."""
    data = {"key": "value"}
    w = yaml.source(data)
    assert(w.data == data, "data property should return the original data")

def test_roundtrip_single():
    """Test yaml round-trip: write then decode."""
    path = "/tmp/starkite_yaml_test_roundtrip.yaml"
    original = {"key": "value", "list": [1, 2, 3]}
    yaml.source(original).write_file(path)
    decoded = yaml.file(path).decode()
    assert(decoded["key"] == "value", "key preserved")
    assert(len(decoded["list"]) == 3, "list length preserved")
    fs.path(path).remove()

def test_roundtrip_multi():
    """Test yaml multi-doc round-trip: write then decode_all."""
    path = "/tmp/starkite_yaml_test_roundtrip_multi.yaml"
    docs = [{"name": "first", "kind": "ConfigMap"}, {"name": "second", "kind": "Deployment"}]
    yaml.source(docs).write_file(path)
    result = yaml.file(path).decode_all()
    assert(len(result) == 2, "should have 2 docs")
    assert(result[0]["kind"] == "ConfigMap", "first doc kind")
    assert(result[1]["kind"] == "Deployment", "second doc kind")
    fs.path(path).remove()

# ===========================================================================
# try_ variants
# ===========================================================================

def test_try_file_success():
    """yaml.try_file with valid path -> ok=True."""
    result = yaml.try_file("/some/path.yaml")
    assert(result.ok, "try_file should succeed")
    assert(result.value.path == "/some/path.yaml", "should have correct path")

def test_try_source_success():
    """yaml.try_sourcewith valid data -> ok=True."""
    result = yaml.try_source({"key": "value"})
    assert(result.ok, "try_sourceshould succeed")

def test_try_decode_success():
    """yaml.file(path).try_decode() with valid file -> ok=True."""
    path = "/tmp/starkite_yaml_test_try_decode.yaml"
    write_text(path, "key: value\n")
    result = yaml.file(path).try_decode()
    assert(result.ok, "try_decode should succeed")
    assert(result.value["key"] == "value", "decoded key should be 'value'")
    fs.path(path).remove()

def test_try_decode_missing_file():
    """yaml.file(path).try_decode() on missing file -> ok=False."""
    result = yaml.file("/tmp/nonexistent_yaml_file.yaml").try_decode()
    assert(not result.ok, "try_decode on missing file should fail")
    assert(result.error != "", "should have non-empty error")

def test_try_decode_all_success():
    """yaml.file(path).try_decode_all() with valid file -> ok=True."""
    path = "/tmp/starkite_yaml_test_try_decode_all.yaml"
    write_text(path, "a: 1\n---\nb: 2\n")
    result = yaml.file(path).try_decode_all()
    assert(result.ok, "try_decode_all should succeed")
    assert(len(result.value) == 2, "should have 2 docs")
    fs.path(path).remove()

def test_try_write_file_success():
    """yaml.source(data).try_write_file(path) -> ok=True."""
    path = "/tmp/starkite_yaml_test_try_write.yaml"
    result = yaml.source({"key": "value"}).try_write_file(path)
    assert(result.ok, "try_write_file should succeed")
    fs.path(path).remove()

# Integration tests for cloud edition (k8s module) are in yaml_cloud_test.star.
# Run with: starctl-cloud test tests/yaml_cloud_test.star
