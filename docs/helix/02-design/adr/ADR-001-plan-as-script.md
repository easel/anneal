---
dun:
  id: ADR-001
  depends_on:
    - FEAT-001
    - SD-001
---
# ADR-001: Plan as Executable Script

**Status**: Accepted
**Date**: 2026-04-01

## Context

Anneal needs a plan/apply workflow where the operator previews changes before
execution. The plan format determines both the user experience and the
extensibility model.

Three approaches were considered:

1. **Diff report + direct execution**: engine reads state, shows a colored diff,
   then applies changes via Go function calls. The plan is read-only — what you
   see is a summary of what the engine will do internally.

2. **Executable script with stdlib**: providers emit operations from a standard
   library. The engine assembles them into a shell script. The plan IS the
   execution artifact.

3. **Structured data (JSON/YAML) with executor**: providers emit structured
   change descriptions. A separate executor interprets them.

## Decision

**Approach 2: Plan as executable shell script with stdlib.**

The plan produced by `anneal plan` is a shell script composed of stdlib
operations. It is both human-readable and machine-executable. `anneal apply`
runs the script in an embedded interpreter.

## Consequences

### Positive

- **Transparency**: what you review is what runs. No abstraction layer between
  preview and execution.
- **Auditability**: plans can be saved, version-controlled, diffed against
  previous plans, and sent for review.
- **Extensibility**: custom providers are shell scripts that emit the same stdlib
  operations. The plan format is the integration contract.
- **Debuggability**: a sysadmin can read the plan and understand every operation
  without knowing Anneal internals.

### Negative

- **Stdlib constrains expressiveness**: providers can only emit operations the
  stdlib defines. Mitigated by `stdlib_exec` escape hatch.
- **Binary size**: the embedded interpreter adds ~2-3 MB. Acceptable trade-off.
- **Secret handling complexity**: plans cannot contain secret values. Secrets are
  represented as `anneal_secret` references resolved at apply time. This means
  plans are not fully self-contained — secret providers must be available at
  apply time.
- **Plan comparison**: drift detection requires comparing plan scripts, which
  must be canonicalized to avoid false positives from formatting differences.

### Rejected Alternatives

- **Diff report** (Approach 1): rejected because the plan is not executable,
  cannot be saved and applied later, and custom providers require Go.
- **Structured data** (Approach 3): rejected because plans are not human-readable
  without tooling, losing the "sysadmin can read it" property.
