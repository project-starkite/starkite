#!/usr/bin/env kite
# shell.star - Demonstrates the os.shell constructor and factory shortcuts

def main():
    print("=" * 60)
    print("Starkite Shell Execution Example")
    print("=" * 60)

    is_windows = runtime.platform() == "windows"

    # 1. Instantiate shell runner
    if is_windows:
        sh = os.cmdexe()
        print("\n[Using cmd.exe shell]")
    else:
        sh = os.sh()
        print("\n[Using POSIX /bin/sh shell]")

    print("Shell binary: %s (flag: %s)" % (sh.command, sh.flag))

    # 2. Pipelines and redirections
    print("\n--- 1. Pipelines & Filters ---")
    if is_windows:
        res = sh.exec("dir | findstr /V /C:\"Volume\"")
    else:
        res = sh.exec("echo 'apple\nbanana\ncherry\napricot' | grep '^a' | sort")
    print("Filtered output:\n" + res.strip())

    # 3. Multi-line script execution
    print("\n--- 2. Multi-line Script Execution ---")
    if is_windows:
        ps = os.powershell()
        script = """
        Write-Output "Setting up workspace..."
        $stage = "staging"
        Write-Output "Target stage: $stage"
        Write-Output "Workspace ready."
        """
        script_out = ps.exec(script)
    else:
        script = """
        set -e
        echo "Setting up workspace..."
        STAGE="staging"
        echo "Target stage: $STAGE"
        echo "Workspace ready."
        """
        script_out = sh.exec(script)
    print(script_out.strip())

    # 4. Programmatic error handling with try_exec
    print("\n--- 3. Error Handling with try_exec ---")
    failing_cmd = "exit 42" if is_windows else "echo 'sample error' >&2; exit 42"
    result = sh.try_exec(failing_cmd)
    if result.ok:
        print("Command succeeded unexpectedly.")
    else:
        print("Handled failure cleanly:")
        print("  Exit code: %d" % result.code)
        if result.stderr:
            print("  Captured stderr: %s" % result.stderr.strip())

    # 5. Bound environment variables and working directory
    print("\n--- 4. Bound Context & Per-Call Overrides ---")
    work_dir = cwd()
    custom_shell = os.shell(
        cwd = work_dir,
        env = {
            "APP_ENV": "production",
            "LOG_LEVEL": "info",
        },
        timeout = "30s",
    )
    print("Custom shell configured with cwd: %s" % custom_shell.cwd)

    if is_windows:
        env_check = custom_shell.exec("echo App: %APP_ENV%, Log: %LOG_LEVEL%")
    else:
        env_check = custom_shell.exec("echo App: $APP_ENV, Log: $LOG_LEVEL")
    print("Bound env output: " + env_check.strip())

    # Per-call override
    if is_windows:
        override_check = custom_shell.exec(
            "echo App: %APP_ENV%, Log: %LOG_LEVEL%",
            env = {"LOG_LEVEL": "debug"},
        )
    else:
        override_check = custom_shell.exec(
            "echo App: $APP_ENV, Log: $LOG_LEVEL",
            env = {"LOG_LEVEL": "debug"},
        )
    print("Overridden env output: " + override_check.strip())

    print("\n" + "=" * 60)
    print("Shell execution example completed successfully.")
    print("=" * 60)
