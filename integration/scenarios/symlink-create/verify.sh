#!/bin/sh
# Verify the symlink was created and points to the right target.
set -e

LINK="/tmp/anneal-test-link"
TARGET="/tmp/anneal-symlink-target"

if [ ! -L "$LINK" ]; then
    echo "FAIL: $LINK does not exist or is not a symlink"
    exit 1
fi

actual_target=$(readlink "$LINK")
if [ "$actual_target" != "$TARGET" ]; then
    echo "FAIL: symlink target mismatch: expected $TARGET, got $actual_target"
    exit 1
fi

echo "verify: symlink-create OK"
