#!/bin/sh
# Anneal Docker integration test runner.
# Runs each scenario directory under /opt/anneal-test/scenarios/.
# Each scenario contains a manifest.yaml and a verify.sh script.
#
# Exit codes: 0 = all passed, 1 = one or more failures.

set -e

SCENARIOS_DIR="/opt/anneal-test/scenarios"
PASS=0
FAIL=0
SKIP=0

for scenario_dir in "$SCENARIOS_DIR"/*/; do
    scenario=$(basename "$scenario_dir")
    manifest="$scenario_dir/manifest.yaml"
    verify="$scenario_dir/verify.sh"

    if [ ! -f "$manifest" ]; then
        echo "SKIP  $scenario (no manifest.yaml)"
        SKIP=$((SKIP + 1))
        continue
    fi

    echo "--- $scenario ---"

    # Phase 1: validate
    if ! anneal validate -f "$manifest" 2>&1; then
        echo "FAIL  $scenario (validate failed)"
        FAIL=$((FAIL + 1))
        continue
    fi

    # Phase 2: plan (must produce output)
    plan_output=$(anneal plan -f "$manifest" 2>&1)
    plan_rc=$?
    if [ $plan_rc -ne 0 ]; then
        echo "FAIL  $scenario (plan failed: $plan_output)"
        FAIL=$((FAIL + 1))
        continue
    fi
    echo "$plan_output"

    # Phase 3: apply
    apply_output=$(anneal apply -f "$manifest" 2>&1)
    apply_rc=$?
    if [ $apply_rc -ne 0 ]; then
        echo "FAIL  $scenario (apply failed: $apply_output)"
        FAIL=$((FAIL + 1))
        continue
    fi
    echo "$apply_output"

    # Phase 4: verify (scenario-specific checks)
    if [ -f "$verify" ]; then
        if ! sh "$verify" 2>&1; then
            echo "FAIL  $scenario (verify failed)"
            FAIL=$((FAIL + 1))
            continue
        fi
    fi

    # Phase 5: idempotency — second plan should be empty
    idem_output=$(anneal plan -f "$manifest" 2>&1)
    if echo "$idem_output" | grep -q "^# plan is empty"; then
        echo "OK    $scenario (idempotent)"
    else
        echo "FAIL  $scenario (not idempotent, second plan produced output)"
        echo "$idem_output"
        FAIL=$((FAIL + 1))
        continue
    fi

    PASS=$((PASS + 1))
done

echo ""
echo "=== Results: $PASS passed, $FAIL failed, $SKIP skipped ==="

if [ $FAIL -gt 0 ]; then
    exit 1
fi
exit 0
