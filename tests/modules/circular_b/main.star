# circular_b - Module that loads circular_a (for testing circular dependency detection)

load("../circular_a", "circular_a")

def from_b():
    return "b"
