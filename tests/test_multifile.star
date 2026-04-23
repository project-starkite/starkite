# Test multi-file module loading from tests directory
load("./modules/mymodule", mymod = "mymodule")

def test_multifile():
    result = mymod.hello()
    assert(result == "Hello, world!", "multi-file module failed")
    print("✓ Criterion 9: Multi-file module loading works")

def test_config():
    result = mymod.get_config_value("nonexistent", "default_value")
    assert(result == "default_value", "config default should work")
    print("✓ Criterion 8: Config passing and _config access works")

def test_merged_symbols():
    result = mymod.multiply(3, 4)
    assert(result == 12, "merged symbols from extras.star should be accessible")
    print("✓ Multi-file module merging works")

test_multifile()
test_config()
test_merged_symbols()
