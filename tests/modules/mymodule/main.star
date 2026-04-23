# mymodule - A test module for external module loading

def hello(name = "world"):
    """Say hello to someone."""
    return "Hello, " + name + "!"

def add(a, b):
    """Add two numbers."""
    return a + b

def get_config_value(key, default = None):
    """Get a value from module configuration."""
    return _config.get(key, default)

# Private function - should not be accessible from outside
def _internal_helper():
    return "internal"

# Expose the internal result through a public function
def use_internal():
    """Use the internal helper."""
    return _internal_helper()
