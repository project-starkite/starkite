#!/usr/bin/env kite
# autonomous_fix.star — run an agent to completion on a single task.
#
# Pattern: the agent gets a task, calls tools to diagnose + remediate, and
# signals completion by including "DONE" in its final message. ai.run_until
# drives the loop; we just supply a stop predicate and a max-steps safety net.
#
# Usage:  ./kite-ai run autonomous_fix.star
# Prereq: OPENAI_API_KEY, ANTHROPIC_API_KEY, or local Ollama running.

def check_service(name):
    """Ping a service's health endpoint."""
    resp = http.url("http://localhost:8080/health/" + name).get(timeout="5s")
    return {"service": name, "ok": resp.status_code == 200}

def restart_service(name):
    """Restart a service via the local-ops CLI."""
    result = local.exec("systemctl restart " + name)
    return {"restarted": name, "exit_code": result.exit_code}

chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You are an SRE. Diagnose and fix service outages. Say DONE when finished.",
    tools  = [check_service, restart_service],
)

result = ai.run_until(chat,
    "The 'api' service is reportedly down. Diagnose and fix.",
    stop_when = lambda r: "DONE" in r.text,
    max_steps = 15,
)

print(result.text)
print("---")
print("tokens:", result.usage.total)
print("turns:", len(chat.history))
