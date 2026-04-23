# fs_test.star - Tests for fs module (Path-first API)

# ============================================================================
# Read/Write tests
# ============================================================================

def test_read_text():
    """Test Path.read_text."""
    content = path("/etc/hosts").read_text()
    assert(content != "", "should read file content")

def test_write_and_read_text():
    """Test Path.write_text and Path.read_text."""
    p = path("/tmp/starkite_test_file.txt")
    test_content = "hello from kite test"

    p.write_text(test_content)
    content = p.read_text()
    assert(content == test_content, "should read back written content")

    p.remove()

def test_read_write_bytes():
    """Test Path.read_bytes and Path.write_bytes."""
    p = path("/tmp/starkite_bytes_test.bin")
    data = b"\x00\x01\x02\x03\xff\xfe\xfd"
    p.write_bytes(data)

    read_data = p.read_bytes()
    assert(read_data == data, "read_bytes should return same data")

    p.remove()

def test_global_read_write_aliases():
    """Test global aliases for read_text and write_text."""
    test_path = "/tmp/starkite_global_alias_test.txt"
    write_text(test_path, "global alias")
    content = read_text(test_path)
    assert(content == "global alias", "global aliases should work")

    path(test_path).remove()

# ============================================================================
# File check tests
# ============================================================================

def test_exists_file():
    """Test Path.exists with existing file."""
    assert(path("/etc/passwd").exists(), "/etc/passwd should exist")

def test_exists_missing():
    """Test Path.exists with missing file."""
    assert(not path("/nonexistent/path/12345").exists(), "missing path should not exist")

def test_exists_global_alias():
    """Test exists global alias."""
    assert(exists("/etc/passwd"), "global exists should work")
    assert(not exists("/nonexistent/path/12345"), "global exists should return false for missing")

def test_is_file():
    """Test Path.is_file."""
    assert(path("/etc/passwd").is_file(), "/etc/passwd should be a file")
    assert(not path("/tmp").is_file(), "/tmp should not be a file")

def test_is_dir():
    """Test Path.is_dir."""
    assert(path("/tmp").is_dir(), "/tmp should be a directory")
    assert(not path("/etc/passwd").is_dir(), "/etc/passwd should not be a directory")

def test_is_symlink():
    """Test Path.is_symlink."""
    result = path("/bin/sh").is_symlink()
    assert(type(result) == "bool", "is_symlink should return bool")

def test_is_symlink_regular_file():
    """Test Path.is_symlink with regular file."""
    assert(not path("/etc/passwd").is_symlink(), "regular file should not be symlink")

def test_stat():
    """Test Path.stat."""
    info = path("/etc/passwd").stat()
    assert(info != None, "should return stat for existing file")
    assert("size" in info, "stat should have size")
    assert("mode" in info, "stat should have mode")
    assert("is_dir" in info, "stat should have is_dir")

# ============================================================================
# Directory tests
# ============================================================================

def test_mkdir_and_remove():
    """Test Path.mkdir and Path.remove."""
    p = path("/tmp/starkite_test_dir")

    p.mkdir()
    assert(p.is_dir(), "directory should exist")

    p.remove()
    assert(not p.exists(), "directory should be removed")

def test_mkdir_parents():
    """Test Path.mkdir with parents option."""
    p = path("/tmp/starkite_test/nested/dir")

    p.mkdir(parents=True)
    assert(p.is_dir(), "nested directory should exist")

    exec("rm -rf /tmp/starkite_test")

def test_listdir():
    """Test Path.listdir."""
    entries = path("/tmp").listdir()
    assert(type(entries) == "list", "listdir should return a list")

def test_listdir_specific():
    """Test Path.listdir with known contents."""
    exec("rm -rf /tmp/starkite_listdir_test")
    test_dir = path("/tmp/starkite_listdir_test")
    test_dir.mkdir()
    (test_dir / "file1.txt").write_text("a")
    (test_dir / "file2.txt").write_text("b")

    entries = test_dir.listdir()
    assert(len(entries) == 2, "should have 2 entries")

    # listdir returns [Path], extract names
    names = [e.name for e in entries]
    assert("file1.txt" in names, "should contain file1.txt")
    assert("file2.txt" in names, "should contain file2.txt")

    exec("rm -rf " + test_dir.string)

def test_walk():
    """Test Path.walk."""
    exec("rm -rf /tmp/starkite_walk_test")
    test_dir = path("/tmp/starkite_walk_test")
    (test_dir / "subdir").mkdir(parents=True)
    (test_dir / "file1.txt").write_text("a")
    (test_dir / "subdir" / "file2.txt").write_text("b")

    results = test_dir.walk()
    assert(type(results) == "list", "walk should return a list")
    assert(len(results) >= 2, "should have at least 2 directory entries")

    root, dirs, files = results[0]
    assert("subdir" in dirs, "dirs should contain subdir")
    assert("file1.txt" in files, "files should contain file1.txt")

    exec("rm -rf " + test_dir.string)

# ============================================================================
# File operation tests
# ============================================================================

