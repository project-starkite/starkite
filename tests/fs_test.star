# fs_test.star - Tests for fs module (Path-first API)

is_windows = runtime.platform() == "windows"
temp_dir = env("TEMP", env("TMP", "/tmp"))
sys_file = "C:/Windows/win.ini" if is_windows else "/etc/passwd"
hosts_file = "C:/Windows/System32/drivers/etc/hosts" if is_windows else "/etc/hosts"

def rm_rf(p):
    p = path(p.string) if type(p) == "fs.path" else path(p)
    if not p.exists():
        return
    if p.is_dir():
        walk_entries = p.walk()
        for i in range(len(walk_entries) - 1, -1, -1):
            root, dirs, files = walk_entries[i]
            for f in files:
                (root / f).remove()
            if root.string != p.string:
                root.remove()
    p.remove()

# ============================================================================
# Read/Write tests
# ============================================================================

def test_read_text():
    """Test Path.read_text."""
    content = path(hosts_file).read_text()
    assert(content != "", "should read file content")

def test_write_and_read_text():
    """Test Path.write_text and Path.read_text."""
    p = path(temp_dir) / "starkite_test_file.txt"
    test_content = "hello from kite test"

    p.write_text(test_content)
    content = p.read_text()
    assert(content == test_content, "should read back written content")

    p.remove()

def test_read_write_bytes():
    """Test Path.read_bytes and Path.write_bytes."""
    p = path(temp_dir) / "starkite_bytes_test.bin"
    data = b"\x00\x01\x02\x03\xff\xfe\xfd"
    p.write_bytes(data)

    read_data = p.read_bytes()
    assert(read_data == data, "read_bytes should return same data")

    p.remove()

def test_global_read_write_aliases():
    """Test global aliases for read_text and write_text."""
    test_path = (path(temp_dir) / "starkite_global_alias_test.txt").string
    write_text(test_path, "global alias")
    content = read_text(test_path)
    assert(content == "global alias", "global aliases should work")

    path(test_path).remove()

# ============================================================================
# File check tests
# ============================================================================

def test_exists_file():
    """Test Path.exists with existing file."""
    assert(path(sys_file).exists(), sys_file + " should exist")

def test_exists_missing():
    """Test Path.exists with missing file."""
    assert(not path("/nonexistent/path/12345").exists(), "missing path should not exist")

def test_exists_global_alias():
    """Test exists global alias."""
    assert(exists(sys_file), "global exists should work")
    assert(not exists("/nonexistent/path/12345"), "global exists should return false for missing")

def test_is_file():
    """Test Path.is_file."""
    assert(path(sys_file).is_file(), sys_file + " should be a file")
    assert(not path(temp_dir).is_file(), temp_dir + " should not be a file")

def test_is_dir():
    """Test Path.is_dir."""
    assert(path(temp_dir).is_dir(), temp_dir + " should be a directory")
    assert(not path(sys_file).is_dir(), sys_file + " should not be a directory")

def test_is_symlink():
    """Test Path.is_symlink."""
    target = path("C:/Windows/notepad.exe") if is_windows else path("/bin/sh")
    result = target.is_symlink()
    assert(type(result) == "bool", "is_symlink should return bool")

def test_is_symlink_regular_file():
    """Test Path.is_symlink with regular file."""
    assert(not path(sys_file).is_symlink(), "regular file should not be symlink")

def test_stat():
    """Test Path.stat."""
    info = path(sys_file).stat()
    assert(info != None, "should return stat for existing file")
    assert("size" in info, "stat should have size")
    assert("mode" in info, "stat should have mode")
    assert("is_dir" in info, "stat should have is_dir")

# ============================================================================
# Directory tests
# ============================================================================

def test_mkdir_and_remove():
    """Test Path.mkdir and Path.remove."""
    p = path(temp_dir) / "starkite_test_dir"
    rm_rf(p)

    p.mkdir()
    assert(p.is_dir(), "directory should exist")

    p.remove()
    assert(not p.exists(), "directory should be removed")

def test_mkdir_parents():
    """Test Path.mkdir with parents option."""
    p = path(temp_dir) / "starkite_test" / "nested" / "dir"
    rm_rf(path(temp_dir) / "starkite_test")

    p.mkdir(parents=True)
    assert(p.is_dir(), "nested directory should exist")

    rm_rf(path(temp_dir) / "starkite_test")

def test_listdir():
    """Test Path.listdir."""
    entries = path(temp_dir).listdir()
    assert(type(entries) == "list", "listdir should return a list")

def test_listdir_specific():
    """Test Path.listdir with known contents."""
    test_dir = path(temp_dir) / "starkite_listdir_test"
    rm_rf(test_dir)
    test_dir.mkdir()
    (test_dir / "file1.txt").write_text("a")
    (test_dir / "file2.txt").write_text("b")

    entries = test_dir.listdir()
    assert(len(entries) == 2, "should have 2 entries")

    # listdir returns [Path], extract names
    names = [e.name for e in entries]
    assert("file1.txt" in names, "should contain file1.txt")
    assert("file2.txt" in names, "should contain file2.txt")

    rm_rf(test_dir)

