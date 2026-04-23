# fs_path_test.star - Tests for fs.path() pathlib-style API

# ============================================================================
# Properties tests
# ============================================================================

def test_path_properties():
    """Test path properties: name, stem, suffix, string."""
    p = fs.path("/var/log/app.log")
    assert(p.name == "app.log", "name should be 'app.log', got: " + p.name)
    assert(p.stem == "app", "stem should be 'app', got: " + p.stem)
    assert(p.suffix == ".log", "suffix should be '.log', got: " + p.suffix)
    assert(p.string == "/var/log/app.log", "string should be path")

def test_path_parent():
    """Test parent property."""
    p = fs.path("/var/log/syslog")
    assert(p.parent.string == "/var/log", "parent should be /var/log")
    assert(p.parent.parent.string == "/var", "grandparent should be /var")

def test_path_parts():
    """Test parts property."""
    p = fs.path("/var/log/syslog")
    parts = p.parts
    assert(parts[0] == "/", "first part should be /")
    assert(parts[1] == "var", "second part should be var")
    assert(parts[2] == "log", "third part should be log")
    assert(parts[3] == "syslog", "fourth part should be syslog")

def test_path_no_extension():
    """Test stem/suffix with no extension."""
    p = fs.path("/etc/hostname")
    assert(p.stem == "hostname", "stem should be 'hostname'")
    assert(p.suffix == "", "suffix should be empty")

def test_path_dotfile():
    """Test stem/suffix with dotfile."""
    p = fs.path("/home/user/.bashrc")
    assert(p.name == ".bashrc", "name should be '.bashrc'")

# ============================================================================
# Path manipulation tests
# ============================================================================

def test_path_join():
    """Test join method."""
    p = fs.path("/etc")
    result = p.join("nginx", "nginx.conf")
    assert(result.string == "/etc/nginx/nginx.conf", "join should combine paths")

def test_path_with_name():
    """Test with_name method."""
    p = fs.path("/var/log/syslog")
    result = p.with_name("messages")
    assert(result.string == "/var/log/messages", "with_name should replace filename")

def test_path_with_suffix():
    """Test with_suffix method."""
    p = fs.path("/var/log/app.log")
    result = p.with_suffix(".txt")
    assert(result.string == "/var/log/app.txt", "with_suffix should replace extension")

def test_path_resolve():
    """Test resolve method."""
    p = fs.path(".")
    result = p.resolve()
    assert(result.string[0] == "/", "resolve should return absolute path")

def test_path_is_absolute():
    """Test is_absolute method."""
    assert(fs.path("/etc").is_absolute(), "/etc should be absolute")
    assert(not fs.path("etc").is_absolute(), "etc should not be absolute")

def test_path_is_relative_to():
    """Test is_relative_to method."""
    p = fs.path("/var/log/syslog")
    assert(p.is_relative_to("/var"), "should be relative to /var")
    assert(not p.is_relative_to("/etc"), "should not be relative to /etc")

def test_path_match():
    """Test match method."""
    p = fs.path("/var/log/app.log")
    assert(p.match("*.log"), "should match *.log")
    assert(not p.match("*.txt"), "should not match *.txt")

def test_path_expanduser():
    """Test expanduser method."""
    p = fs.path("~/Documents")
    result = p.expanduser()
    assert(result.string[0] == "/", "expanduser should return absolute path")
    assert("Documents" in result.string, "expanduser should preserve path after ~")

# ============================================================================
# Slash operator tests
# ============================================================================

def test_path_slash_operator():
    """Test / operator for path joining."""
    p = fs.path("/etc") / "nginx" / "nginx.conf"
    assert(p.string == "/etc/nginx/nginx.conf", "/ operator should join paths")

def test_path_slash_with_path():
    """Test / operator between two Path objects."""
    base = fs.path("/etc")
    sub = fs.path("nginx")
    result = base / sub
    assert(result.string == "/etc/nginx", "/ should work between Path objects")

# ============================================================================
# File check tests
# ============================================================================

