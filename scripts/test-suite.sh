#!/bin/bash

# Comprehensive Test Suite for Email Validator
# Usage: ./test-suite.sh

set -e

echo "=========================================="
echo "  Email Validator Test Suite"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
PASSED=0
FAILED=0

# Helper functions
test_pass() {
    echo -e "${GREEN}✅ PASS:${NC} $1"
    ((PASSED++))
}

test_fail() {
    echo -e "${RED}❌ FAIL:${NC} $1"
    ((FAILED++))
}

test_info() {
    echo -e "${YELLOW}ℹ️  INFO:${NC} $1"
}

# Test 1: Infrastructure Health
echo "=== Test 1: Infrastructure Health ==="
if docker compose ps | grep -q "healthy"; then
    test_pass "Docker services are healthy"
else
    test_fail "Docker services not healthy"
    echo "Run: docker compose up -d"
    exit 1
fi
echo ""

# Test 2: API Server
echo "=== Test 2: API Server ==="
if curl -s http://localhost:8080 > /dev/null 2>&1; then
    test_pass "Next.js API server is running"
else
    test_fail "Next.js API server is not running"
    test_info "Start with: cd hub && npm run dev"
    exit 1
fi
echo ""

# Test 3: Redis Connection
echo "=== Test 3: Redis Connection ==="
if docker exec email-validator-redis redis-cli PING 2>/dev/null | grep -q "PONG"; then
    test_pass "Redis is accessible"
else
    test_fail "Redis is not accessible"
    exit 1
fi
echo ""

# Test 4: PostgreSQL Connection
echo "=== Test 4: PostgreSQL Connection ==="
if PGPASSWORD=postgres psql -h localhost -p 5433 -U postgres -d emailvalidator -c "SELECT 1;" > /dev/null 2>&1; then
    test_pass "PostgreSQL is accessible"
else
    test_fail "PostgreSQL is not accessible"
    exit 1
fi
echo ""

# Test 5: Create Job
echo "=== Test 5: Create Validation Job ==="
RESPONSE=$(curl -s -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test1@example.com", "test2@nonexistent-xyz-999.com"]')

if echo "$RESPONSE" | python3 -c "import sys, json; json.load(sys.stdin)" 2>/dev/null; then
    JOB_ID=$(echo "$RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['jobId'])" 2>/dev/null)
    if [ -n "$JOB_ID" ]; then
        test_pass "Job created successfully (ID: ${JOB_ID:0:8}...)"
    else
        test_fail "Job creation response invalid"
        exit 1
    fi
else
    test_fail "Job creation failed"
    echo "Response: $RESPONSE"
    exit 1
fi
echo ""

# Test 6: Verify Queue
echo "=== Test 6: Verify Redis Queue ==="
QUEUE_LEN=$(docker exec email-validator-redis redis-cli LLEN email_queue 2>/dev/null)
if [ "$QUEUE_LEN" -gt 0 ]; then
    test_pass "Emails queued in Redis ($QUEUE_LEN items)"
else
    test_fail "Queue is empty"
fi
echo ""

# Test 7: Worker Processing (if running)
echo "=== Test 7: Worker Status ==="
if pgrep -f validator-worker > /dev/null; then
    test_pass "Worker process is running"
    test_info "Waiting 10 seconds for processing..."
    sleep 10
else
    test_info "Worker not running (start with: cd worker && ./validator-worker)"
fi
echo ""

# Test 8: Check Results
echo "=== Test 8: Check Job Results ==="
if [ -n "$JOB_ID" ]; then
    RESULTS=$(curl -s http://localhost:8080/api/job/$JOB_ID)
    if echo "$RESULTS" | python3 -c "import sys, json; data=json.load(sys.stdin); exit(0 if 'emailChecks' in data else 1)" 2>/dev/null; then
        test_pass "Job results retrieved"
        
        # Check if any emails were processed
        PROCESSED=$(echo "$RESULTS" | python3 -c "import sys, json; data=json.load(sys.stdin); checks=[c for c in data['emailChecks'] if c['status'] != 'PENDING']; print(len(checks))" 2>/dev/null)
        if [ "$PROCESSED" -gt 0 ]; then
            test_pass "$PROCESSED email(s) processed"
        else
            test_info "Emails still pending (worker may need more time)"
        fi
    else
        test_fail "Failed to retrieve job results"
    fi
fi
echo ""

# Test 9: Error Handling
echo "=== Test 9: Error Handling ==="
ERROR_RESPONSE=$(curl -s -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '[]')
if echo "$ERROR_RESPONSE" | python3 -c "import sys, json; data=json.load(sys.stdin); exit(0 if 'error' in data else 1)" 2>/dev/null; then
    test_pass "Error handling works (empty array rejected)"
else
    test_fail "Error handling failed"
fi
echo ""

# Test 10: Non-existent Job
echo "=== Test 10: Non-existent Job ==="
NOT_FOUND=$(curl -s http://localhost:8080/api/job/00000000-0000-0000-0000-000000000000)
if echo "$NOT_FOUND" | python3 -c "import sys, json; data=json.load(sys.stdin); exit(0 if data.get('error') == 'Job not found' else 1)" 2>/dev/null; then
    test_pass "404 handling works"
else
    test_fail "404 handling failed"
fi
echo ""

# Summary
echo "=========================================="
echo "  Test Summary"
echo "=========================================="
echo -e "${GREEN}Passed: $PASSED${NC}"
if [ $FAILED -gt 0 ]; then
    echo -e "${RED}Failed: $FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}Failed: $FAILED${NC}"
    echo ""
    echo -e "${GREEN}✅ All tests passed!${NC}"
    exit 0
fi
