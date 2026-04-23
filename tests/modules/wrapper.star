# wrapper - A module that loads another module (dependency chain)

load("./greeter.star", "greeter")

def greet_upper(name):
    """Greet someone in uppercase using the greeter module."""
    return greeter.greet(name).upper()

def double_farewell(name):
    """Say farewell twice."""
    msg = greeter.farewell(name)
    return msg + " " + msg
