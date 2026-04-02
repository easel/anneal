#!/bin/sh
# Verify the directory was created.
set -e

DIR="/tmp/anneal-test-dir"

if [ ! -d "$DIR" ]; then
    echo "FAIL: $DIR does not exist or is not a directory"
    exit 1
fi

echo "verify: directory-create OK"
