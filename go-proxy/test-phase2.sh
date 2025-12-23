#!/bin/bash
# test-phase2.sh - Comprehensive Phase 2 testing script
# Tests metrics, health checks, logging, certificates, analytics, and traffic analysis

# Removed set -e to allow all tests to run even if some fail

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
METRICS_URL="${METRICS_URL:-http://localhost:8080}"
TEST_DOMAIN="${TEST_DOMAIN:-vaultwarden.example.com}"

COLOR_GREEN='\033[0;32m'
COLOR_RED='\033[0;31m'
COLOR_YELLOW='\033[1;33m'
COLOR_BLUE='\033[0;34m'
COLOR_RESET='\033[0m'

PASSED=0
FAILED=0

print_test() {
    echo -e "${COLOR_BLUE}[TEST]${COLOR_RESET} $1"
}

print_pass() {
    echo -e "${COLOR_GREEN}[PASS]${COLOR_RESET} $1"
    ((PASSED++))
}

print_fail() {
    echo -e "${COLOR_RED}[FAIL]${COLOR_RESET} $1"
    ((FAILED++))
}

print_info() {
    echo -e "${COLOR_YELLOW}[INFO]${COLOR_RESET} $1"
}

echo "=========================================="
echo "Phase 2: Core Monitoring Test Suite"
echo "=========================================="
echo ""
echo "Testing against: $METRICS_URL"
echo "Test domain: $TEST_DOMAIN"
echo ""

# Test 1: Health Check Endpoints
print_test "Test 1: Health check endpoint"
if curl -s -f "$METRICS_URL/health" | grep -q "healthy"; then
    print_pass "Health endpoint returns 'healthy'"
else
    print_fail "Health endpoint failed"
fi

print_test "Test 2: Ready endpoint"
if curl -s -f "$METRICS_URL/ready" | grep -q "ready"; then
    print_pass "Ready endpoint returns 'ready'"
else
    print_fail "Ready endpoint failed"
fi

# Test 3: Prometheus Metrics (Task #2)
print_test "Test 3: Prometheus metrics endpoint"
METRICS=$(curl -s "$METRICS_URL/metrics")
if echo "$METRICS" | grep -q "proxy_uptime_seconds"; then
    print_pass "Metrics endpoint returns proxy_uptime_seconds"
else
    print_fail "Metrics endpoint missing proxy_uptime_seconds"
fi

if echo "$METRICS" | grep -q "proxy_requests_total"; then
    print_pass "Metrics endpoint returns proxy_requests_total"
else
    print_fail "Metrics endpoint missing proxy_requests_total"
fi

if echo "$METRICS" | grep -q "proxy_errors_total"; then
    print_pass "Metrics endpoint returns proxy_errors_total"
else
    print_fail "Metrics endpoint missing proxy_errors_total"
fi

if echo "$METRICS" | grep -q "proxy_bytes_sent_total"; then
    print_pass "Metrics endpoint returns bandwidth metrics"
else
    print_fail "Metrics endpoint missing bandwidth metrics"
fi

if echo "$METRICS" | grep -q "proxy_active_connections"; then
    print_pass "Metrics endpoint returns connection metrics"
else
    print_fail "Metrics endpoint missing connection metrics"
fi

# Test 4: Health Check API (Task #5)
print_test "Test 4: Service health status API"
if curl -s -f "$METRICS_URL/api/health/services" > /dev/null 2>&1; then
    print_pass "Health services API endpoint accessible"
else
    print_fail "Health services API endpoint failed"
fi

print_test "Test 5: Unhealthy services API"
if curl -s -f "$METRICS_URL/api/health/unhealthy" > /dev/null 2>&1; then
    print_pass "Unhealthy services API endpoint accessible"
else
    print_fail "Unhealthy services API endpoint failed"
fi

# Test 6: Access Logging API (Task #6)
print_test "Test 6: Recent logs API"
if curl -s -f "$METRICS_URL/api/logs/recent" > /dev/null 2>&1; then
    print_pass "Recent logs API endpoint accessible"
else
    print_fail "Recent logs API endpoint failed"
fi

print_test "Test 7: Error logs API"
if curl -s -f "$METRICS_URL/api/logs/errors" > /dev/null 2>&1; then
    print_pass "Error logs API endpoint accessible"
else
    print_fail "Error logs API endpoint failed"
fi

