#!/bin/bash
# Development script to run worker with hot reload

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "🔧 Starting Worker in DEV MODE with Hot Reload..."
echo ""
echo "This will:"
echo "  ✅ Mount source code as volume"
echo "  ✅ Auto-rebuild on file changes"
echo "  ✅ Use MailHog for SMTP testing"
echo ""

# Check if air is installed locally (optional, for standalone dev)
if command -v air &> /dev/null; then
    echo "ℹ️  'air' found locally. You can also run: cd worker && air"
    echo ""
fi

# Start infrastructure if not running
echo "📦 Checking infrastructure..."
if ! docker compose ps postgres redis mailhog 2>/dev/null | grep -q "Up"; then
    echo "🚀 Starting infrastructure services..."
    docker compose up -d postgres redis mailhog
    echo "⏳ Waiting for services to be healthy..."
    sleep 5
fi

# Start worker with dev override
echo "🚀 Starting worker with hot reload..."
docker compose -f docker-compose.yml -f docker-compose.dev.yml up worker
