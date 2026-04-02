---
dun:
  id: FEAT-001
  depends_on:
    - anneal.prd
---
# Feature Specification: FEAT-001 - Core Engine

**Feature ID**: FEAT-001
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

The core engine is Anneal's central capability: read a manifest, diff it against
system state via providers, produce an executable plan from standard library
operations, and apply that plan. This feature covers the validate → plan → apply
workflow, the provider contract, the standard library, and the execution model.

## Problem Statement

- **Current situation**: Host configuration is done via imperative shell scripts
  or enterprise CM tools. Neither produces a readable, executable plan artifact.
- **Pain points**: No preview of changes before execution. No standard contract
  for what "configure this resource" means. No way to extend without learning a
  framework or recompiling.
- **Desired outcome**: An operator runs `anneal plan`, reads the output, and
  knows exactly what will happen. The plan is executable. Providers — built-in
  or custom — produce the same kind of output.

## Requirements

### Functional Requirements

- `anneal validate` orchestrates manifest parsing (delegated to FEAT-002's
  config loader), validates resource specs against providers, constructs the
  dependency DAG, and detects cycles — all without system access.
- `anneal plan` invokes each provider's read operation against the live system,
  diffs current state against the manifest, and emits an executable plan
  composed of standard library operations.
- `anneal apply` executes a plan. When given a saved plan file, it re-plans and
  compares the output to the saved plan — executing only if they match.
- The plan is a text artifact: readable by a human, executable by Anneal's
  embedded interpreter.
- Providers implement a read/diff/emit contract. Built-in providers are compiled
  in. Custom providers are shell scripts discovered from the manifest directory.
- Resources can specify a `run_as` user. The provider's operations execute as
  that user instead of root. Required for Homebrew, npm, pipx, and other tools
  that must not run as root.
- The standard library defines the set of operations that appear in plans. Both
  built-in and custom providers emit from the same stdlib.
- Built-in template variables are available in manifests and templates:
  hostname, FQDN, architecture (in Go, Debian, and kernel naming conventions),
  and OS version. These enable URLs and conditionals that adapt to the host.
- Fail-stop: execution halts on the first error. Output identifies the failing
  resource, the operation that failed, and what was already applied.
- The embedded interpreter runs custom providers and executes plans without
  depending on the host's installed shell.

### Non-Functional Requirements

- **Performance**: plan phase completes in seconds for 50+ resources.
- **Security**: secrets never appear in plan output or logs. Providers and plans
  run with Anneal's privileges (typically root).
- **Reliability**: same manifest + same system state always produces the same
  plan (deterministic).
- **Portability**: the embedded interpreter produces identical behavior across
  Linux distributions.

## User Stories

### US-001: Preview changes before applying [FEAT-001]
**As a** server operator
**I want** to see every change Anneal will make before it runs
**So that** I can catch mistakes before they affect a production system

**Acceptance Criteria:**
- [ ] `anneal plan` produces output showing every operation that would run.
- [ ] The plan contains no operations for resources already in desired state.
- [ ] A converged system produces an empty plan.

### US-002: Apply a reviewed plan [FEAT-001]
**As a** server operator
**I want** to save a plan, review it, and apply it later
**So that** I can separate the review step from the execution step

**Acceptance Criteria:**
- [ ] `anneal plan -o plan.sh` writes the plan to a file.
- [ ] `anneal apply plan.sh` re-validates that the system still produces the
  same plan before executing.
- [ ] If the system has drifted, apply aborts with a clear message.

### US-003: Extend with a custom provider [FEAT-001]
**As a** server operator with a custom resource type
**I want** to write a shell script that teaches Anneal about my resource
**So that** I can manage it alongside built-in resources

**Acceptance Criteria:**
- [ ] A shell script implementing read/diff/emit is discovered as a provider.
- [ ] Custom providers appear in the plan identically to built-in providers.
- [ ] `anneal validate` checks custom provider scripts for required functions.

### US-004: Validate without a server [FEAT-001]
**As a** developer writing manifests on a laptop
**I want** to validate my manifest without access to the target host
**So that** I can catch errors before deploying

**Acceptance Criteria:**
- [ ] `anneal validate` succeeds or fails without any network or system access.
- [ ] Validation catches: syntax errors, undefined variables, missing includes,
  unknown resource kinds, dependency cycles.

### US-005: Manage secrets without storing them in config [FEAT-001]
**As a** server operator with credentials in 1Password
**I want** my manifest to reference secrets by name
**So that** passwords and tokens are never stored in config files or plan output

**Acceptance Criteria:**
- [ ] Resources can reference secrets by name in their spec.
- [ ] `anneal plan` renders secret values as `(secret)` in plan output.
- [ ] `anneal apply` resolves secret references at execution time via the
  provider chain (env vars → 1Password).
- [ ] Supports pre-resolved secrets: `anneal resolve-secrets` runs as the
  unprivileged user (who has 1Password access), writes a secrets env file,
  then `anneal apply` (running as root) reads from it. This handles the
  common case where root cannot access the 1Password user session.
- [ ] If a secret is not found and not marked optional, apply fails with a clear
  message naming the missing secret.
- [ ] Auto-generated secrets: if `generate` is set and no provider resolves the
  secret, the generate command runs and the operator is warned to store the
  result.

### US-006: Diagnose failures during apply [FEAT-001]
**As a** server operator
**I want** clear error output when apply fails partway through
**So that** I know what succeeded, what failed, and what was skipped

**Acceptance Criteria:**
- [ ] On failure, output identifies the failing resource and operation.
- [ ] A summary lists resources that were applied, the one that failed, and
  resources that were skipped.
- [ ] The error message includes the system error (e.g., "permission denied",
  "command not found").

## Edge Cases and Error Handling

- A provider's read operation fails (e.g., service not installed): the provider
  returns "absent" state, not an error. The plan shows a create operation.
- A provider's read operation genuinely errors (e.g., permission denied): fail
  the plan for that resource with a clear message.
- Plan file is stale (system drifted since plan was generated): apply aborts,
  shows what changed.
- Custom provider script has syntax errors: `anneal validate` catches this
  before plan.
- Circular dependencies: detected at validate time, fatal error with the cycle
  path printed.

## Success Metrics

- Plan output for the reference deployment passes a readability test with
  operators unfamiliar with Anneal.
- Custom provider end-to-end (write script → validate → plan → apply) under
  1 hour.
- Zero false positives: a converged system always produces an empty plan.

## Constraints and Assumptions

- The embedded interpreter must support enough shell syntax for real-world
  provider scripts (pipes, command substitution, conditionals, functions).
- Plan determinism requires providers to produce consistent output (no
  timestamps, no random ordering in reads).
- The stdlib must be small enough to learn quickly but expressive enough to
  cover the built-in provider set without excessive `stdlib_exec` escaping.

## Dependencies

- Manifest system (FEAT-002): the engine consumes parsed, expanded manifests.
- Embedded shell interpreter: third-party or custom.
- 1Password CLI for secret resolution.

## Out of Scope

- Specific built-in providers (covered by separate features/stories).
- Module registry or sharing mechanism.
- Remote execution.
