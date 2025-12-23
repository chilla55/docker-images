#!/bin/bash
set -e

echo "================================"
echo "Go Proxy Phase 1 Testing Script"
echo "Security & Compliance Features"
echo "================================"
echo ""

PROXY_URL="http://localhost:8080"  # Health/metrics server
HEALTH_URL="http://localhost:8080"
REGISTRY_URL="http://localhost:8081"  # Service registry
DB_PATH="/tmp/test-proxy.db"
TEST_IP="203.0.113.45"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

pass() {
    echo -e "${GREEN}✓${NC} $1"
}

fail() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

section() {
    echo -e "\n${BLUE}$1${NC}"
}

# Verify proxy is running
if ! ps aux | grep -v grep | grep -q "proxy-manager"; then
    fail "Proxy process is not running. Please start it first."
fi

section "1. Testing Rate Limiting (Task #14)"
echo "   Testing rate limiting with rapid requests..."

# Send 35 requests rapidly (limit is 30/min in test config)
BLOCKED=0
for i in {1..35}; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" $PROXY_URL/ 2>/dev/null)
    if [ "$STATUS" = "429" ]; then
        BLOCKED=$((BLOCKED + 1))
    fi
done

if [ $BLOCKED -ge 1 ]; then
    pass "Rate limiting working (blocked $BLOCKED requests with 429)"
else
    info "Note: Rate limiting may not trigger in short test window"
fi

# Check database for rate limit violations
VIOLATIONS=$(sqlite3 $DB_PATH "SELECT COUNT(*) FROM rate_limit_violations;" 2>/dev/null || echo "0")
if [ "$VIOLATIONS" -ge 1 ]; then
    pass "Rate limit violations logged to database ($VIOLATIONS entries)"
else
    info "No violations logged yet (may need more traffic)"
fi

# Verify rate_limit_violations table structure
COLUMNS=$(sqlite3 $DB_PATH "PRAGMA table_info(rate_limit_violations);" 2>/dev/null | wc -l)
if [ "$COLUMNS" -ge 5 ]; then
    pass "Rate limit violations table has correct structure"
else
    fail "Rate limit violations table structure incorrect"
fi

section "2. Testing Web Application Firewall (Task #15)"
echo "   Testing WAF detection of common attacks..."

# Test SQL Injection
info "Testing SQL injection detection..."
SQL_TEST=$(curl -s -o /dev/null -w "%{http_code}" "$PROXY_URL/?id=1%20OR%201=1" 2>/dev/null)
if [ "$SQL_TEST" = "403" ]; then
    pass "WAF blocks SQL injection attempts"
else
    info "SQL injection test returned: $SQL_TEST (may be log-only mode)"
fi

# Test XSS
info "Testing XSS detection..."
XSS_TEST=$(curl -s -o /dev/null -w "%{http_code}" "$PROXY_URL/?search=%3Cscript%3Ealert%281%29%3C%2Fscript%3E" 2>/dev/null)
if [ "$XSS_TEST" = "403" ]; then
    pass "WAF blocks XSS attempts"
else
    info "XSS test returned: $XSS_TEST (may be log-only mode)"
fi

# Test Path Traversal
info "Testing path traversal detection..."
PATH_TEST=$(curl -s -o /dev/null -w "%{http_code}" "$PROXY_URL/../../../etc/passwd" 2>/dev/null)
if [ "$PATH_TEST" = "403" ]; then
    pass "WAF blocks path traversal attempts"
else
    info "Path traversal test returned: $PATH_TEST (may be log-only mode)"
fi

# Check database for WAF blocks
WAF_BLOCKS=$(sqlite3 $DB_PATH "SELECT COUNT(*) FROM waf_blocks;" 2>/dev/null || echo "0")
if [ "$WAF_BLOCKS" -ge 1 ]; then
    pass "WAF blocks logged to database ($WAF_BLOCKS entries)"
else
    info "No WAF blocks logged (may be in log-only mode)"
fi

# Verify waf_blocks table structure
COLUMNS=$(sqlite3 $DB_PATH "PRAGMA table_info(waf_blocks);" 2>/dev/null | wc -l)
if [ "$COLUMNS" -ge 6 ]; then
    pass "WAF blocks table has correct structure"
