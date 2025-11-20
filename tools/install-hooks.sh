#!/bin/bash
# Install git hooks for automatic RAG database updates

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"
POST_COMMIT_HOOK="$HOOKS_DIR/post-commit"

echo "🔧 Installing git hooks..."

# Create hooks directory if it doesn't exist
mkdir -p "$HOOKS_DIR"

# Create post-commit hook for RAG updates
cat > "$POST_COMMIT_HOOK" << 'EOF'
#!/bin/bash
# Post-commit hook: Automatically rebuild RAG database after commits
# This ensures the RAG database stays in sync with code changes

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the repository root
REPO_ROOT="$(git rev-parse --show-toplevel)"
DB_PATH="$REPO_ROOT/codebase_rag.db"
PYTHON="${PYTHON:-python3}"

# Check if RAG system exists
if [ ! -f "$REPO_ROOT/tools/codebase_rag.py" ]; then
    exit 0
fi

# Check if Python is available
if ! command -v $PYTHON &> /dev/null; then
    exit 0
fi

# Get the last commit message to check if it's a RAG-related commit
LAST_COMMIT_MSG=$(git log -1 --pretty=%B)

# Skip if the last commit was already a RAG update (avoid infinite loops)
if echo "$LAST_COMMIT_MSG" | grep -q "chore: Update RAG database"; then
    exit 0
fi

# Store the current RAG database state
if [ -f "$DB_PATH" ]; then
    RAG_OLD_STATE=$(md5sum "$DB_PATH" | awk '{print $1}')
else
    RAG_OLD_STATE=""
fi

# Rebuild RAG database silently
echo -e "${BLUE}🔄 Updating RAG database...${NC}"
$PYTHON "$REPO_ROOT/tools/codebase_rag.py" index > /dev/null 2>&1 || exit 0

# Check if RAG database changed
if [ -f "$DB_PATH" ]; then
    RAG_NEW_STATE=$(md5sum "$DB_PATH" | awk '{print $1}')

    if [ "$RAG_OLD_STATE" != "$RAG_NEW_STATE" ]; then
        # RAG database changed, commit the update
        echo -e "${GREEN}✅ RAG database updated, committing...${NC}"

        git add "$DB_PATH"
        git commit -m "chore: Update RAG database

Automatically updated codebase_rag.db to reflect latest code changes.

🧠 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>" --no-verify || true

        echo -e "${GREEN}✅ RAG database committed${NC}"
    else
        echo -e "${GREEN}✅ RAG database already up-to-date${NC}"
    fi
fi
EOF

# Make the hook executable
chmod +x "$POST_COMMIT_HOOK"

echo "✅ Post-commit hook installed at: $POST_COMMIT_HOOK"
echo ""
echo "ℹ️  How it works:"
echo "   • Runs automatically after each 'git commit'"
echo "   • Rebuilds RAG database if code changed"
echo "   • Creates follow-up commit if RAG database updated"
echo "   • No action needed from you!"
echo ""
echo "💡 To disable temporarily:"
echo "   git commit --no-verify"
echo ""
echo "✨ Git hooks are ready!"
