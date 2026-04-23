#!/usr/bin/env kite
# Module showcase example for starkite

def main():
    # Global aliases (from os and fmt modules)
    printf("Hello from starkite!\n")
    printf("Current directory: %s\n", cwd())
    printf("Hostname: %s\n", hostname())
    printf("Username: %s\n", username())

    # Environment
    printf("\nEnvironment variable HOME: %s\n", env("HOME", "not set"))

    # String operations
    printf("'hello world'.upper(): %s\n", "hello world".upper())
    printf("'a,b,c'.split(','): %s\n", "a,b,c".split(","))

    # JSON module
    data = {"name": "starkite", "version": "0.1.0", "awesome": True}
    json_str = json.encode(data)
    printf("\njson.encode: %s\n", json_str)
    printf("json.decode: %s\n", json.decode(json_str))

    # Path object
    p = path("/home/user/scripts/test.star")
    printf("\npath: %s\n", p.string)
    printf("parent: %s\n", p.parent.string)
    printf("name: %s\n", p.name)
    printf("suffix: %s\n", p.suffix)
    printf("stem: %s\n", p.stem)

    # Path separator
    joined = path("/home") / "user" / "scripts"
    printf("joined: %s\n", joined.string)

    # Time module
    now = time.now()
    printf("\ntime.now: %s\n", now)

    # Hash module (builder pattern)
    hash_result = hash.text("hello world").sha256()
    printf("\nhash.sha256('hello world'): %s\n", hash_result)

    # Base64 module (builder pattern)
    encoded = base64.text("hello world").encode()
    printf("\nbase64.encode: %s\n", encoded)

    # UUID module
    id = uuid.v4()
    printf("\nuuid.v4: %s\n", id)

    # File operations (global aliases)
    if exists("/etc/hosts"):
        content = read_text("/etc/hosts")
        lines = content.split("\n")
        printf("\n/etc/hosts has %d lines\n", len(lines))

    printf("\nAll modules working!\n")

main()