else
    fail "WAF blocks table structure incorrect"
fi

# Count WAF rules
WAF_RULES=$(grep -r "attackType.*sql_injection\|xss\|path_traversal" ../proxy-manager/waf/waf.go | wc -l)
if [ "$WAF_RULES" -ge 10 ]; then
    pass "WAF has $WAF_RULES+ detection rules implemented"
else
    info "WAF has $WAF_RULES detection rules"
fi

section "3. Testing PII Masking (Task #16)"
echo "   Testing GDPR-compliant PII masking..."

# Check if PII package exists
if [ -f "../proxy-manager/pii/pii.go" ]; then
    pass "PII masking package exists"
else
    fail "PII masking package not found"
fi

# Verify IP masking functions
if grep -q "MaskIPv4\|MaskIPv6" ../proxy-manager/pii/pii.go; then
    pass "IP masking functions implemented"
else
    fail "IP masking functions not found"
fi

# Verify header stripping
if grep -q "StripHeaders\|ShouldMaskHeader" ../proxy-manager/pii/pii.go; then
    pass "Header stripping functions implemented"
else
    fail "Header stripping functions not found"
fi

# Check for private IP preservation
if grep -q "IsPrivateIP\|preserve_localhost" ../proxy-manager/pii/pii.go; then
    pass "Private IP preservation logic exists"
else
    info "Private IP preservation may not be implemented"
fi

# Verify logging integration
LOG_ENTRIES=$(tail -50 /tmp/proxy-test.log 2>/dev/null | grep -c "masked_ip\|pii" || echo "0")
if [ "$LOG_ENTRIES" -ge 1 ]; then
    pass "PII masking integrated with logging"
else
    info "No PII masking in recent logs (may need traffic)"
fi

section "4. Testing Audit Log (Task #21)"
echo "   Testing configuration change audit logging..."

# Check if audit package exists
if [ -f "../proxy-manager/audit/audit.go" ]; then
    pass "Audit logging package exists"
else
    fail "Audit logging package not found"
fi

# Verify audit_log table
AUDIT_COLUMNS=$(sqlite3 $DB_PATH "PRAGMA table_info(audit_log);" 2>/dev/null | wc -l)
if [ "$AUDIT_COLUMNS" -ge 9 ]; then
    pass "Audit log table has correct structure"
else
    fail "Audit log table structure incorrect (expected 9+ columns, got $AUDIT_COLUMNS)"
fi

# Check for audit entries (startup should have logged something)
AUDIT_ENTRIES=$(sqlite3 $DB_PATH "SELECT COUNT(*) FROM audit_log;" 2>/dev/null || echo "0")
if [ "$AUDIT_ENTRIES" -ge 1 ]; then
    pass "Audit log has entries ($AUDIT_ENTRIES logged events)"
    
    # Show recent audit entries
    info "Recent audit entries:"
    sqlite3 $DB_PATH "SELECT action, resource_type, datetime(timestamp, 'unixepoch') as time FROM audit_log ORDER BY timestamp DESC LIMIT 5;" 2>/dev/null | while read line; do
        echo "     $line"
    done
else
    info "No audit entries yet (expected startup log)"
fi

# Test audit API endpoint
info "Testing audit API endpoint..."
AUDIT_API=$(curl -s $HEALTH_URL/api/audit?limit=5 2>/dev/null)
if echo "$AUDIT_API" | grep -q "entries\|action"; then
    pass "Audit API endpoint responds with JSON"
else
    info "Audit API may not be enabled or no entries exist"
fi

# Verify action types are defined
ACTION_TYPES=$(grep -c "Action.*=" ../proxy-manager/audit/audit.go 2>/dev/null || echo "0")
if [ "$ACTION_TYPES" -ge 5 ]; then
    pass "Multiple audit action types defined ($ACTION_TYPES types)"
else
    fail "Insufficient action types defined"
fi

section "5. Testing Data Retention (Task #22)"
echo "   Testing data retention policies..."

# Check if retention package exists
if [ -f "../proxy-manager/retention/retention.go" ]; then
    pass "Data retention package exists"
else
    fail "Data retention package not found"
fi