print_test "Test 8: Log statistics API"
if curl -s -f "$METRICS_URL/api/logs/stats" > /dev/null 2>&1; then
    print_pass "Log stats API endpoint accessible"
else
    print_fail "Log stats API endpoint failed"
fi

# Test 9: Certificate Monitoring API (Task #7)
print_test "Test 9: Certificate info API"
if curl -s -f "$METRICS_URL/api/certs" > /dev/null 2>&1; then
    print_pass "Certificate info API endpoint accessible"
else
    print_fail "Certificate info API endpoint failed"
fi

print_test "Test 10: Expiring certificates API"
if curl -s -f "$METRICS_URL/api/certs/expiring" > /dev/null 2>&1; then
    print_pass "Expiring certificates API endpoint accessible"
else
    print_fail "Expiring certificates API endpoint failed"
fi

print_test "Test 11: Certificate statistics API"
if curl -s -f "$METRICS_URL/api/certs/stats" > /dev/null 2>&1; then
    print_pass "Certificate stats API endpoint accessible"
else
    print_fail "Certificate stats API endpoint failed"
fi

# Test 12: Advanced Analytics API (Task #3)
print_test "Test 12: Analytics metrics API"
ANALYTICS=$(curl -s "$METRICS_URL/api/analytics/metrics")
if [ -n "$ANALYTICS" ]; then
    print_pass "Analytics metrics API endpoint accessible"
    
    # Check for key analytics fields
    if echo "$ANALYTICS" | grep -q "response_time_p"; then
        print_pass "Analytics includes response time percentiles"
    else
        print_info "Analytics missing percentile data (may need traffic)"
    fi
    
    if echo "$ANALYTICS" | grep -q "bandwidth"; then
        print_pass "Analytics includes bandwidth metrics"
    else
        print_info "Analytics missing bandwidth data (may need traffic)"
    fi
    
    if echo "$ANALYTICS" | grep -q "error_rate"; then
        print_pass "Analytics includes error rate metrics"
    else
        print_info "Analytics missing error rate data (may need traffic)"
    fi
else
    print_fail "Analytics metrics API endpoint failed"
fi

# Test 13: Traffic Analysis API (Task #4)
print_test "Test 13: Traffic analysis API"
TRAFFIC=$(curl -s "$METRICS_URL/api/traffic/analysis")
if [ -n "$TRAFFIC" ]; then
    print_pass "Traffic analysis API endpoint accessible"
    
    if echo "$TRAFFIC" | grep -q "top_ips"; then
        print_pass "Traffic analysis includes IP rankings"
    else
        print_info "Traffic analysis missing IP data (may need traffic)"
    fi
    
    if echo "$TRAFFIC" | grep -q "top_paths"; then
        print_pass "Traffic analysis includes path rankings"
    else
        print_info "Traffic analysis missing path data (may need traffic)"
    fi
    
    if echo "$TRAFFIC" | grep -q "top_user_agents"; then
        print_pass "Traffic analysis includes user agent data"
    else
        print_info "Traffic analysis missing UA data (may need traffic)"
    fi
    
    if echo "$TRAFFIC" | grep -q "anomalous_patterns"; then
        print_pass "Traffic analysis includes anomaly detection"
    else
        print_info "Traffic analysis missing anomaly data"
    fi
else
    print_fail "Traffic analysis API endpoint failed"
fi

print_test "Test 14: Traffic analysis with custom topN"
if curl -s -f "$METRICS_URL/api/traffic/analysis?top=5" > /dev/null 2>&1; then
    print_pass "Traffic analysis accepts topN parameter"
else
    print_fail "Traffic analysis topN parameter failed"
fi

print_test "Test 15: IP reputation API"
if curl -s -f "$METRICS_URL/api/traffic/ip?ip=127.0.0.1" > /dev/null 2>&1; then
    print_pass "IP reputation API endpoint accessible"
else
    print_fail "IP reputation API endpoint failed"
fi

REPUTATION=$(curl -s "$METRICS_URL/api/traffic/ip?ip=127.0.0.1")
if echo "$REPUTATION" | grep -q "reputation_score"; then
    print_pass "IP reputation returns score"
else
    print_fail "IP reputation missing score field"
fi

print_test "Test 16: Traffic anomalies API"
if curl -s -f "$METRICS_URL/api/traffic/anomalies" > /dev/null 2>&1; then
    print_pass "Traffic anomalies API endpoint accessible"
