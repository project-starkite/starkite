# circular_a - Module that loads circular_b (for testing circular dependency detection)

load("./circular_b.star", "circular_b")

def from_a():
    return "a"
