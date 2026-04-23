# extras.star - Additional functions merged into mymodule

def multiply(a, b):
    """Multiply two numbers."""
    return a * b

def _extra_private():
    """Private function from extras - should NOT be merged."""
    return "extra_private"