def test_copy():
    """Test Path.copy_to."""
    src = path("/tmp/starkite_copy_src.txt")
    dst_path = "/tmp/starkite_copy_dst.txt"

    src.write_text("copy test")
    src.copy_to(dst_path)

    dst = path(dst_path)
    assert(dst.exists(), "destination should exist")
    assert(dst.read_text() == "copy test", "content should match")

    src.remove()
    dst.remove()

def test_move():
    """Test Path.move_to."""
    src = path("/tmp/starkite_move_src.txt")
    dst_path = "/tmp/starkite_move_dst.txt"

    src.write_text("move test")
    src.move_to(dst_path)

    dst = path(dst_path)
    assert(not src.exists(), "source should not exist")
    assert(dst.exists(), "destination should exist")
    assert(dst.read_text() == "move test", "content should match")

    dst.remove()

def test_touch_new_file():
    """Test Path.touch creates new file."""
    p = path("/tmp/starkite_touch_test.txt")
    if p.exists():
        p.remove()

    p.touch()
    assert(p.exists(), "touch should create file")
    assert(p.is_file(), "touched path should be a file")

    p.remove()

def test_touch_existing_file():
    """Test Path.touch updates existing file time."""
    p = path("/tmp/starkite_touch_existing.txt")
    p.write_text("content")

    p.touch()
    assert(p.exists(), "file should still exist")

    p.remove()

def test_truncate():
    """Test Path.truncate."""
    p = path("/tmp/starkite_truncate_test.txt")
    p.write_text("hello world")

    p.truncate(5)
    content = p.read_text()
    assert(content == "hello", "truncate should reduce file size")

    p.remove()

# ============================================================================
# Link tests
# ============================================================================

def test_symlink_and_readlink():
    """Test Path.symlink_to and Path.readlink."""
    exec("rm -f /tmp/starkite_symlink_src.txt /tmp/starkite_symlink_dst")
    test_file = path("/tmp/starkite_symlink_src.txt")
    test_link = path("/tmp/starkite_symlink_dst")

    test_file.write_text("test content")
    test_link.symlink_to("/tmp/starkite_symlink_src.txt")

    assert(test_link.is_symlink(), "link should be a symlink")
    target = test_link.readlink()
    assert(target.string == "/tmp/starkite_symlink_src.txt", "readlink should return target path")

    test_link.remove()
    test_file.remove()

def test_readlink_non_symlink():
    """Test Path.readlink with non-symlink returns error."""
    result = path("/etc/passwd").try_readlink()
    assert(result.ok == False, "readlink of regular file should fail")

def test_hardlink():
    """Test Path.hardlink_to."""
    exec("rm -f /tmp/starkite_hardlink_src.txt /tmp/starkite_hardlink_dst.txt")
    test_file = path("/tmp/starkite_hardlink_src.txt")
    test_link = path("/tmp/starkite_hardlink_dst.txt")

    test_file.write_text("hardlink test")
    test_link.hardlink_to("/tmp/starkite_hardlink_src.txt")

    assert(test_link.exists(), "hardlink should exist")
    assert(test_link.read_text() == "hardlink test", "hardlink should have same content")
    assert(not test_link.is_symlink(), "hardlink should not be symlink")

    test_link.remove()
    test_file.remove()

# ============================================================================
# Search and disk tests
# ============================================================================

def test_glob():
    """Test glob global alias."""
    files = glob("/etc/*.conf")
    assert(type(files) == "list", "should return a list")

def test_disk_usage():
    """Test Path.disk_usage."""
    usage = path("/").disk_usage()
    assert(type(usage) == "dict", "disk_usage should return dict")
    assert("total" in usage, "should have total")
    assert("used" in usage, "should have used")
    assert("free" in usage, "should have free")
    assert(usage["total"] > 0, "total should be positive")

# ============================================================================
# Path manipulation tests
# ============================================================================

def test_path_join_with_slash():
    """Test path joining with / operator."""
    p = path("a") / "b" / "c"
    assert(p.string == "a/b/c", "should join path components with / operator")

def test_path_parent():
    """Test Path.parent (replaces fs.dir)."""
    p = path("/var/log/syslog")
    assert(p.parent.string == "/var/log", "parent should return directory")

def test_path_name():
    """Test Path.name (replaces fs.base)."""
    p = path("/var/log/syslog")
    assert(p.name == "syslog", "name should return filename")

def test_path_suffix():
    """Test Path.suffix (replaces fs.ext)."""
    p = path("file.txt")
    assert(p.suffix == ".txt", "suffix should return extension")

def test_path_resolve():
    """Test Path.resolve (replaces fs.abs)."""
    p = path(".")
    result = p.resolve().string
    assert("/" in result, "resolve should return absolute path")

def test_path_rel():
    """Test relative path via fs.path (replaces fs.rel)."""
    p = path("/var/log/syslog")
    result = p.relative_to("/var")
    assert(result.string == "log/syslog", "relative_to should return relative path")

def test_path_clean():
    """Test Path.clean (replaces fs.clean)."""
    p = path("/var//log/../log/syslog")
    result = p.clean().string
    assert(result == "/var/log/syslog", "clean should normalize path")

def test_path_match():
    """Test Path.match (replaces fs.match)."""
    assert(path("file.txt").match("*.txt"), "should match pattern")
    assert(not path("file.log").match("*.txt"), "should not match different extension")
