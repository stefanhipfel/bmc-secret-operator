#!/bin/bash
# Script to verify Prometheus metrics are exposed correctly

set -e

echo "Testing BMC Secret Operator Metrics Endpoint"
echo "============================================="
echo ""

# Check if operator is running
if ! pgrep -f "bin/manager" > /dev/null; then
    echo "⚠️  Operator not running. Start with: make run"
    exit 1
fi

echo "✓ Operator is running"
echo ""

# Test metrics endpoint
echo "Fetching metrics from http://localhost:8080/metrics..."
METRICS=$(curl -s http://localhost:8080/metrics)

if [ $? -ne 0 ]; then
    echo "✗ Failed to fetch metrics endpoint"
    exit 1
fi

echo "✓ Metrics endpoint accessible"
echo ""

# Check for BMC Secret metrics
echo "Checking for BMC Secret Operator metrics..."
echo ""

METRICS_TO_CHECK=(
    "bmcsecret_reconcile_duration_seconds"
    "bmcsecret_reconcile_total"
    "bmcsecret_bmc_count"
    "bmcsecret_sync_success_paths"
    "bmcsecret_sync_failed_paths"
    "bmcsecret_sync_last_success_timestamp"
    "bmcsecret_backend_operation_duration_seconds"
    "bmcsecret_backend_operation_total"
    "bmcsecret_backend_errors_total"
    "bmcsecret_backend_auth_duration_seconds"
    "bmcsecret_backend_auth_total"
    "bmcsecret_bmc_discovery_duration_seconds"
    "bmcsecret_credential_extraction_total"
)

FOUND=0
MISSING=0

for metric in "${METRICS_TO_CHECK[@]}"; do
    if echo "$METRICS" | grep -q "^# HELP $metric"; then
        echo "✓ $metric"
        FOUND=$((FOUND + 1))
    else
        echo "✗ $metric (not found)"
        MISSING=$((MISSING + 1))
    fi
done

echo ""
echo "============================================="
echo "Results: $FOUND/$((FOUND + MISSING)) metrics found"

if [ $MISSING -gt 0 ]; then
    echo "⚠️  Some metrics are missing. This may be expected if no reconciliations have occurred yet."
    echo "   Try creating a BMCSecret resource to trigger metric collection."
else
    echo "✓ All metrics are registered and available"
fi

# Show sample output
echo ""
echo "Sample metrics output:"
echo "====================="
curl -s http://localhost:8080/metrics | grep "^bmcsecret_" | head -20
