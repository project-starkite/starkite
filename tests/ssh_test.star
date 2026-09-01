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
        exec_max_workers=16,
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
    assert(client.exec_max_workers == 16, "exec_max_workers should be 16")
    assert(client.exec_policy == "linear", "exec_policy should be linear")

def test_config_exec_max_workers():
    c = ssh.config(hosts=["h1", "h2"], user="root", exec_max_workers=32, dry_run=True)
    assert(c.exec_max_workers == 32, "exec_max_workers should be 32")
    results = c.exec("uptime", exec_max_workers=8)
    assert(len(results) == 2, "should execute across 2 hosts")

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

def test_exec_dry_run_dual_invocation_args():
    """Test SSH exec with dual invocation structured argument list."""
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.exec("git", ["commit", "-m", "release v0.1.0"])
    assert(len(results) == 1, "should return 1 result")
    r = results[0]
    assert("release v0.1.0" in r.stdout, "should format dual invocation command in dry-run stdout")

def test_exec_dry_run_quoted_string():
    """Test SSH exec with quoted command string."""
    client = ssh.config(hosts=["host1"], user="deploy", dry_run=True)
    results = client.exec('k3s kubectl apply -f "deploy.yaml"')
    assert(len(results) == 1, "should return 1 result")
    r = results[0]
    assert("deploy.yaml" in r.stdout, "should handle quoted command string in dry-run stdout")

def test_ssh_exec_oneshot():
    """Test module-level one-shot ssh.exec."""
    results = ssh.exec("uptime", hosts=["host1", "host2"], user="deploy", dry_run=True)
    assert(len(results) == 2, "should execute across 2 hosts")
    assert(results[0].host == "host1", "first host should be host1")
    assert("uptime" in results[0].stdout, "should execute uptime")

def test_ssh_exec_oneshot_dual_args():
    """Test module-level one-shot ssh.exec with structured args."""
    results = ssh.exec("git", ["status", "-s"], hosts=["host1"], user="deploy", dry_run=True)
    assert(len(results) == 1, "should return 1 result")
    assert("git status -s" in results[0].stdout, "should format dual args")

def test_ssh_try_exec_oneshot():
    """Test module-level try_exec."""
    res = ssh.try_exec("hostname", hosts=["host1"], user="deploy", dry_run=True)
    assert(res.ok == True, "try_exec dry_run should be ok")
    assert(len(res.value) == 1, "should return 1 result")

def test_exec_commands_pipeline_dry_run():
    """Test client.exec with commands list."""
    client = ssh.config(hosts=["web1", "web2"], user="deploy", dry_run=True)
    results = client.exec(
        commands = [
            "git pull origin main",
            "npm install",
            "systemctl restart webapp",
        ],
        exec_on_error = "stop",
    )
    assert(len(results) == 2, "should return results for 2 hosts")
    r = results[0]
    assert(r.host == "web1", "first host should be web1")
    assert(r.ok == True, "batch should be ok in dry-run")
    assert(r.stopped_early == False, "should not be stopped early")
    assert(len(r.steps) == 3, "should contain 3 step results")
    assert(r.steps[0].cmd == "git pull origin main", "step 0 cmd should match")
    assert("npm install" in r.steps[1].stdout, "step 1 stdout should contain npm install")

def test_ssh_exec_commands_oneshot():
    """Test module-level ssh.exec with commands list."""
    results = ssh.exec(
        commands = ["echo 1", "echo 2"],
        hosts = ["host1"],
        user = "deploy",
        exec_on_error = "continue",
        dry_run = True,
    )
    assert(len(results) == 1, "should return result for 1 host")
    batch = results[0]
    assert(batch.ok == True, "batch should be ok")
    assert(len(batch.steps) == 2, "should have 2 steps")
    assert(batch.steps[0].cmd == "echo 1", "first step cmd should match")

def test_client_exec_positional_commands_list():
    """Test client.exec passing commands as positional list."""
    client = ssh.config(hosts=["h1"], user="deploy", dry_run=True)
    results = client.exec(["ls -la", "df -h"])
    assert(len(results) == 1, "should return 1 host result")
    assert(len(results[0].steps) == 2, "should have 2 step results")
    assert(results[0].steps[0].cmd == "ls -la", "step 0 cmd match")
    assert(results[0].steps[1].cmd == "df -h", "step 1 cmd match")