def test_path_exists():
    """Test exists method."""
    assert(fs.path("/etc/passwd").exists(), "/etc/passwd should exist")
    assert(not fs.path("/nonexistent/12345").exists(), "missing path should not exist")

def test_path_is_file():
    """Test is_file method."""
    assert(fs.path("/etc/passwd").is_file(), "/etc/passwd should be a file")
    assert(not fs.path("/tmp").is_file(), "/tmp should not be a file")

def test_path_is_dir():
    """Test is_dir method."""
    assert(fs.path("/tmp").is_dir(), "/tmp should be a directory")
    assert(not fs.path("/etc/passwd").is_dir(), "/etc/passwd should not be a directory")

def test_path_is_symlink():
    """Test is_symlink method."""
    result = fs.path("/etc/passwd").is_symlink()
    assert(type(result) == "bool", "is_symlink should return bool")

def test_path_stat():
    """Test stat method."""
    stat = fs.path("/etc/passwd").stat()
    assert("size" in stat, "stat should have size")
    assert("mode" in stat, "stat should have mode")
    assert("is_dir" in stat, "stat should have is_dir")

def test_path_owner_group():
    """Test owner and group methods."""
    p = fs.path("/etc/passwd")
    owner = p.owner()
    assert(type(owner) == "string", "owner should return string")
    assert(owner != "", "owner should not be empty")

    group = p.group()
    assert(type(group) == "string", "group should return string")
    assert(group != "", "group should not be empty")

# ============================================================================
# Read/Write tests
# ============================================================================

def test_path_read_write_roundtrip():
    """Test write_text and read_text roundtrip."""
    p = fs.path("/tmp/starkite_path_rw_test.txt")
    p.write_text("hello from path")
    content = p.read_text()
    assert(content == "hello from path", "read_text should return written content")
    p.remove()

def test_path_read_write_bytes():
    """Test write_bytes and read_bytes."""
    p = fs.path("/tmp/starkite_path_bytes_test.bin")
    data = b"\x00\x01\x02\xff"
    p.write_bytes(data)
    result = p.read_bytes()
    assert(result == data, "read_bytes should return same data")
    p.remove()

def test_path_append():
    """Test append_text method."""
    p = fs.path("/tmp/starkite_path_append_test.txt")
    p.write_text("hello")
    p.append_text(" world")
    content = p.read_text()
    assert(content == "hello world", "append should add to file")
    p.remove()

# ============================================================================
# File & directory operation tests
# ============================================================================

def test_path_touch():
    """Test touch method."""
    p = fs.path("/tmp/starkite_path_touch_test.txt")
    if p.exists():
        p.remove()
    p.touch()
    assert(p.exists(), "touch should create file")
    assert(p.is_file(), "touched path should be a file")
    p.remove()

def test_path_mkdir_listdir():
    """Test mkdir and listdir methods."""
    d = fs.path("/tmp/starkite_path_mkdir_test")
    if d.exists():
        exec("rm -rf /tmp/starkite_path_mkdir_test")

    d.mkdir()
    assert(d.is_dir(), "mkdir should create directory")

    # Create some files
    (d / "a.txt").write_text("a")
    (d / "b.txt").write_text("b")

    entries = d.listdir()
    assert(len(entries) == 2, "listdir should have 2 entries")
    # Entries should be Path objects
    assert(type(entries[0]) == "fs.path", "listdir entries should be Path objects")

    exec("rm -rf /tmp/starkite_path_mkdir_test")

def test_path_glob():
    """Test glob method."""
    d = fs.path("/tmp/starkite_path_glob_test")
    if d.exists():
        exec("rm -rf /tmp/starkite_path_glob_test")

    d.mkdir()
    (d / "a.txt").write_text("a")
    (d / "b.log").write_text("b")
    (d / "c.txt").write_text("c")

    txt_files = d.glob("*.txt")
    assert(len(txt_files) == 2, "glob('*.txt') should find 2 files")
    assert(type(txt_files[0]) == "fs.path", "glob results should be Path objects")

    exec("rm -rf /tmp/starkite_path_glob_test")

