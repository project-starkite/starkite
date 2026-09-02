# ssh_integration_test.star — Integration tests for SSH module using test server

def test_exec_basic():
    """Test basic exec with password auth."""
    srv = ssh.test_server(user="testuser", password="testpass")
    srv.handle_exec(lambda cmd: ("hello\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={
            "user": "testuser",
            "password": "testpass",
        },
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
    srv = ssh.test_server(user="testuser", password="pass")
    srv.handle_exec(lambda cmd: ("", "error\n", 1))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "testuser", "password": "pass"},
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
    srv = ssh.test_server(user="deploy")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "deploy", "key": key.path},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.exec("whoami")
    assert(results[0].ok == True, "key auth should work")
    assert(results[0].stdout == "ok\n", "stdout should match")
    srv.shutdown()

def test_exec_multi_host():
    """Test exec on multiple hosts (same server)."""
    srv = ssh.test_server(user="u", password="p")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1", "127.0.0.1", "127.0.0.1"],
        auth={"user": "u", "password": "p"},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.exec("test")
    assert(len(results) == 3, "should have 3 results, got %d" % len(results))
    for r in results:
        assert(r.ok == True, "all should succeed")
    srv.shutdown()

def test_try_exec():
    """Test try_exec returns Result wrapper."""
    srv = ssh.test_server(user="u", password="p")
    srv.handle_exec(lambda cmd: ("ok\n", "", 0))
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "u", "password": "p"},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    r = client.try_exec("test")
    assert(r.ok == True, "should succeed")
    srv.shutdown()

def test_upload():
    """Test SCP upload."""
    srv = ssh.test_server(user="u", password="p")
    srv.start()

    # Create a local temp file
    path = (fs.path(temp_dir()) / "crsh_ssh_upload_test.txt").string
    write_text(path, "upload content")

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "u", "password": "p"},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.upload(path, "/remote/file.txt")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "upload should succeed")
    assert(results[0].bytes > 0, "should transfer bytes")

    uploaded = srv.uploaded("/remote/file.txt")
    assert(uploaded != None, "server should have received file")
    assert(uploaded.content == "upload content", "content should match, got: %s" % uploaded.content)

    fs.path(path).remove()
    srv.shutdown()

def test_download():
    """Test SCP download."""
    srv = ssh.test_server(user="u", password="p")
    srv.add_file("/remote/data.txt", "download content", "0644")
    srv.start()

    local_path = (fs.path(temp_dir()) / "crsh_ssh_download_test.txt").string
    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "u", "password": "p"},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    results = client.download("/remote/data.txt", local_path)
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "download should succeed")

    content = read_text(local_path)
    assert(content == "download content", "downloaded content should match, got: %s" % content)

    fs.path(local_path).remove()
    srv.shutdown()

def test_auth_failure():
    """Test authentication failure with wrong password."""
    # Prevent falling through to SSH-agent auth on dev machines with
    # SSH_AUTH_SOCK set — the test server accepts any key when no
    # authorized_keys are configured, so agent auth would succeed and
    # defeat the point of the test.
    saved = os.env("SSH_AUTH_SOCK")
    os.setenv("SSH_AUTH_SOCK", "")

    srv = ssh.test_server(user="u", password="correct")
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"],
        auth={"user": "u", "password": "wrong"},
        port=srv.port(), host_key_check=False, max_retries=0,
    )
    r = client.try_exec("test")
    assert(r.ok == False, "should fail with wrong password")
    srv.shutdown()

    # Restore
    os.setenv("SSH_AUTH_SOCK", saved)

def test_keyscan_basic():
    """Test discovering server public host key with ssh.keyscan."""
    srv = ssh.test_server(user="u", password="p")
    srv.start()

    keys = ssh.keyscan(hosts=["127.0.0.1"], port=srv.port(), timeout="2s")
    assert(len(keys) == 1, "expected 1 scanned key")
    k = keys[0]
    assert(k.host == "127.0.0.1", "host should match")
    assert(k.port == srv.port(), "port should match")
    assert(k.type == "ssh-ed25519", "type should be ssh-ed25519")
    assert(k.public_key == srv.host_key(), "public_key should match server key")
    assert(k.fingerprint == srv.fingerprint(), "fingerprint should match server fingerprint")
    assert("ssh-ed25519" in k.line, "line should contain algorithm")
    assert(k.hashed_line.startswith("|1|"), "hashed_line should start with |1|")

    srv.shutdown()

def test_keyscan_save_and_strict_host_key_check():
    """Test scanning host key, saving to known_hosts, and executing with strict host_key_check=True."""
    srv = ssh.test_server(user="u", password="p")
    srv.handle_exec(lambda cmd: ("success\n", "", 0))
    srv.start()

    kh_path = (fs.path(temp_dir()) / "test_keyscan_known_hosts").string

    # 1. Scan and save host key to file
    ssh.keyscan(
        hosts = ["127.0.0.1"],
        port  = srv.port(),
        save  = True,
        path  = kh_path,
    )
    assert(fs.path(kh_path).exists(), "known_hosts file should be created")

    # 2. Connect with strict host_key_check=True against that known_hosts file
    client = ssh.config(
        hosts            = ["127.0.0.1"],
        port             = srv.port(),
        auth             = {"user": "u", "password": "p"},
        known_hosts_file = kh_path,
        host_key_check   = True,
    )
    results = client.exec("test")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "exec with verified host key should succeed")
    assert(results[0].stdout == "success\n", "output should match")

    # Clean up
    fs.path(kh_path).remove()
    srv.shutdown()

