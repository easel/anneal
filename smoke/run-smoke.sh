#!/bin/sh
# Anneal screencast smoke proof.
#
# Exercises the core operator workflow:
#   1. validate — manifest parses and DAG is valid
#   2. plan     — shows what will change
#   3. apply    — converges system to desired state
#   4. plan     — second run proves idempotency (empty plan)
#
# Usage:
#   ./smoke/run-smoke.sh [anneal-binary] [smoke-root]
#
# Defaults:
#   anneal-binary = ./anneal (or "anneal" if not found)
#   smoke-root    = /tmp/anneal-smoke-$$
#
# Exit codes: 0 = all passed, 1 = failure

set -e

ANNEAL="${1:-./anneal}"
SMOKE_ROOT="${2:-/tmp/anneal-smoke-$$}"
MANIFEST="$(cd "$(dirname "$0")" && pwd)/manifest.yaml"

# Verify anneal binary.
if [ ! -x "$ANNEAL" ] && ! command -v "$ANNEAL" >/dev/null 2>&1; then
    echo "ERROR: anneal binary not found at $ANNEAL"
    exit 1
fi

# Create smoke root directory.
mkdir -p "$SMOKE_ROOT"

# Export the smoke_root variable for template rendering.
export ANNEAL_SMOKE_ROOT="$SMOKE_ROOT"

echo "=== Anneal Smoke Proof ==="
echo "Binary:     $ANNEAL"
echo "Manifest:   $MANIFEST"
echo "Smoke root: $SMOKE_ROOT"
echo ""

# --- Step 1: Validate ---
echo "--- Step 1: validate ---"
$ANNEAL validate -f "$MANIFEST"
echo ""

# --- Step 2: Plan (first run — should produce operations) ---
echo "--- Step 2: plan (initial) ---"
plan_output=$($ANNEAL plan -f "$MANIFEST")
echo "$plan_output"
if echo "$plan_output" | grep -q "^# plan is empty"; then
    echo "FAIL: initial plan should not be empty"
    rm -rf "$SMOKE_ROOT"
    exit 1
fi
echo ""

# --- Step 3: Apply ---
echo "--- Step 3: apply ---"
$ANNEAL apply -f "$MANIFEST"
echo ""

# --- Step 4: Verify state ---
echo "--- Step 4: verify ---"
fail=0

if [ ! -d "$SMOKE_ROOT/data" ]; then
    echo "FAIL: directory $SMOKE_ROOT/data not created"
    fail=1
fi

if [ ! -f "$SMOKE_ROOT/data/config.txt" ]; then
    echo "FAIL: file $SMOKE_ROOT/data/config.txt not created"
    fail=1
elif ! grep -q "setting_a = enabled" "$SMOKE_ROOT/data/config.txt"; then
    echo "FAIL: config.txt content mismatch"
    fail=1
fi

if [ ! -L "$SMOKE_ROOT/config-link" ]; then
    echo "FAIL: symlink $SMOKE_ROOT/config-link not created"
    fail=1
fi

if [ ! -f "$SMOKE_ROOT/data/flag" ]; then
    echo "FAIL: command flag file not created"
    fail=1
fi

if [ $fail -ne 0 ]; then
    echo "FAIL: state verification failed"
    rm -rf "$SMOKE_ROOT"
    exit 1
fi
echo "All resources converged correctly."
echo ""

# --- Step 5: Idempotency (second plan should be empty) ---
echo "--- Step 5: plan (idempotency check) ---"
idem_output=$($ANNEAL plan -f "$MANIFEST")
echo "$idem_output"
if ! echo "$idem_output" | grep -q "^# plan is empty"; then
    echo "FAIL: second plan should be empty (idempotency violation)"
    rm -rf "$SMOKE_ROOT"
    exit 1
fi
echo ""

# --- Cleanup ---
rm -rf "$SMOKE_ROOT"

echo "=== Smoke proof PASSED ==="
echo "Workflow verified: validate → plan → apply → verify → idempotent re-plan"
