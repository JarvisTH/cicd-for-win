#!/bin/sh
# Git hooks installer (Linux/macOS)
# Copies hooks from CI/CD hooks/ directory to project's .git/hooks/

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "[install-hooks] Installing Git hooks for $PROJECT_DIR..."

for hook in pre-commit pre-push; do
    if [ -f "$SCRIPT_DIR/hooks/$hook" ]; then
        cp "$SCRIPT_DIR/hooks/$hook" "$PROJECT_DIR/.git/hooks/$hook"
        chmod +x "$PROJECT_DIR/.git/hooks/$hook"
        echo "  ✅ $hook installed"
    else
        echo "  ⚠ $hook not found"
    fi
done

echo "[install-hooks] Done"
