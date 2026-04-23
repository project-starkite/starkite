#!/usr/bin/env kite
# hello.starctl - Simple hello world example

# Print a greeting
print("Hello from starctl! 🦞")

# Get system information
printf("Hostname: %s\n", hostname())
printf("Username: %s\n", username())
printf("Current directory: %s\n", cwd())

# Execute a local command
output = os.exec("uname -a")
printf("System: %s", output)

# Show current time
now = time.now()
printf("Current time: %s\n", time.format(now, time.RFC3339))

# Environment variable example
home = env("HOME", "/tmp")
printf("HOME directory: %s\n", home)