def test_client_keyscan_method():
    """Test client.keyscan inheriting hosts and port."""
    srv = ssh.test_server(user="u", password="p")
    srv.start()

    client = ssh.config(
        hosts = ["127.0.0.1"],
        port  = srv.port(),
        auth  = {"user": "u", "password": "p"},
    )
    keys = client.keyscan()
    assert(len(keys) == 1, "client.keyscan should return 1 key")
    assert(keys[0].public_key == srv.host_key(), "public key should match")

    srv.shutdown()

def test_try_keyscan_error_handling():
    """Test ssh.try_keyscan and client.try_keyscan on closed ports."""
    r1 = ssh.try_keyscan(hosts=["127.0.0.1"], port=1, timeout="200ms")
    assert(r1.ok == False, "try_keyscan should fail on closed port")
    assert(len(r1.error) > 0, "error message should be populated")

    client = ssh.config(hosts=["127.0.0.1"], port=1, timeout="200ms", auth={"user": "u"})
    r2 = client.try_keyscan()
    assert(r2.ok == False, "client.try_keyscan should fail on closed port")
    assert(len(r2.error) > 0, "error message should be populated")

def test_key_check_basic():
    """Test remote public key acceptance probe."""
    srv = ssh.test_server(user="u", password="p")
    dummy_kp = ssh.keygen(type="ed25519")
    srv.add_authorized_key(dummy_kp.public_key)
    srv.start()

    kp = ssh.keygen(type="ed25519")

    # 1. Probe before key is installed -> accepted should be False
    res1 = ssh.key_check(
        key            = kp.public_key,
        hosts          = ["127.0.0.1"],
        port           = srv.port(),
        user           = "u",
        host_key_check = False,
    )
    assert(len(res1) == 1, "expected 1 result")
    assert(res1[0].accepted == False, "key should not be accepted yet")
    assert(res1[0].ok == True, "probe should complete successfully")
    assert(res1[0].host == "127.0.0.1", "host should match")
    assert(res1[0].user == "u", "user should match")

    # 2. Add public key to server's authorized list
    srv.add_authorized_key(kp.public_key)

    # 3. Probe after key is installed -> accepted should be True!
    res2 = ssh.key_check(
        key            = kp.public_key,
        hosts          = ["127.0.0.1"],
        port           = srv.port(),
        user           = "u",
        host_key_check = False,
    )
    assert(len(res2) == 1, "expected 1 result")
    assert(res2[0].accepted == True, "key should now be accepted")
    assert(res2[0].ok == True, "probe should succeed")
    assert(res2[0].fingerprint == kp.fingerprint, "fingerprint should match")

    srv.shutdown()

def test_client_key_check_method():
    """Test client.key_check method."""
    srv = ssh.test_server(user="deploy")
    srv.start()

    kp = ssh.keygen(type="ed25519")
    srv.add_authorized_key(kp.public_key)

    client = ssh.config(
        hosts          = ["127.0.0.1"],
        port           = srv.port(),
        auth           = {"user": "deploy"},
        host_key_check = False,
    )
    results = client.key_check(kp.public_key)
    assert(len(results) == 1, "expected 1 result")
    assert(results[0].accepted == True, "key should be accepted")

    srv.shutdown()

def test_copy_id_key_check_optimization():
    """Test copy_id skips remote install when key_check=True and key is already authorized."""
    srv = ssh.test_server(user="deploy")
    srv.start()

    kp = ssh.keygen(type="ed25519")
    srv.add_authorized_key(kp.public_key)

    client = ssh.config(
        hosts          = ["127.0.0.1"],
        port           = srv.port(),
        auth           = {"user": "deploy"},
        host_key_check = False,
    )
    results = client.copy_id(kp.public_key, key_check=True)
    assert(len(results) == 1, "expected 1 result")
    assert(results[0].ok == True, "result should be ok")
    assert("ALREADY INSTALLED" in results[0].stdout, "output should indicate already installed")

    srv.shutdown()

def test_try_key_check_unreachable():
    """Test try_key_check on closed port."""
    kp = ssh.keygen(type="ed25519")
    res = ssh.try_key_check(
        hosts          = ["127.0.0.1"],
        port           = 1,
        user           = "u",
        key            = kp.public_key,
        timeout        = "200ms",
        host_key_check = False,
    )
    # The call should complete without raising
    assert(res.ok == True, "try_key_check should not raise Go error")
    item = res.value[0]
    assert(item.ok == False, "item ok should be False on closed port")
    assert(len(item.error) > 0, "item error should describe connection failure")


