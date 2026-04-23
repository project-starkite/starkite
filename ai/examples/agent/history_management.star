#!/usr/bin/env kite
# history_management.star — keep a long-running chat under a context budget.
#
# Pattern: after every N turns, use a second chat to summarize everything so
# far, then rebuild the working chat with the summary as a seed "system"-style
# message plus the most recent turn. The primitives used:
#
#   chat.history           — read snapshot of what's happened
#   ai.chat(history=...)   — rebuild a new chat with prior context
#   chat.reset()           — alternative: clear in-place
#
# This is a template, not a drop-in: pick the turn threshold and summary prompt
# that match your workload.

MAX_TURNS_BEFORE_SUMMARIZE = 10

def build_chat(seed_history = None):
    return ai.chat(
        model   = "openai/gpt-4o-mini",
        system  = "You answer user questions about their codebase.",
        history = seed_history,
    )

def summarize(history):
    """Use a cheap model to compress prior turns into a single summary."""
    transcript = "\n".join([m["role"] + ": " + m.get("content", "") for m in history])
    resp = ai.generate(
        "Summarize this conversation in 3 bullet points, preserving key facts:\n\n" + transcript,
        model = "openai/gpt-4o-mini",
    )
    return resp.text

chat = build_chat()

questions = [
    "What's the main entry point of a Go program?",
    "How do I run tests in Go?",
    "What's the difference between a slice and an array?",
    # ... imagine many more turns
]

for i, q in enumerate(questions):
    resp = chat.send(q)
    printf("Q%d: %s\n", i+1, q)
    printf("A%d: %s\n\n", i+1, resp.text)

    # Every Nth turn, compress and rebuild.
    if (i + 1) % MAX_TURNS_BEFORE_SUMMARIZE == 0:
        old = chat.history
        summary = summarize(old)
        chat = build_chat(seed_history = [
            {"role": "user", "content": "Here is a summary of our prior conversation:"},
            {"role": "assistant", "content": summary},
        ])
        printf("[compacted %d messages → summary]\n\n", len(old))
