#!/bin/sh
# Verify the command created the flag file.
set -e

FLAG="/tmp/anneal-cmd-flag"

if [ ! -f "$FLAG" ]; then
    echo "FAIL: $FLAG does not exist"
    exit 1
fi

echo "verify: command-creates OK"
