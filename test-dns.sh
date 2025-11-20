#!/bin/bash

# Test script for Glory Hole DNS server
# Make sure the server is running on 127.0.0.1:5353

SERVER="127.0.0.1"
PORT="5353"

echo "================================"
echo "Glory Hole DNS Server Test Suite"
echo "================================"
echo ""

# Check if server is running
echo "1. Checking if server is responding..."
if dig @${SERVER} -p ${PORT} +short +time=2 google.com > /dev/null 2>&1; then
    echo "✓ Server is responding"
else
    echo "✗ Server is not responding on ${SERVER}:${PORT}"
    exit 1
fi
echo ""

# Test forwarding
echo "2. Testing DNS forwarding..."
RESULT=$(dig @${SERVER} -p ${PORT} +short google.com A | head -1)
if [ -n "$RESULT" ]; then
    echo "✓ Forwarding works: google.com -> $RESULT"
else
    echo "✗ Forwarding failed"
fi
echo ""

# Test different record types
echo "3. Testing different record types..."
echo "   A record (google.com):"
dig @${SERVER} -p ${PORT} +short google.com A | head -3

echo "   AAAA record (google.com):"
dig @${SERVER} -p ${PORT} +short google.com AAAA | head -1

echo "   MX record (gmail.com):"
dig @${SERVER} -p ${PORT} +short gmail.com MX | head -1
echo ""

# Test multiple queries
echo "4. Testing multiple concurrent queries..."
for domain in google.com github.com cloudflare.com amazon.com; do
    dig @${SERVER} -p ${PORT} +short $domain A > /dev/null 2>&1 &
done
wait
echo "✓ Concurrent queries completed"
echo ""

# Test round-robin
echo "5. Testing round-robin upstream selection..."
for i in {1..5}; do
    dig @${SERVER} -p ${PORT} +short example.com > /dev/null 2>&1
done
echo "✓ Multiple queries sent (check server logs for round-robin)"
echo ""

# Performance test
echo "6. Performance test (10 queries)..."
START=$(date +%s%N)
for i in {1..10}; do
    dig @${SERVER} -p ${PORT} +short google.com > /dev/null 2>&1
done
END=$(date +%s%N)
DURATION=$(( ($END - $START) / 1000000 ))
echo "✓ 10 queries completed in ${DURATION}ms ($(( $DURATION / 10 ))ms average)"
echo ""

echo "================================"
echo "All tests completed!"
echo "================================"
echo ""
echo "Check the server logs for detailed query information."
