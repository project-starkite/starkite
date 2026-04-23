# counter - A module to test caching
#
# The load_count variable is set at load time. If caching works correctly,
# this file executes only once. Starlark freezes all module globals after
# load, so the value remains 1 across all accesses.

load_count = 1

def get_load_count():
    """Get the load count (should always be 1 if module is cached)."""
    return load_count
