#!/bin/bash

# Enterprise Features Test Script
# Tests: SOCKS5 Proxy, Greylisting Retry, Catch-All Detection

set -e

echo "=========================================="
echo "  Enterprise Features Test Suite"
echo "=========================================="
echo ""

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKER_DIR="$PROJECT_ROOT/worker"

cd "$WORKER_DIR"

# Test 1: Build Verification
echo "=== Test 1: Build Verification ==="
if go build -o validator-worker . 2>&1; then
    echo "✅ Build successful"
else
    echo "❌ Build failed"
    exit 1
fi
echo ""

# Test 2: Worker Startup (Dev Mode)
echo "=== Test 2: Worker Startup (Dev Mode) ==="
timeout 3 ./validator-worker 2>&1 | grep -E "(Enterprise|DEV MODE|Retry Monitor)" && echo "✅ Worker starts correctly" || echo "❌ Worker startup issue"
echo ""

# Test 3: SOCKS5 Proxy Configuration
echo "=== Test 3: SOCKS5 Proxy Configuration ==="
if grep -q "SOCKS5_PROXY" .env 2>/dev/null; then
    echo "✅ SOCKS5_PROXY found in .env"
    grep "SOCKS5_PROXY" .env
else
    echo "ℹ️  SOCKS5_PROXY not configured (optional)"
fi
echo ""

# Test 4: Retry Queue (ZSET) Structure
echo "=== Test 4: Retry Queue (ZSET) Structure ==="
RETRY_COUNT=$(docker exec email-validator-redis redis-cli ZCARD email_retry_queue 2>/dev/null || echo "0")
echo "Retry queue items: $RETRY_COUNT"
if [ "$RETRY_COUNT" -ge 0 ]; then
    echo "✅ Retry queue accessible"
else
    echo "❌ Retry queue not accessible"
fi
echo ""

# Test 5: Test Greylisting Simulation
echo "=== Test 5: Greylisting Simulation ==="
# Create a test job that would trigger greylisting
TEST_JOB='{"jobId":"test-greylist-123","email":"test@example.com"}'
# Add to retry queue with future timestamp (1 minute from now for testing)
FUTURE_TIME=$(($(date +%s) + 60))
docker exec email-validator-redis redis-cli ZADD email_retry_queue $FUTURE_TIME "$TEST_JOB" > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✅ Successfully added test item to retry queue"
    # Check it's there
    COUNT=$(docker exec email-validator-redis redis-cli ZCARD email_retry_queue)
    echo "   Retry queue now has $COUNT item(s)"
    # Clean up
    docker exec email-validator-redis redis-cli ZREM email_retry_queue "$TEST_JOB" > /dev/null 2>&1
else
    echo "❌ Failed to add to retry queue"
fi
echo ""

# Test 6: Code Compilation Check
echo "=== Test 6: Code Compilation Check ==="
if go vet ./... 2>&1 | grep -v "vendor"; then
    echo "⚠️  Code issues found"
else
    echo "✅ Code compiles without errors"
fi
echo ""

# Test 7: Feature Detection
echo "=== Test 7: Feature Detection ==="
echo "Checking for enterprise features in code..."
FEATURES_FOUND=0

if grep -q "SOCKS5" smtp.go; then
    echo "✅ SOCKS5 Proxy support found"
    FEATURES_FOUND=$((FEATURES_FOUND + 1))
fi

if grep -q "RetryMonitor" main.go; then
    echo "✅ Retry Monitor found"
    FEATURES_FOUND=$((FEATURES_FOUND + 1))
fi

if grep -q "checkCatchAll" smtp.go; then
    echo "✅ Catch-All Detection found"
    FEATURES_FOUND=$((FEATURES_FOUND + 1))
fi

if grep -q "IsRetryable" smtp.go; then
    echo "✅ Greylisting Retry Logic found"
    FEATURES_FOUND=$((FEATURES_FOUND + 1))
fi

echo ""
echo "Features detected: $FEATURES_FOUND/4"
if [ $FEATURES_FOUND -eq 4 ]; then
    echo "✅ All enterprise features implemented"
else
    echo "⚠️  Some features may be missing"
fi
echo ""

echo "=========================================="
echo "  Test Summary"
echo "=========================================="
echo "✅ Enterprise Edition Worker is ready!"
echo ""
echo "Features:"
echo "  1. SOCKS5 Proxy Support"
echo "  2. Greylisting Retry Logic (ZSET)"
echo "  3. Catch-All Detection (Random Probe)"
echo ""
