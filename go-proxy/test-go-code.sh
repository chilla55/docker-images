#!/bin/bash
#
# Run comprehensive Go tests for proxy-manager
#

set -e

cd "$(dirname "$0")/proxy-manager"

echo "================================================"
echo "Running Go Unit Tests"
echo "================================================"
echo

# Run tests with coverage
echo "Running tests with coverage..."
go test -v -race -coverprofile=coverage.out -covermode=atomic ./... 2>&1 | tee test-output.txt

echo
echo "================================================"
echo "Generating coverage report..."
echo "================================================"
go tool cover -func=coverage.out

echo
echo "HTML coverage report: proxy-manager/coverage.html"
go tool cover -html=coverage.out -o coverage.html

# Extract coverage percentage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo
echo "================================================"
echo "Total Coverage: $COVERAGE"
echo "================================================"

# Run benchmarks
echo
echo "================================================"
echo "Running benchmarks..."
echo "================================================"
go test -bench=. -benchmem ./... 2>&1 | tee bench-output.txt

echo
echo "✅ All tests complete!"
echo "Test output saved to: proxy-manager/test-output.txt"
echo "Benchmark output saved to: proxy-manager/bench-output.txt"
