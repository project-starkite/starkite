#!/usr/bin/env kite
# hello.star — exercises print, printf, os.exec, time.now, env.

print("Hello from starkite!")

printf("Hostname: %s\n", hostname())
printf("User:     %s\n", username())
printf("Cwd:      %s\n", cwd())

uname = os.exec("uname -s").strip()
printf("Kernel:   %s\n", uname)

printf("Time:     %s\n", time.format(time.now(), time.RFC3339))
printf("Home:     %s\n", env("HOME", "/tmp"))
