---
dun:
  id: anneal.test-plan
  depends_on:
    - anneal.prd
    - SD-001
---
# Test Plan

## Testing Strategy

Anneal uses a five-tier testing strategy that provides confidence without
requiring real infrastructure for most tests, while still proving the operator
experience end to end before release.

### Tier 1: Unit Tests

**Scope**: individual components in isolation.
**Environment**: developer laptop, no root, no network.
**Framework**: Go standard `testing` package.

Coverage:
- **Config loader**: YAML parsing, variable resolution, include graph, template
  evaluation, iterator expansion, composite expansion.
- **Configuration matrix**: table-driven combinations of manifests, includes,
  variable precedence, optional secrets, run-as-user settings, and fact-driven
  template branches.
- **DAG**: topological sort correctness, cycle detection, tiebreaker ordering,
  diamond dependencies.
- **Provider contract**: each built-in provider's `read`, `diff`, and `emit`
  methods tested against a `MockSystem` interface. No real apt/zfs/docker calls.
- **Provider matrix**: each provider exercised against representative present /
  absent / drifted / partially-configured states and error cases.
- **Stdlib**: each stdlib operation tested for correct command generation.
- **Secret chain**: env var provider, 1Password provider (mocked CLI), chain
  ordering, missing secrets, optional secrets, auto-generation.
- **Interpreter**: mvdan/sh conformance for shell features used by providers
  (pipes, command substitution, conditionals, functions, arrays).

### Tier 2: Golden-File Plan Tests

**Scope**: end-to-end plan generation.
**Environment**: developer laptop, no root, no network.
**Approach**: known manifest + mock system state → expected plan output.

For each golden-file test:
1. A manifest declares resources.
2. A mock system state describes what exists (packages installed, files present,
   users created, etc.).
3. `anneal plan` runs against the mock state.
4. The plan output is compared byte-for-byte against a checked-in expected file.

Golden files catch regressions in:
- Plan formatting and ordering.
- Stdlib operation selection.
- Diff computation.
- Secret placeholder rendering.
- Trigger/notify behavior (which triggers fire, which don't).
- Iterator expansion.
- Composite resource expansion.

### Tier 3: Provider Integration Tests

**Scope**: built-in providers against real system calls.
**Environment**: Docker-based Linux distribution matrix, runs as root inside
test containers.
**Approach**: each provider is tested end-to-end: read → diff → emit → execute
across representative OS and configuration combinations.

Test container setup:
- Debian / Ubuntu family containers for apt behavior.
- Fedora / RHEL-like containers for dnf behavior.
- Arch Linux containers for pacman behavior.
- A Linux container with Homebrew for brew-specific provider coverage where
  feasible.
- No pre-installed optional packages unless the scenario requires them.
- Writable filesystem (test file providers).
- Fake systemd (systemd-container or mock).
- Scenario fixtures covering multiple manifest combinations, not just one happy
  path per provider.
- No ZFS, Docker, or Kerberos in the generic matrix (those remain tier 4 or
  specialized harnesses).

Coverage:
- `apt_packages`: install, already-installed, purge.
- `dnf_packages` and `pacman_packages`: install, already-installed, purge /
  remove semantics.
- `brew_packages`, `brew_tap`, `npm_packages`, `python_packages`: unprivileged
  execution, already-installed behavior, and package drift.
- `file`, `template_file`, `static_file`: write, diff, permissions.
- `directory`, `symlink`: create, update, already-correct.
- `user`, `group`, `user_in_group`: create, already-exists.
- `hosts_entry`: add, update, already-correct.
- `command`: execute, trigger-only behavior.
- Module and manifest-composition scenarios: nested includes, host overrides,
  iterator expansion, and notify / trigger behavior in containerized runs.

### Tier 4: Reference Deployment Tests

**Scope**: full manifest against real or near-real infrastructure.
**Environment**: VM or dedicated test host with ZFS, Docker, Kerberos.
**Approach**: `anneal apply` on a fresh OS install, then verify convergence.

Tests:
- Fresh install → `anneal apply` → all resources converged.
- Second `anneal apply` → empty plan (idempotency).
- Manifest change → `anneal plan` shows expected diff → `anneal apply` converges.
- Add user → `anneal apply` → user, home dir, group membership created.

These tests are expensive and run manually or in CI with VM provisioning.

### Tier 5: Screencast Smoke Proof

**Scope**: operator-visible proof that the core workflow behaves correctly.
**Environment**: reproducible terminal capture in CI or release verification,
preferably against a Dockerized baseline for the minimal supported flow.
**Approach**: record and replay a basic operation screencast that exercises:

1. `anneal validate` on a valid manifest.
2. `anneal plan` showing the initial diff.
3. `anneal apply` converging the host.
4. A second `anneal plan` or `anneal apply` proving idempotency.

Requirements:
- The screencast asset is versioned or reproducibly generated from repo-owned
  scripts and fixtures.
- The smoke scenario uses a small but representative manifest that touches at
  least one real provider path, not a mocked no-op flow.
- Release verification fails if the screencast scenario no longer matches the
  documented operator workflow.

## Custom Provider Testing

Custom providers are tested by:
1. **Syntax check**: `anneal validate` catches shell parse errors in provider
   scripts.
2. **Mock testing**: provider scripts can be run against mock `read` input and
   their `emit` output compared to expected stdlib calls.
3. **Integration**: include the custom provider in a tier 2 golden-file test.

## Acceptance Criteria Verification

Each user story's acceptance criteria maps to a specific test:

| Criteria pattern | Test tier |
|-----------------|-----------|
| "produces no operations" (idempotency) | Tier 2 golden-file |
| "produces stdlib_X operations" (plan content) | Tier 2 golden-file |
| "validates at validate time" (error detection) | Tier 1 unit |
| "works end-to-end" (full lifecycle) | Tier 3 or 4 integration |
| "readable by a sysadmin" (plan quality) | Manual review + tier 2 |
| "basic workflow is demonstrably usable" (operator proof) | Tier 5 screencast smoke |
