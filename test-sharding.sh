#!/bin/bash
# Simple test to demonstrate sharding functionality

BINARY="./bin/cluster-control-plane-machine-set-operator-ext"

echo "=== Sharding Verification ==="
echo ""
echo "1. Listing all tests in cpmso/presubmit suite:"
TOTAL=$($BINARY list tests --suite=cpmso/presubmit 2>/dev/null | jq '. | length')
echo "   Total tests: $TOTAL"
echo ""

echo "2. Testing help output for sharding flags:"
$BINARY run-suite --help | grep -A1 "shard-index"
echo ""

echo "3. Ready to test with sharding:"
echo "   Run shard 1 of 2: $BINARY run-suite cpmso/presubmit --shard-index=1 --shard-total=2"
echo "   Run shard 2 of 2: $BINARY run-suite cpmso/presubmit --shard-index=2 --shard-total=2"
echo ""
echo "Each shard will print: 'Sharding enabled: shard X/2, running Y of $TOTAL tests'"