def test_client_exec_positional_commands_tuple():
    """Test client.exec passing commands as positional tuple."""
    client = ssh.config(hosts=["h1"], user="deploy", dry_run=True)
    results = client.exec(("uptime", "whoami"))
    assert(len(results) == 1, "should return 1 host result")
    assert(len(results[0].steps) == 2, "should have 2 step results")

def test_client_exec_commands_with_options():
    """Test client.exec commands with sudo, cwd, and env options."""
    client = ssh.config(hosts=["h1"], user="deploy", dry_run=True)
    results = client.exec(
        commands = ["make build", "make install"],
        cwd = "/opt/myapp",
        sudo = True,
        as_user = "app",
        env = {"GOOS": "linux"},
    )
    assert(len(results) == 1, "should return 1 host result")
    assert(results[0].ok == True, "should be ok")
    assert(results[0].stopped_early == False, "should not be stopped early")
    assert("cd /opt/myapp" in results[0].steps[0].stdout, "stdout should contain cd")
    assert("sudo -u app" in results[0].steps[0].stdout, "stdout should contain sudo -u app")
    assert('GOOS="linux"' in results[0].steps[0].stdout, "stdout should contain env var")

def test_client_try_exec_commands():
    """Test client.try_exec with multi-command pipeline."""
    client = ssh.config(hosts=["h1"], user="deploy", dry_run=True)
    res = client.try_exec(commands=["echo 1", "echo 2"])
    assert(res.ok == True, "try_exec on dry_run should be ok")
    batch = res.value[0]
    assert(batch.ok == True, "batch ok should be true")
    assert(len(batch.steps) == 2, "should have 2 steps")

def test_ssh_try_exec_commands_oneshot():
    """Test ssh.try_exec with multi-command pipeline at module level."""
    res = ssh.try_exec(
        commands = ["date", "cal"],
        hosts = ["srv1", "srv2"],
        user = "deploy",
        dry_run = True,
    )
    assert(res.ok == True, "try_exec on dry_run should be ok")
    assert(len(res.value) == 2, "should return 2 hosts")
    assert(len(res.value[0].steps) == 2, "each host should have 2 steps")

def test_ssh_exec_commands_with_fleet():
    """Test ssh.exec with commands targeting a Fleet instance."""
    compute_fleet = fleet.new([
        {"id": "node-1", "name": "node-1", "address": "10.0.1.10"},
        {"id": "node-2", "name": "node-2", "address": "10.0.1.11"},
    ])
    results = ssh.exec(
        commands = ["cat /etc/os-release", "uname -r"],
        fleet = compute_fleet,
        user = "deploy",
        exec_max_workers = 4,
        dry_run = True,
    )
    assert(len(results) == 2, "should return 2 hosts")
    assert(results[0].host == "10.0.1.10", "host 1 should match")
    assert(results[1].host == "10.0.1.11", "host 2 should match")
    assert(len(results[0].steps) == 2, "should contain 2 steps")

def test_ssh_exec_single_host_string():
    """Test ssh.exec with single host string shortcut."""
    results = ssh.exec("hostname", hosts="192.168.1.50", user="root", dry_run=True)
    assert(len(results) == 1, "should return 1 host")
    assert(results[0].host == "192.168.1.50", "host should match")
    assert("hostname" in results[0].stdout, "stdout should contain hostname")

def test_ssh_config_jump_host_params():
    """Test jump_host, jump_user, jump_port configuration and attributes."""
    client = ssh.config(
        hosts = ["picluster-0", "picluster-1"],
        user = "pi",
        jump_host = "rbp4-1",
        jump_user = "vladimir",
        jump_port = 2222,
        dry_run = True,
    )
    assert(client.jump_host == "rbp4-1", "jump_host should be rbp4-1")
    assert(client.jump_user == "vladimir", "jump_user should be vladimir")
    assert(client.jump_port == 2222, "jump_port should be 2222")

def test_ssh_exec_with_jump_host():
    """Test one-shot ssh.exec through a jump host."""
    results = ssh.exec(
        "uname -m",
        hosts = ["picluster-0"],
        user = "pi",
        jump_host = "rbp4-1",
        jump_user = "vladimir",
        dry_run = True,
    )
    assert(len(results) == 1, "should return 1 result")
    assert(results[0].host == "picluster-0", "target host should be picluster-0")
    assert("uname -m" in results[0].stdout, "stdout should contain uname -m")

