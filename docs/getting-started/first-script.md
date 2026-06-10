---
title: "Run a Starkite script"
description: "Write and run your first Starkite script - hello.star"
weight: 1
---

# Hello.star: your first script

A Starkite script is written as a Starlark file and saved with the `.star` extension. 
As a first script, create a file called `hello.star` as shown below.

```python
# hello.star

print("Hello from starkite!")
```

Next, use the `kite` command to execute the script:

```
$ kite run ./hello.star

Hello from starkite!
```

Starkite script files can also use the hashbang (`#!`) preamble to specify the `kite` command
to run the file as an executable script (on Linux and MacOS).

```python
#!/usr/bin/env kite
# hello.star

print("Hello from starkite!")
```

Next, make the script file executable and run it.

```
$ chmod +x hello.star
$ ./deploy.star

Hello from starkite!
```

Congratulations, you' written and executed your first Starkite script. 

## Starkite comes with everything
Starkite comes pre-loaded with a standard set of library modules which provides
everything you need to create functional and useful scripts. Let's extend the previous
script to print local system information.

```python
#!/usr/bin/env kite
# hello.star

print("Hello from starkite!")

printf("Hostname: %s\n", hostname())
printf("User:     %s\n", username())
printf("Cwd:      %s\n", cwd())

uname = os.exec("uname -s").strip()
printf("Kernel:   %s\n", uname)

printf("Time:     %s\n", time.format(time.now(), time.RFC3339))
printf("Home:     %s\n", env("HOME", "/tmp"))
```

When you run the example, it should print system information
similar to the output below.

```bash
$ kite run ./examples/core/hello.star

Hello from starkite!
Hostname: dev-host.local
User:     alice
Cwd:      /home/alice/projects/starkite
Kernel:   Linux
Time:     2026-01-15T10:00:00Z
Home:     /home/alice
```

*See example* [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star).