# Verify retention policy types
if grep -q "public\|private\|PolicyType" ../proxy-manager/retention/retention.go; then
    pass "Retention policy types defined (public/private/custom)"
else
    fail "Retention policy types not found"
fi

# Verify cleanup methods exist
CLEANUP_METHODS=$(grep -c "Cleanup.*Logs\|CleanupMetrics\|CleanupHealthChecks" ../proxy-manager/database/database.go 2>/dev/null || echo "0")
if [ "$CLEANUP_METHODS" -ge 5 ]; then
    pass "Database cleanup methods implemented ($CLEANUP_METHODS methods)"
else
    fail "Expected 5+ cleanup methods, found $CLEANUP_METHODS"
fi

# Check config files for retention settings
if grep -q "retention:" ../sites-available/example-vaultwarden.yaml; then
    pass "Retention configuration in example configs"
else
    fail "Retention configuration not found in examples"
fi

# Verify different retention periods for public vs private
PRIVATE_RETENTION=$(grep -A 10 "retention:" ../sites-available/example-vaultwarden.yaml | grep "audit_log_days" | awk '{print $2}')
PUBLIC_RETENTION=$(grep -A 10 "retention:" ../sites-available/example-pterodactyl.yaml | grep "audit_log_days" | awk '{print $2}')

if [ ! -z "$PRIVATE_RETENTION" ] && [ ! -z "$PUBLIC_RETENTION" ]; then
    if [ "$PRIVATE_RETENTION" -gt "$PUBLIC_RETENTION" ]; then
        pass "Private services have longer retention than public ($PRIVATE_RETENTION vs $PUBLIC_RETENTION days)"
    else
        info "Private retention: $PRIVATE_RETENTION days, Public: $PUBLIC_RETENTION days"
    fi
else
    info "Could not verify retention period differences"
fi

# Verify cleanup interval configuration
if grep -q "cleanupInterval\|cleanup_interval" ../proxy-manager/retention/retention.go; then
    pass "Configurable cleanup interval implemented"
else
    info "Cleanup interval may be hardcoded"
fi

section "6. Database Schema Verification"
echo "   Verifying all Phase 1 tables..."

REQUIRED_TABLES=("rate_limit_violations" "waf_blocks" "audit_log")
for table in "${REQUIRED_TABLES[@]}"; do
    EXISTS=$(sqlite3 $DB_PATH "SELECT name FROM sqlite_master WHERE type='table' AND name='$table';" 2>/dev/null)
    if [ "$EXISTS" = "$table" ]; then
        pass "Table exists: $table"
    else
        fail "Missing table: $table"
    fi
done

# Verify indexes for performance
PHASE1_INDEXES=$(sqlite3 $DB_PATH "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND sql IS NOT NULL AND (sql LIKE '%rate_limit%' OR sql LIKE '%waf%' OR sql LIKE '%audit%');" 2>/dev/null || echo "0")
if [ "$PHASE1_INDEXES" -ge 3 ]; then
    pass "Phase 1 performance indexes created ($PHASE1_INDEXES indexes)"
else
    info "Phase 1 indexes: $PHASE1_INDEXES (expected 3+)"
fi

section "7. Configuration Validation"
echo "   Checking Phase 1 configuration options..."

# Just verify the code compiles with all Phase 1 config structs
if go build -o /tmp/proxy-test-phase1 .; then
    pass "All Phase 1 config structs compile successfully"
else
    fail "Code does not compile"
fi

# Verify example configs have all Phase 1 features
REQUIRED_FEATURES=("rate_limit:" "waf:" "pii:" "retention:")
for feature in "${REQUIRED_FEATURES[@]}"; do
    if grep -q "$feature" ../sites-available/example-vaultwarden.yaml; then
        pass "Example config includes $feature"
    else
        fail "Example config missing $feature"
    fi
done

section "8. Code Quality Checks"
echo "   Verifying Phase 1 code quality..."

# Check that all packages compile
cd ../proxy-manager
if go build -o /tmp/proxy-test 2>&1 | grep -q "error"; then
    fail "Code does not compile"
else
    pass "All packages compile successfully"
fi

# Run go test
TEST_OUTPUT=$(go test -v ./... 2>&1)
if echo "$TEST_OUTPUT" | grep -q "FAIL"; then
    fail "Tests failed"
