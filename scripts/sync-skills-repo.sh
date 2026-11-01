#!/usr/bin/env bash
# scripts/sync-skills-repo.sh: Synchronize generated agent skills to project-starkite/skills.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SKILLS_REPO_DIR="${1:-$REPO_ROOT/../skills}"
KITE_BIN="${2:-$REPO_ROOT/bin/kite}"

if [ ! -x "$KITE_BIN" ]; then
    echo "ERROR: kite binary not found at $KITE_BIN. Building..."
    mkdir -p "$REPO_ROOT/bin"
    (cd "$REPO_ROOT/kite" && go build -ldflags="-s -w" -o "$REPO_ROOT/bin/kite" .)
fi

mkdir -p "$SKILLS_REPO_DIR"

# Scaffold package.json if absent
if [ ! -f "$SKILLS_REPO_DIR/package.json" ]; then
    cat << 'EOF' > "$SKILLS_REPO_DIR/package.json"
{
  "name": "@project-starkite/skills",
  "version": "0.11.3",
  "description": "Official agent skills for Starkite automation DSL, Kubernetes, and cloud workflows",
  "keywords": [
    "starkite",
    "starlark",
    "agent-skills",
    "skills",
    "claude-code",
    "cursor",
    "antigravity",
    "automation",
    "kubernetes"
  ],
  "homepage": "https://starkite.dev",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/project-starkite/skills.git"
  },
  "license": "Apache-2.0",
  "skills": {
    "starkite": "skills/starkite/SKILL.md",
    "starkite-k8s": "skills/starkite-k8s/SKILL.md",
    "starkite-cloud": "skills/starkite-cloud/SKILL.md"
  }
}
EOF
fi

# Scaffold README.md if absent
if [ ! -f "$SKILLS_REPO_DIR/README.md" ]; then
    cat << 'EOF' > "$SKILLS_REPO_DIR/README.md"
# Starkite Agent Skills

Official agent skill definitions for the [Starkite](https://github.com/project-starkite/starkite) automation DSL.

These skills teach autonomous AI coding agents (Claude Code, Cursor, Antigravity, Robo) how to author secure, idiomatic, sandboxed Starkite scripts for infrastructure, operations, and Kubernetes tasks.

---

## Available Skills

| Skill | Directory | Description |
|---|---|---|
| **`starkite`** | `skills/starkite/` | Complete Starkite automation DSL (all built-in standard modules). |
| **`starkite-k8s`** | `skills/starkite-k8s/` | Specialized Kubernetes cluster operations and triage. |
| **`starkite-cloud`** | `skills/starkite-cloud/` | HTTP APIs, cloud services, and concurrent task pipelines. |

---

## Installation

### 1. Universal Skills CLI (`npx skills`)
Install skills directly into your workspace:

```bash
# Add all Starkite skills to your current workspace:
npx skills add project-starkite/skills

# Add only the Kubernetes automation skill:
npx skills add project-starkite/skills --skill starkite-k8s
```

### 2. Claude Code Plugin Marketplace
Add the Starkite skills repository to your Claude Code configuration:

```bash
/plugin marketplace add project-starkite/skills
/plugin install starkite@project-starkite
```

### 3. Cursor & Antigravity IDE
Copy the desired skill directory to `.cursor/skills/` or `.agent/skills/`:

```bash
mkdir -p .agent/skills/starkite
curl -sSL https://raw.githubusercontent.com/project-starkite/skills/main/skills/starkite/SKILL.md \
  -o .agent/skills/starkite/SKILL.md
```

---

## License

Apache-2.0. Copyright (c) 2026 Starkite Authors.
EOF
fi

echo "=== Synchronizing Starkite Agent Skills to $SKILLS_REPO_DIR ==="

mkdir -p "$SKILLS_REPO_DIR/skills/starkite" \
         "$SKILLS_REPO_DIR/skills/starkite-k8s" \
         "$SKILLS_REPO_DIR/skills/starkite-cloud"

echo "Generating starkite/SKILL.md..."
"$KITE_BIN" doc --skills > "$SKILLS_REPO_DIR/skills/starkite/SKILL.md"

echo "Generating starkite-k8s/SKILL.md..."
"$KITE_BIN" doc k8s --skills > "$SKILLS_REPO_DIR/skills/starkite-k8s/SKILL.md"

echo "Generating starkite-cloud/SKILL.md..."
"$KITE_BIN" doc --skills --category=cloud > "$SKILLS_REPO_DIR/skills/starkite-cloud/SKILL.md"

echo "=== Verification ==="
for skill in starkite starkite-k8s starkite-cloud; do
    file="$SKILLS_REPO_DIR/skills/$skill/SKILL.md"
    if [ ! -s "$file" ]; then
        echo "ERROR: Generated skill is empty: $file"
        exit 1
    fi
    echo "  - $skill: $(wc -l < "$file") lines"
done

echo "=== Skills synchronization complete ==="