def test_ssh_keygen_ed25519():
    """Test in-memory Ed25519 key generation."""
    kp = ssh.keygen(type="ed25519", comment="test-ed25519-key")
    assert(kp.type == "ed25519", "type should be ed25519")
    assert(kp.comment == "test-ed25519-key", "comment should match")
    assert(kp.public_key.startswith("ssh-ed25519 "), "public_key should have ssh-ed25519 prefix")
    assert("test-ed25519-key" in kp.public_key, "public key should contain comment")
    assert(kp.fingerprint.startswith("SHA256:"), "fingerprint should start with SHA256:")
    assert("BEGIN OPENSSH PRIVATE KEY" in kp.private_key, "private key should have OpenSSH header")

def test_ssh_keygen_rsa():
    """Test in-memory RSA key generation."""
    kp = ssh.keygen(type="rsa", bits=2048, comment="admin@test")
    assert(kp.type == "rsa", "type should be rsa")
    assert(kp.public_key.startswith("ssh-rsa "), "public_key should have ssh-rsa prefix")
    assert("admin@test" in kp.public_key, "public key should contain comment")

def test_ssh_keygen_ecdsa():
    """Test in-memory ECDSA key generation."""
    kp = ssh.keygen(type="ecdsa", bits=256, comment="ecdsa-test")
    assert(kp.type == "ecdsa", "type should be ecdsa")
    assert(kp.public_key.startswith("ecdsa-sha2-nistp256 "), "public_key should have ecdsa-sha2-nistp256 prefix")

def test_ssh_keygen_disk_persistence():
    """Test key generation with disk persistence."""
    key_file = (fs.path(temp_dir()) / "starkite_test_keygen_ed25519").string
    kp = ssh.keygen(type="ed25519", path=key_file, overwrite=True)
    assert(kp.path == key_file, "path should match")
    assert(kp.pub_path == key_file + ".pub", "pub_path should match")
    assert(fs.path(key_file).exists() == True, "private key file should exist on disk")
    assert(fs.path(key_file + ".pub").exists() == True, "public key file should exist on disk")
    # Clean up
    fs.path(key_file).remove()
    fs.path(key_file + ".pub").remove()

def test_ssh_try_keygen():
    """Test ssh.try_keygen error handling."""
    # Invalid key type
    res = ssh.try_keygen(type="invalid_type")
    assert(res.ok == False, "try_keygen should fail for invalid key type")
    assert(res.error != None and len(res.error) > 0, "error message should be populated")

    # Success case
    ok_res = ssh.try_keygen(type="ed25519")
    assert(ok_res.ok == True, "try_keygen should succeed for ed25519")
    assert(ok_res.value.type == "ed25519", "value should be SSHKeyPair")

def test_client_copy_id_dry_run():
    """Test client.copy_id in dry-run mode."""
    kp = ssh.keygen(type="ed25519", comment="dry-run-copy")
    client = ssh.config(hosts=["web1", "web2"], user="pi", dry_run=True)
    results = client.copy_id(key=kp.public_key)
    assert(len(results) == 2, "should return 2 results")
    assert(results[0].ok == True, "result 0 ok")
    assert(results[1].ok == True, "result 1 ok")
    assert("[DRY RUN] Would install public key" in results[0].stdout, "stdout should contain dry run msg")

def test_ssh_copy_id_oneshot_dry_run():
    """Test one-shot ssh.copy_id across a fleet in dry-run mode."""
    kp = ssh.keygen(type="ed25519", comment="oneshot-copy")
    results = ssh.copy_id(
        key = kp.public_key,
        hosts = ["srv1", "srv2", "srv3"],
        user = "pi",
        jump_host = "bastion.lan",
        jump_user = "admin",
        dry_run = True,
    )
    assert(len(results) == 3, "should return 3 results")
    assert(results[0].host == "srv1", "host should match")
    assert(results[0].ok == True, "ok should be True")

def test_ssh_try_copy_id_invalid():
    """Test ssh.try_copy_id with invalid key data."""
    res = ssh.try_copy_id(key="invalid-key-data", hosts=["srv1"], user="pi", dry_run=True)
    assert(res.ok == False, "try_copy_id should fail for invalid key")
    assert(res.error != None and len(res.error) > 0, "error message should be present")

def test_ssh_config_use_agent():
    """Test use_agent parameter and client attribute."""
    client = ssh.config(
        hosts = ["srv1"],
        user = "pi",
        use_agent = True,
        dry_run = True,
    )
    assert(client.use_agent == True, "client.use_agent should be True")
