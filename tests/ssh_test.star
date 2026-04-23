# ssh_test.star - Tests for ssh module (dry-run mode)

# ============================================================================
# Config creation tests
# ============================================================================

def test_config_basic():
    client = ssh.config(hosts=["host1", "host2"], user="root", key="/tmp/id_rsa", dry_run=True)
    assert(type(client) == "ssh.client", "config should return ssh.client")

def test_config_hosts_attribute():
    client = ssh.config(hosts=["web1", "web2", "web3"], user="deploy", dry_run=True)
    hosts = client.hosts
    assert(len(hosts) == 3, "should have 3 hosts")
    assert(hosts[0] == "web1", "first host should be web1")
    assert(hosts[1] == "web2", "second host should be web2")
    assert(hosts[2] == "web3", "third host should be web3")

def test_config_truth():
    client_with_hosts = ssh.config(hosts=["h1"], user="u", dry_run=True)
    assert(client_with_hosts, "client with hosts should be truthy")
    client_no_hosts = ssh.config(user="u", dry_run=True)
    assert(not client_no_hosts, "client without hosts should be falsy")

def test_config_all_params():
    client = ssh.config(
        hosts=["host1"],
        user="admin",
        key="/tmp/key",
        key_passphrase="secret",
        password="pass",
        port=2222,
        timeout="60s",
        max_retries=5,
        exec_policy="linear",
        jump_host="bastion.example.com",
        known_hosts_file="/tmp/known_hosts",
        host_key_check=False,
        keep_alive_interval="15s",
        keep_alive_max=5,
        sudo=True,
        as_user="deploy",
        cwd="/opt/app",
        dry_run=True,
    )
    assert(type(client) == "ssh.client", "config with all params should work")

# ============================================================================
# Exec dry-run tests
# ============================================================================

def test_exec_dry_run():
    client = ssh.config(hosts=["host1", "host2"], user="root", dry_run=True)
    results = client.exec("uname -a")
    assert(type(results) == "list", "exec should return a list")
    assert(len(results) == 2, "should have result per host")

    r = results[0]
    assert(r.host == "host1", "first result host should be host1")
    assert(r.ok == True, "dry run should be ok")
    assert(r.code == 0, "dry run exit code should be 0")
    assert(r.dry_run == True, "should have dry_run flag")
    assert("DRY RUN" in r.stdout, "stdout should indicate dry run")
    assert(r.stderr == "", "stderr should be empty")

def test_exec_dry_run_with_sudo():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.exec("systemctl restart app", sudo=True)
    r = results[0]
    assert("sudo" in r.stdout, "dry run stdout should contain sudo command")

def test_exec_dry_run_with_as_user():
    client = ssh.config(hosts=["host1"], user="root", dry_run=True)
    results = client.exec("whoami", sudo=True, as_user="www-data")
    r = results[0]
    assert("sudo -u www-data" in r.stdout, "should contain sudo -u www-data")

def test_exec_dry_run_with_cwd():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.exec("ls", cwd="/opt/app")
    r = results[0]
    assert("cd /opt/app" in r.stdout, "should contain cd /opt/app")

def test_exec_dry_run_with_env():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.exec("app start", env={"PORT": "8080"})
    r = results[0]
    assert("PORT=" in r.stdout, "should contain env var")

def test_exec_empty_hosts():
    client = ssh.config(user="deploy", dry_run=True)
    results = client.exec("echo hello")
    assert(type(results) == "list", "should return list")
    assert(len(results) == 0, "empty hosts should produce empty results")

# ============================================================================
# Upload dry-run tests
# ============================================================================

def test_upload_dry_run():
    client = ssh.config(hosts=["host1", "host2"], user="deploy", dry_run=True)
    results = client.upload("/local/app.tar.gz", "/remote/app.tar.gz")
    assert(type(results) == "list", "upload should return a list")
    assert(len(results) == 2, "should have result per host")

    r = results[0]
    assert(r.host == "host1", "first result host should be host1")
    assert(r.ok == True, "dry run should be ok")
    assert(r.bytes == 0, "dry run bytes should be 0")
    assert(r.src == "/local/app.tar.gz", "src should match")
    assert(r.dst == "/remote/app.tar.gz", "dst should match")
    assert(r.dry_run == True, "should have dry_run flag")

def test_upload_dry_run_with_mode():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.upload("/local/script.sh", "/remote/script.sh", mode="0755")
    assert(len(results) == 1, "should have 1 result")
    assert(results[0].ok == True, "should be ok")

def test_upload_empty_hosts():
    client = ssh.config(user="deploy", dry_run=True)
    results = client.upload("/local/file", "/remote/file")
    assert(len(results) == 0, "empty hosts should produce empty results")

# ============================================================================
# Download dry-run tests
# ============================================================================

def test_download_dry_run():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.download("/remote/data.csv", "/local/data.csv")
    assert(type(results) == "list", "download should return a list")
    assert(len(results) == 1, "should have 1 result")

    r = results[0]
    assert(r.host == "host1", "result host should be host1")
    assert(r.ok == True, "dry run should be ok")
    assert(r.bytes == 0, "dry run bytes should be 0")
    assert(r.src == "/remote/data.csv", "src should match")
    assert(r.dst == "/local/data.csv", "dst should match")
    assert(r.dry_run == True, "should have dry_run flag")

def test_download_empty_hosts():
    client = ssh.config(user="deploy", dry_run=True)
    results = client.download("/remote/file", "/local/file")
    assert(len(results) == 0, "empty hosts should produce empty results")

# ============================================================================
# try_ prefix tests
# ============================================================================

def test_try_exec():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    r = client.try_exec("echo hello")
    assert(type(r) == "Result", "try_exec should return Result")
    assert(r.ok == True, "try_exec should succeed in dry run")
    # value contains the list of SSHResult
    results = r.value
    assert(type(results) == "list", "value should be a list")
    assert(len(results) == 1, "should have 1 result")

def test_try_upload():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    r = client.try_upload("/local/file", "/remote/file")
    assert(type(r) == "Result", "try_upload should return Result")
    assert(r.ok == True, "try_upload should succeed in dry run")
    results = r.value
    assert(type(results) == "list", "value should be a list")

def test_try_download():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    r = client.try_download("/remote/file", "/local/file")
    assert(type(r) == "Result", "try_download should return Result")
    assert(r.ok == True, "try_download should succeed in dry run")
    results = r.value
    assert(type(results) == "list", "value should be a list")

# ============================================================================
# Attr/AttrNames tests
# ============================================================================

def test_attr_names():
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    # Verify all expected methods are accessible
    assert(client.exec != None, "exec attr should exist")
    assert(client.upload != None, "upload attr should exist")
    assert(client.download != None, "download attr should exist")
    assert(client.hosts != None, "hosts attr should exist")
    assert(client.try_exec != None, "try_exec attr should exist")
    assert(client.try_upload != None, "try_upload attr should exist")
    assert(client.try_download != None, "try_download attr should exist")

def test_string_repr():
    client = ssh.config(hosts=["h1", "h2"], user="root", dry_run=True)
    s = str(client)
    assert("ssh.client" in s, "string repr should contain ssh.client")
    assert("h1" in s, "string repr should contain host")

def test_type():
    client = ssh.config(hosts=["h1"], user="root", dry_run=True)
    assert(type(client) == "ssh.client", "type should be ssh.client")