def test_path_rename():
    """Test rename method."""
    src = fs.path("/tmp/starkite_path_rename_src.txt")
    dst_path = "/tmp/starkite_path_rename_dst.txt"
    src.write_text("rename test")

    result = src.rename(dst_path)
    assert(result.string == dst_path, "rename should return new path")
    assert(result.exists(), "renamed file should exist")
    assert(not src.exists(), "source should not exist after rename")

    result.remove()

def test_path_chmod():
    """Test chmod method."""
    p = fs.path("/tmp/starkite_path_chmod_test.txt")
    p.write_text("test")
    p.chmod(0o755)
    stat = p.stat()
    assert("x" in stat["mode"], "chmod 755 should set execute bit")
    p.remove()

def test_path_symlink_to():
    """Test symlink_to method."""
    target = fs.path("/tmp/starkite_path_symlink_target.txt")
    link = fs.path("/tmp/starkite_path_symlink_link.txt")
    target.write_text("target content")

    if link.exists():
        link.remove()

    link.symlink_to(target.string)
    assert(link.is_symlink(), "should be a symlink")
    assert(link.read_text() == "target content", "symlink should read target content")

    link.remove()
    target.remove()

def test_path_remove():
    """Test remove method."""
    p = fs.path("/tmp/starkite_path_remove_test.txt")
    p.write_text("to be removed")
    assert(p.exists(), "file should exist before remove")

    p.remove()
    assert(not p.exists(), "file should not exist after remove")

# ============================================================================
# try_ dispatch tests
# ============================================================================

def test_try_read_text_success():
    """Test try_read_text on existing file."""
    p = fs.path("/tmp/starkite_path_try_success.txt")
    p.write_text("hello")

    result = p.try_read_text()
    assert(result.ok, "try_read_text should succeed")
    assert(result.value == "hello", "try_read_text value should match")
    assert(result.error == "", "try_read_text error should be empty")

    p.remove()

def test_try_read_text_failure():
    """Test try_read_text on missing file."""
    p = fs.path("/nonexistent/path/12345.txt")
    result = p.try_read_text()
    assert(not result.ok, "try_read_text on missing file should fail")
    assert(result.value == None, "try_read_text value should be None on failure")
    assert("no such file" in result.error, "try_read_text error should mention 'no such file'")

def test_try_write_text_failure():
    """Test try_write_text to unwritable path."""
    p = fs.path("/nonexistent/dir/file.txt")
    result = p.try_write_text("data")
    assert(not result.ok, "try_write_text should fail for bad path")
    assert(result.error != "", "try_write_text should have error message")

def test_try_result_truthiness():
    """Test that Result is truthy on success and falsy on failure."""
    p = fs.path("/tmp/starkite_path_try_truth.txt")
    p.write_text("data")

    result = p.try_read_text()
    if result:
        pass  # good
    else:
        assert(False, "successful result should be truthy")

    result2 = fs.path("/nonexistent/12345.txt").try_read_text()
    if result2:
        assert(False, "failed result should be falsy")

    p.remove()

# ============================================================================
# Chaining tests
# ============================================================================

def test_path_chaining():
    """Test method chaining."""
    p = fs.path("/etc")
    result = p.join("nginx", "sites-available", "default")
    assert(result.string == "/etc/nginx/sites-available/default",
           "chaining should work: " + result.string)

def test_path_repr():
    """Test string representation."""
    p = fs.path("/etc/hostname")
    s = str(p)
    assert("path(" in s, "repr should contain 'path('")
    assert("/etc/hostname" in s, "repr should contain the path")

def test_path_truth():
    """Test truth value."""
    p = fs.path("/etc")
    if p:
        pass  # good, truthy
    else:
        assert(False, "non-empty path should be truthy")

def test_path_type():
    """Test type()."""
    p = fs.path("/etc")
    assert(type(p) == "fs.path", "type should be 'fs.path'")

