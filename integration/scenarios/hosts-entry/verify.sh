#!/bin/sh
# Verify the hosts entry was added to /etc/hosts.
set -e

if ! grep -q "anneal-test-host" /etc/hosts; then
    echo "FAIL: anneal-test-host not found in /etc/hosts"
    cat /etc/hosts
    exit 1
fi

if ! grep -q "192.168.99.1" /etc/hosts; then
    echo "FAIL: 192.168.99.1 not found in /etc/hosts"
    exit 1
fi

echo "verify: hosts-entry OK"
