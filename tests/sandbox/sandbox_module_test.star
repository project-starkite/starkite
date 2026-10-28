# sandbox_module_test.star - Tests for sandbox module

def test_list_and_default_drivers():
    """Verify list_drivers and default_driver work as expected"""
    drivers = sandbox.list_drivers()
    assert(len(drivers) > 0, "expected at least one registered driver")
    
    def_driver = sandbox.default_driver()
    assert(len(def_driver) > 0, "expected default driver name")
    assert(def_driver in drivers, "default driver must be in registered drivers list")

def test_sandbox_config():
    """Verify sandbox.config creates a valid Sandbox object with attributes"""
    box = sandbox.config(
        driver="default",
        memory="256MB",
        network="none",
        timeout="10s",
        no_exec=True,
        allowed_exec_paths=["/bin/echo", "/usr/bin/git"]
    )
    assert(box.driver != "", "driver should not be empty")
    assert(box.network == "none", "network should be none")
    assert(box.memory == 256, "memory should be 256MB")
    assert(box.no_exec == True, "no_exec should be True")
    assert(len(box.allowed_exec_paths) == 2, "allowed_exec_paths should have 2 entries")

def test_box_exec():
    """Verify box.exec runs a basic command and captures output"""
    if runtime.platform() == "windows":
        skip("sandbox box.exec unsupported on windows")
    box = sandbox.config(driver="default", network="host")
    res = box.exec("echo", ["hello-starlark-sandbox"])
    assert(res.ok == True, "execution should be ok")
    assert(res.exit_code == 0, "exit code should be 0")
    assert("hello-starlark-sandbox" in res.stdout, "stdout should contain hello-starlark-sandbox")
    assert(res.duration != "", "duration should not be empty")

def test_box_exec_network_none():
    """Verify box.exec with network="none" either succeeds in isolated netns or fails closed cleanly"""
    if runtime.platform() == "windows":
        skip("sandbox box.exec unsupported on windows")
    box = sandbox.config(driver="default", network="none")
    res = box.exec("echo", ["isolated"])
    if res.ok:
        assert("isolated" in res.stdout, "stdout should contain isolated")
    else:
        assert("network isolation" in res.error, "failure should report network isolation failure")