def test_walk():
    """Test Path.walk."""
    test_dir = path(temp_dir) / "starkite_walk_test"
    rm_rf(test_dir)
    (test_dir / "subdir").mkdir(parents=True)
    (test_dir / "file1.txt").write_text("a")
    (test_dir / "subdir" / "file2.txt").write_text("b")

    results = test_dir.walk()
    assert(type(results) == "list", "walk should return a list")
    assert(len(results) >= 2, "should have at least 2 directory entries")

    root, dirs, files = results[0]
    assert("subdir" in dirs, "dirs should contain subdir")
    assert("file1.txt" in files, "files should contain file1.txt")

    rm_rf(test_dir)

# ============================================================================
# File operation tests
# ============================================================================

def test_copy():
    """Test Path.copy_to."""
    src = path(temp_dir) / "starkite_copy_src.txt"
    dst = path(temp_dir) / "starkite_copy_dst.txt"
    rm_rf(src)
    rm_rf(dst)

    src.write_text("copy test")
    src.copy_to(dst.string)

    assert(dst.exists(), "destination should exist")
    assert(dst.read_text() == "copy test", "content should match")

    src.remove()
    dst.remove()

def test_move():
    """Test Path.move_to."""
    src = path(temp_dir) / "starkite_move_src.txt"
    dst = path(temp_dir) / "starkite_move_dst.txt"
    rm_rf(src)
    rm_rf(dst)

    src.write_text("move test")
    src.move_to(dst.string)

    assert(not src.exists(), "source should not exist")
    assert(dst.exists(), "destination should exist")
    assert(dst.read_text() == "move test", "content should match")

    dst.remove()

def test_touch_new_file():
    """Test Path.touch creates new file."""
    p = path(temp_dir) / "starkite_touch_test.txt"
    rm_rf(p)

    p.touch()
    assert(p.exists(), "touch should create file")
    assert(p.is_file(), "touched path should be a file")

    p.remove()

def test_touch_existing_file():
    """Test Path.touch updates existing file time."""
    p = path(temp_dir) / "starkite_touch_existing.txt"
    rm_rf(p)
    p.write_text("content")

    p.touch()
    assert(p.exists(), "file should still exist")

    p.remove()

def test_truncate():
    """Test Path.truncate."""
    p = path(temp_dir) / "starkite_truncate_test.txt"
    rm_rf(p)
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
    test_file = path(temp_dir) / "starkite_symlink_src.txt"
    test_link = path(temp_dir) / "starkite_symlink_dst"
    rm_rf(test_file)
    rm_rf(test_link)

    test_file.write_text("test content")
    test_link.symlink_to(test_file.string)

    assert(test_link.is_symlink(), "link should be a symlink")
    target = test_link.readlink()
    assert(target.name == test_file.name, "readlink should return target path")

    test_link.remove()
    test_file.remove()

def test_readlink_non_symlink():
    """Test Path.readlink with non-symlink returns error."""
    result = path(sys_file).try_readlink()
    assert(result.ok == False, "readlink of regular file should fail")

def test_hardlink():
    """Test Path.hardlink_to."""
    test_file = path(temp_dir) / "starkite_hardlink_src.txt"
    test_link = path(temp_dir) / "starkite_hardlink_dst.txt"
    rm_rf(test_file)
    rm_rf(test_link)

    test_file.write_text("hardlink test")
    test_link.hardlink_to(test_file.string)

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
    pattern = "C:/Windows/*.ini" if is_windows else "/etc/*.conf"
    files = glob(pattern)
    assert(type(files) == "list", "should return a list")

def test_disk_usage():
    """Test Path.disk_usage."""
    root_p = "C:\\" if is_windows else "/"
    usage = path(root_p).disk_usage()
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
    assert(p.string in ["a/b/c", "a\\b\\c"], "should join path components with / operator")

def test_path_parent():
    """Test Path.parent (replaces fs.dir)."""
    p = path("var/log/syslog")
    assert(p.parent.name == "log", "parent should return directory")

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
    assert(len(result) > 0 and (result[0] == "/" or ":" in result or result.startswith("\\\\")), "resolve should return absolute path")

def test_path_rel():
    """Test relative path via fs.path (replaces fs.rel)."""
    p = path("var/log/syslog")
    result = p.relative_to("var")
    assert(result.string in ["log/syslog", "log\\syslog"], "relative_to should return relative path")

def test_path_clean():
    """Test Path.clean (replaces fs.clean)."""
    p = path("var//log/../log/syslog")
    result = p.clean().string
    assert(result in ["var/log/syslog", "var\\log\\syslog"], "clean should normalize path")

def test_path_match():
    """Test Path.match (replaces fs.match)."""
    assert(path("file.txt").match("*.txt"), "should match pattern")
    assert(not path("file.log").match("*.txt"), "should not match different extension")
