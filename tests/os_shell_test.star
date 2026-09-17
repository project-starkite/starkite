# os_shell_test.star - Tests for os.shell and shell factory shortcuts

def test_shell_constructor_defaults():
    """Test os.shell() returns Shell object with proper defaults."""
    sh_inst = os.shell()
    assert(type(sh_inst) == "os.shell", "expected type 'os.shell', got %s" % type(sh_inst))
    assert(len(sh_inst.command) > 0, "default command should not be empty")
    assert(len(sh_inst.flag) > 0, "default flag should not be empty")
    assert(sh_inst.cwd == "", "default cwd should be empty string")
    assert(sh_inst.timeout == "60s", "default timeout should be 60s")

def test_shell_constructor_custom():
    """Test os.shell() with explicit options."""
    sh_inst = os.shell(
        command = "/bin/custom_sh",
        flag = "-c",
        cwd = "/tmp",
        timeout = "30s",
        env = {"KEY1": "val1"},
    )
    assert(sh_inst.command == "/bin/custom_sh", "command should match")
    assert(sh_inst.flag == "-c", "flag should match")
    assert(sh_inst.cwd == "/tmp", "cwd should match")
    assert(sh_inst.timeout == "30s", "timeout should match")

def test_shell_factory_presets():
    """Test factory shortcuts configure expected shell command and flag presets."""
    sh_obj = os.sh()
    assert(sh_obj.command == "/bin/sh", "os.sh() command should be /bin/sh")
    assert(sh_obj.flag == "-c", "os.sh() flag should be -c")

    bash_obj = os.bash()
    assert(bash_obj.command == "/bin/bash", "os.bash() command should be /bin/bash")
    assert(bash_obj.flag == "-c", "os.bash() flag should be -c")

    zsh_obj = os.zsh()
    assert(zsh_obj.command == "/bin/zsh", "os.zsh() command should be /bin/zsh")
    assert(zsh_obj.flag == "-c", "os.zsh() flag should be -c")

    cmd_obj = os.cmdexe()
    assert(cmd_obj.command == "cmd.exe", "os.cmdexe() command should be cmd.exe")
    assert(cmd_obj.flag == "/c", "os.cmdexe() flag should be /c")

    pwsh_obj = os.powershell()
    assert(pwsh_obj.flag == "-Command", "os.powershell() flag should be -Command")
    assert("pwsh" in pwsh_obj.command or "powershell" in pwsh_obj.command, "os.powershell() command should resolve to pwsh or powershell")

def test_shell_global_aliases():
    """Test shell factory aliases exported globally."""
    s1 = sh()
    assert(type(s1) == "os.shell", "sh() alias should return shell object")
    assert(s1.command == "/bin/sh", "sh() alias should have /bin/sh")

    s2 = bash()
    assert(s2.command == "/bin/bash", "bash() alias should have /bin/bash")

    s3 = cmdexe()
    assert(s3.command == "cmd.exe", "cmdexe() alias should have cmd.exe")

    s4 = powershell()
    assert(s4.flag == "-Command", "powershell() alias should have -Command")

def test_shell_exec_and_try_exec():
    """Test shell exec and try_exec script execution."""
    if runtime.platform() == "windows":
        sh_inst = os.cmdexe()
        out = sh_inst.exec("echo hello_world")
        assert("hello_world" in out, "exec output should contain hello_world")

        res = sh_inst.try_exec("echo try_success")
        assert(res.ok, "try_exec should succeed")
        assert("try_success" in res.stdout, "stdout should contain try_success")

        res_err = sh_inst.try_exec("exit 42")
        assert(not res_err.ok, "try_exec with non-zero exit should not be ok")
        assert(res_err.code == 42, "exit code should be 42")
    else:
        sh_inst = os.sh()
        out = sh_inst.exec("echo hello_world")
        assert("hello_world" in out, "exec output should contain hello_world")

        # Pipelines and redirections
        pipeline_out = sh_inst.exec("echo 'foo bar baz' | tr 'a-z' 'A-Z'")
        assert("FOO BAR BAZ" in pipeline_out, "pipeline output should be upper-cased")

        # try_exec success
        res = sh_inst.try_exec("echo try_success")
        assert(res.ok, "try_exec should succeed")
        assert(res.code == 0, "exit code should be 0")
        assert("try_success" in res.stdout, "stdout should contain try_success")

        # try_exec failure
        res_err = sh_inst.try_exec("exit 42")
        assert(not res_err.ok, "try_exec with non-zero exit should not be ok")
        assert(res_err.code == 42, "exit code should be 42")

        # Stderr capture
        res_stderr = sh_inst.try_exec("echo error_output >&2; exit 1")
        assert(not res_stderr.ok, "try_exec with non-zero exit should not be ok")
        assert(res_stderr.code == 1, "exit code should be 1")
        assert("error_output" in res_stderr.stderr, "stderr should capture error_output")

def test_shell_multiline_script():
    """Test multi-line script execution in shell."""
    if runtime.platform() == "windows":
        sh_inst = os.cmdexe()
        script = """
        echo line1
        echo line2
        """
        out = sh_inst.exec(script)
        assert("line1" in out, "output should contain line1")
        assert("line2" in out, "output should contain line2")
    else:
        sh_inst = os.sh()
        script = """
        X="hello"
        Y="world"
        echo "$X $Y"
        """
        out = sh_inst.exec(script)
        assert("hello world" in out, "output should contain hello world")

def test_shell_env_inheritance_and_override():
    """Test bound environment variables and per-call overrides."""
    if runtime.platform() == "windows":
        sh_inst = os.cmdexe(env = {"VAR1": "val1", "VAR2": "val2"})
        out = sh_inst.exec("echo %VAR1% %VAR2%")
        assert("val1 val2" in out, "output should contain val1 val2")

        out_override = sh_inst.exec("echo %VAR1% %VAR2%", env = {"VAR2": "override2"})
        assert("val1 override2" in out_override, "output should contain val1 override2")
    else:
        sh_inst = os.sh(env = {"VAR1": "val1", "VAR2": "val2"})
        out = sh_inst.exec("echo $VAR1 $VAR2")
        assert("val1 val2" in out, "output should contain val1 val2")

        out_override = sh_inst.exec("echo $VAR1 $VAR2", env = {"VAR2": "override2"})
        assert("val1 override2" in out_override, "output should contain val1 override2")

def test_shell_cwd_inheritance_and_override():
    """Test bound cwd and per-call cwd override."""
    target_dir = cwd()
    parent_dir = fs.path(target_dir).parent.string
    if runtime.platform() == "windows":
        sh_inst = os.cmdexe(cwd = target_dir)
        out = sh_inst.exec("cd")
        assert(len(out.strip()) > 0, "cd output should not be empty")
    else:
        sh_inst = os.sh(cwd = target_dir)
        out = sh_inst.exec("pwd")
        assert(out.strip() == target_dir, "pwd output should match bound cwd")

        out_override = sh_inst.exec("pwd", cwd = parent_dir)
        assert(out_override.strip() == parent_dir, "pwd output should match overridden cwd")
