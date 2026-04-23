# builtins_user - A module that uses built-in modules

def encode_json(data):
    """Encode data as JSON using built-in json module."""
    return json.encode(data)

def join_strings(items, sep):
    """Join strings using built-in strings module."""
    return sep.join(items)

def get_platform():
    """Get platform using built-in runtime module."""
    return runtime.platform()
