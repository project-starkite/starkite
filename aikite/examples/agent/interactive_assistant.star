#!/usr/bin/env kite
# interactive_assistant.star — user-in-the-loop chat with tools.
#
# Pattern: a simple REPL built from starbase primitives (io.prompt for input)
# plus ai.chat() for the conversation. The ai module does NOT ship a built-in
# REPL — you compose one yourself with exactly the UX you want.
#
# Usage:  ./kite-ai run interactive_assistant.star
# Exit:   type "exit" or "quit", or send EOF (Ctrl-D).

def read_file(path):
    """Read a text file and return its contents."""
    return fs.read_text(path)

def list_dir(path):
    """List files in a directory."""
    return [e.name for e in fs.ls(path)]

chat = ai.chat(
    model  = "openai/gpt-4o-mini",
    system = "You are a helpful filesystem assistant. Use the tools to answer questions about local files.",
    tools  = [read_file, list_dir],
)

print("Filesystem assistant — type 'exit' to quit.")
for _ in range(1000):  # generous cap; user Ctrl-C to exit in practice
    user_msg = io.prompt("You: ")
    if user_msg == None or user_msg.lower() in ("exit", "quit"):
        break
    if user_msg == "":
        continue

    resp = chat.send(user_msg)
    printf("Agent: %s\n\n", resp.text)
