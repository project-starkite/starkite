# sandbox_module_test.star - Tests for sandbox module

def test_list_and_default_drivers():
    """Verify list_drivers and default_driver work as expected"""
    if runtime.platform() == "windows":
        skip("sandbox not supported on windows")
    drivers = sandbox.list_drivers()
    assert(len(drivers) > 0, "expected at least one registered driver")
    
    def_driver = sandbox.default_driver()
    assert(len(def_driver) > 0, "expected default driver name")
    assert(def_driver in drivers, "default driver must be in registered drivers list")

def test_sandbox_config():
    """Verify sandbox.config creates a valid Sandbox object with attributes"""
    if runtime.platform() == "windows":
        skip("sandbox not supported on windows")
    box = sandbox.config(
        driver="default",
        memory="256MB",
        network="none",
        timeout="10s"
    )
    assert(box.driver != "", "driver should not be empty")
    assert(box.network == "none", "network should be none")
    assert(box.memory == 256, "memory should be 256MB")

def test_box_exec():
    """Verify box.exec runs a basic command and captures output"""
    if runtime.platform() == "windows":
        skip("sandbox not supported on windows")
    box = sandbox.config(driver="default", network="none")
    res = box.exec("echo", ["hello-starlark-sandbox"])
    assert(res.ok == True, "execution should be ok")
    assert(res.exit_code == 0, "exit code should be 0")
    assert("hello-starlark-sandbox" in res.stdout, "stdout should contain hello-starlark-sandbox")
    assert(res.duration != "", "duration should not be empty")