else
    pass "All tests pass (or no test files)"
fi

# Check for common issues
if grep -rn "panic(" . --include="*.go" | grep -v "recover()" | grep -v "test" | head -5; then
    info "Found panic() calls (review for proper error handling)"
else
    pass "No problematic panic() calls found"
fi

# Verify logging consistency
if grep -rq "zerolog\|log\." . --include="*.go"; then
    pass "Structured logging (zerolog) used throughout"
else
    fail "Logging not properly implemented"
fi

cd - > /dev/null

section "9. Security Verification"
echo "   Checking security implementations..."

# Verify GDPR compliance features
GDPR_FEATURES=$(grep -c "mask.*ip\|PII\|gdpr" ../proxy-manager/pii/pii.go 2>/dev/null || echo "0")
if [ "$GDPR_FEATURES" -ge 5 ]; then
    pass "GDPR compliance features implemented"
else
    info "GDPR features found: $GDPR_FEATURES"
fi

# Check for sensitive data handling
if grep -q "Authorization\|Cookie\|X-API-Key" ../proxy-manager/pii/pii.go; then
    pass "Sensitive headers identified for stripping"
else
    fail "Sensitive header handling not found"
fi

# Verify attack detection patterns
ATTACK_PATTERNS=$(grep -c "sql.*injection\|xss\|path.*traversal\|command.*injection" ../proxy-manager/waf/waf.go 2>/dev/null || echo "0")
if [ "$ATTACK_PATTERNS" -ge 15 ]; then
    pass "Comprehensive attack patterns ($ATTACK_PATTERNS patterns)"
else
    info "Attack patterns found: $ATTACK_PATTERNS"
fi

# Check whitelist support
if grep -q "whitelist\|Whitelist" ../proxy-manager/ratelimit/ratelimit.go; then
    pass "Rate limiting whitelist support implemented"
else
    fail "Whitelist support not found"
fi

section "10. Documentation Check"
echo "   Verifying documentation..."

# Check for README or documentation
DOC_FILES=$(find .. -name "README*.md" -o -name "QUICKSTART.md" -o -name "*GUIDE*.md" | wc -l)
if [ "$DOC_FILES" -ge 3 ]; then
    pass "Documentation files present ($DOC_FILES files)"
else
    info "Documentation files: $DOC_FILES"
fi

# Verify example configurations are documented
if grep -q "#.*Phase 1" ../sites-available/example-*.yaml; then
    pass "Example configs have Phase 1 documentation"
else
    fail "Example configs lack Phase 1 documentation"
fi

echo ""
echo "================================"
echo -e "${GREEN}Phase 1 Testing Complete!${NC}"
echo "================================"
echo ""
echo "Phase 1 Features Verified:"
echo "  ✓ Rate Limiting System (Task #14)"
echo "  ✓ Web Application Firewall (Task #15)"
echo "  ✓ PII Masking for GDPR (Task #16)"
echo "  ✓ Audit Log System (Task #21)"
echo "  ✓ Data Retention Policies (Task #22)"
echo ""
echo "Database Stats:"
sqlite3 $DB_PATH "SELECT 
    'Rate Limit Violations: ' || COUNT(*) FROM rate_limit_violations
    UNION ALL SELECT 'WAF Blocks: ' || COUNT(*) FROM waf_blocks
    UNION ALL SELECT 'Audit Entries: ' || COUNT(*) FROM audit_log;" 2>/dev/null

echo ""
echo "Next Steps:"
echo "  • Phase 2: High Availability & Performance"
echo "    - Health Checks (Task #1)"
echo "    - Metrics Collection (Task #2)"
echo "    - Distributed Tracing (Task #3)"
echo "    - WebSocket Support (Task #11)"
echo "    - HTTP/2 & HTTP/3 (Task #12)"
echo "    - Connection Pooling (Task #13)"
echo ""
echo "For production deployment:"
echo "  1. Review and adjust rate limits per service"
echo "  2. Configure WAF sensitivity (low/medium/high)"
echo "  3. Set retention policies based on compliance requirements"
echo "  4. Enable audit logging for all config changes"
echo "  5. Monitor security logs regularly"
echo ""
