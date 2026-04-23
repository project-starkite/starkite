# ssh_integration_test.star — Integration tests for SSH module using test server

def test_exec_basic():
    """Test basic exec with password auth."""
    srv = ssh.testserver(user="testuser", password="testpass")
    srv.handle_exec(lambda cmd: ("hello\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        user="testuser",
        password="testpass",
        port=srv.port(),
        host_key_check=False,
        max_retries=0,
    )
    results = client.exec("echo hello")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "should succeed")
    assert(results[0].stdout == "hello\n", "stdout should match, got: %s" % results[0].stdout)
    assert(results[0].code == 0, "exit code should be 0")
    srv.shutdown()

def test_exec_nonzero_exit():
    """Test exec returning non-zero exit code."""
    srv = ssh.testserver(user="testuser", password="pass")
    srv.handle_exec(lambda cmd: ("", "error\n", 1))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="testuser", password="pass",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.exec("fail")
    assert(results[0].ok == False, "should fail")
    assert(results[0].code == 1, "exit code should be 1")
    assert(results[0].stderr == "error\n", "stderr should match")
    srv.shutdown()

def test_exec_with_key_auth():
    """Test exec with public key authentication."""
    key = ssh.test_key()
    srv = ssh.testserver(user="deploy")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="deploy", key=key.path,
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.exec("whoami")
    assert(results[0].ok == True, "key auth should work")
    assert(results[0].stdout == "ok\n", "stdout should match")
    srv.shutdown()

def test_exec_multi_host():
    """Test exec on multiple hosts (same server)."""
    srv = ssh.testserver(user="u", password="p")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1", "127.0.0.1", "127.0.0.1"],
        user="u", password="p",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.exec("test")
    assert(len(results) == 3, "should have 3 results, got %d" % len(results))
    for r in results:
        assert(r.ok == True, "all should succeed")
    srv.shutdown()

def test_try_exec():
    """Test try_exec returns Result wrapper."""
    srv = ssh.testserver(user="u", password="p")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="u", password="p",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    r = client.try_exec("test")
    assert(r.ok == True, "should succeed")
    srv.shutdown()

def test_upload():
    """Test SCP upload."""
    srv = ssh.testserver(user="u", password="p")
    srv.start()

    # Create a local temp file
    path = "/tmp/crsh_ssh_upload_test.txt"
    write_text(path, "upload content")

    client = ssh.config(
        hosts=["127.0.0.1"], user="u", password="p",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.upload(path, "/remote/file.txt")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "upload should succeed")
    assert(results[0].bytes > 0, "should transfer bytes")

    uploaded = srv.uploaded("/remote/file.txt")
    assert(uploaded != None, "server should have received file")
    assert(uploaded.content == "upload content", "content should match, got: %s" % uploaded.content)

    remove(path)
    srv.shutdown()

def test_download():
    """Test SCP download."""
    srv = ssh.testserver(user="u", password="p")
    srv.add_file("/remote/data.txt", "download content", "0644")
    srv.start()

    local_path = "/tmp/crsh_ssh_download_test.txt"
    client = ssh.config(
        hosts=["127.0.0.1"], user="u", password="p",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.download("/remote/data.txt", local_path)
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "download should succeed")

    content = read_text(local_path)
    assert(content == "download content", "downloaded content should match, got: %s" % content)

    remove(local_path)
    srv.shutdown()

def test_auth_failure():
    """Test authentication failure with wrong password."""
    srv = ssh.testserver(user="u", password="correct")
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="u", password="wrong",
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    r = client.try_exec("test")
    assert(r.ok == False, "should fail with wrong password")
    srv.shutdown()
