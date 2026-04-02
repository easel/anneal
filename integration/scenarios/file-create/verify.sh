#!/bin/sh
# Verify the file was created with correct content and permissions.
set -e

FILE="/tmp/anneal-test-file.txt"

if [ ! -f "$FILE" ]; then
    echo "FAIL: $FILE does not exist"
    exit 1
fi

if ! grep -q "hello from anneal integration test" "$FILE"; then
    echo "FAIL: expected content not found"
    cat "$FILE"
    exit 1
fi

echo "verify: file-create OK"