# ============================================================================
# Global alias tests
# ============================================================================

def test_path_global_alias():
    """Test that path() is available as global alias."""
    p = path("/etc/hostname")
    assert(p.name == "hostname", "global path() alias should work")

# ============================================================================
# New Path method tests
# ============================================================================

def test_path_clean():
    """Test clean method resolves . and .. components."""
    p = fs.path("/var/log/../log/./syslog")
    result = p.clean()
    assert(result.string == "/var/log/syslog", "clean should resolve .. and ., got: " + result.string)

def test_path_copy_to():
    """Test copy_to method copies file to target."""
    src = fs.path("/tmp/starkite_test_copy_src.txt")
    dst = fs.path("/tmp/starkite_test_copy_dst.txt")
    src.write_text("copy this content")

    result = src.copy_to(dst.string)
    assert(dst.exists(), "destination should exist after copy_to")
    assert(dst.read_text() == "copy this content", "destination should have same content")
    assert(src.exists(), "source should still exist after copy_to")

    src.remove()
    dst.remove()

def test_path_move_to():
    """Test move_to method moves file to target."""
    src = fs.path("/tmp/starkite_test_move_src.txt")
    dst = fs.path("/tmp/starkite_test_move_dst.txt")
    src.write_text("move this content")

    result = src.move_to(dst.string)
    assert(dst.exists(), "destination should exist after move_to")
    assert(dst.read_text() == "move this content", "destination should have same content")
    assert(not src.exists(), "source should not exist after move_to")

    dst.remove()

def test_path_truncate():
    """Test truncate method truncates file to given size."""
    p = fs.path("/tmp/starkite_test_truncate.txt")
    p.write_text("hello world")
    p.truncate(5)
    content = p.read_text()
    assert(content == "hello", "truncate(5) should keep first 5 bytes, got: " + content)

    p.remove()

def test_path_readlink():
    """Test readlink method returns symlink target."""
    target = fs.path("/tmp/starkite_test_readlink_target.txt")
    link = fs.path("/tmp/starkite_test_readlink_link.txt")
    target.write_text("readlink target")

    if link.exists():
        link.remove()

    link.symlink_to(target.string)
    result = link.readlink()
    assert(result.string == target.string, "readlink should return target path, got: " + result.string)

    link.remove()
    target.remove()

def test_path_hardlink_to():
    """Test hardlink_to method creates a hard link."""
    target = fs.path("/tmp/starkite_test_hardlink_target.txt")
    link = fs.path("/tmp/starkite_test_hardlink_link.txt")
    target.write_text("hardlink content")

    if link.exists():
        link.remove()

    link.hardlink_to(target.string)
    assert(link.exists(), "hard link should exist")
    assert(link.read_text() == "hardlink content", "hard link should have same content")

    link.remove()
    target.remove()

def test_path_walk():
    """Test walk method for recursive directory traversal."""
    base = fs.path("/tmp/starkite_test_walk")
    if base.exists():
        exec("rm -rf /tmp/starkite_test_walk")

    base.mkdir()
    (base / "file1.txt").write_text("f1")
    (base / "sub").mkdir()
    (base / "sub" / "file2.txt").write_text("f2")

    entries = base.walk()
    assert(len(entries) > 0, "walk should return entries")

    # Each entry should be a tuple of (Path, dirs, files)
    for entry in entries:
        assert(len(entry) == 3, "walk entry should have 3 elements (path, dirs, files)")

    exec("rm -rf /tmp/starkite_test_walk")

def test_path_disk_usage():
    """Test disk_usage method returns disk space info."""
    p = fs.path("/")
    usage = p.disk_usage()
    assert("total" in usage, "disk_usage should have 'total' key")
    assert("used" in usage, "disk_usage should have 'used' key")
    assert("free" in usage, "disk_usage should have 'free' key")
    assert(usage["total"] > 0, "total should be positive")
    assert(usage["free"] > 0, "free should be positive")

def test_path_chown():
    """Test chown method (requires root, skip)."""
    skip("requires root")