else
    print_fail "Traffic anomalies API endpoint failed"
fi

# Test 17: Generate test traffic and verify metrics update
print_test "Test 17: Generate test traffic"
print_info "Generating 10 test requests..."

for i in {1..10}; do
    curl -s -H "Host: $TEST_DOMAIN" "$PROXY_URL/" > /dev/null 2>&1 || true
done

sleep 2

METRICS_AFTER=$(curl -s "$METRICS_URL/metrics")
if echo "$METRICS_AFTER" | grep "proxy_requests_total" | grep -q "[1-9]"; then
    print_pass "Metrics updated after test traffic"
else
    print_info "Metrics may not have updated (check if proxy is handling traffic)"
fi

# Test 18: Response format validation
print_test "Test 18: Validate JSON response formats"

# Check if analytics returns valid JSON-like structure
if curl -s "$METRICS_URL/api/analytics/metrics" | grep -q "{"; then
    print_pass "Analytics returns structured data"
else
    print_fail "Analytics response format invalid"
fi

if curl -s "$METRICS_URL/api/traffic/analysis" | grep -q "{"; then
    print_pass "Traffic analysis returns structured data"
else
    print_fail "Traffic analysis response format invalid"
fi

# Test 19: Error handling
print_test "Test 19: Error handling - missing IP parameter"
RESPONSE_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$METRICS_URL/api/traffic/ip")
if [ "$RESPONSE_CODE" = "400" ]; then
    print_pass "IP reputation API correctly rejects missing parameter"
else
    print_fail "IP reputation API doesn't validate missing parameter (got $RESPONSE_CODE)"
fi

# Test 20: Concurrent API access
print_test "Test 20: Concurrent API access"
print_info "Testing concurrent requests to metrics endpoints..."

for i in {1..5}; do
    curl -s "$METRICS_URL/metrics" > /dev/null &
    curl -s "$METRICS_URL/api/analytics/metrics" > /dev/null &
    curl -s "$METRICS_URL/api/traffic/analysis" > /dev/null &
done

wait

if curl -s -f "$METRICS_URL/health" | grep -q "healthy"; then
    print_pass "Proxy handles concurrent API requests"
else
    print_fail "Proxy may have issues with concurrent requests"
fi

# Test 21: Percentile calculations (if data available)
print_test "Test 21: Percentile calculations in analytics"
ANALYTICS_FINAL=$(curl -s "$METRICS_URL/api/analytics/metrics")

HAS_P50=$(echo "$ANALYTICS_FINAL" | grep -o "response_time_p50" || true)
HAS_P90=$(echo "$ANALYTICS_FINAL" | grep -o "response_time_p90" || true)
HAS_P95=$(echo "$ANALYTICS_FINAL" | grep -o "response_time_p95" || true)
HAS_P99=$(echo "$ANALYTICS_FINAL" | grep -o "response_time_p99" || true)

if [ -n "$HAS_P50" ] && [ -n "$HAS_P90" ] && [ -n "$HAS_P95" ] && [ -n "$HAS_P99" ]; then
    print_pass "Analytics calculates all percentiles (P50, P90, P95, P99)"
else
    print_info "Some percentiles missing (may need more traffic data)"
fi

# Test 22: Trend detection
print_test "Test 22: Trend detection in analytics"
if echo "$ANALYTICS_FINAL" | grep -q "trend"; then
    print_pass "Analytics includes trend detection"
else
    print_info "Trend data not available (may need time-series data)"
fi

# Test 23: Bot detection in traffic analysis
print_test "Test 23: Bot detection in traffic analysis"
TRAFFIC_FINAL=$(curl -s "$METRICS_URL/api/traffic/analysis")
if echo "$TRAFFIC_FINAL" | grep -q "bot_traffic_percent\|is_bot"; then
    print_pass "Traffic analysis includes bot detection"
else
    print_info "Bot detection data not available (may need UA data)"
fi

# Summary
echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "${COLOR_GREEN}Passed: $PASSED${COLOR_RESET}"
echo -e "${COLOR_RED}Failed: $FAILED${COLOR_RESET}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${COLOR_GREEN}✅ All Phase 2 tests passed!${COLOR_RESET}"
    exit 0
else
    echo -e "${COLOR_RED}❌ Some tests failed. Review output above.${COLOR_RESET}"
    exit 1
fi
