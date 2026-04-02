---
dun:
  id: ADR-004
  depends_on:
    - ADR-001
    - FEAT-001
    - SD-001
---
# ADR-004: Standard Library as Provider Contract

**Status**: Accepted
**Date**: 2026-04-01

## Context

In the plan-as-script model (ADR-001), providers emit operations that compose
into the plan. The set of allowed operations defines the contract surface
between providers and the execution engine.

Options considered:

1. **Free-form shell**: providers emit arbitrary shell commands. The plan is a
   raw shell script.

2. **Structured operations (stdlib)**: providers emit calls from a defined set
   of operations. The plan is composed exclusively of stdlib calls.

3. **Hybrid**: stdlib for common operations, with an escape hatch for arbitrary
   commands.

## Decision

**Hybrid (Approach 3)**: a standard library of named operations for common
primitives, plus `stdlib_exec` as an escape hatch for anything the stdlib
doesn't cover.

### Stdlib Categories

| Category | Operations |
|----------|-----------|
| Files | `stdlib_file_write`, `stdlib_file_copy`, `stdlib_file_remove`, `stdlib_dir_create`, `stdlib_symlink` |
| Packages | `stdlib_apt_install`, `stdlib_apt_purge`, `stdlib_deb_install`, `stdlib_dnf_install`, `stdlib_brew_install`, `stdlib_npm_install`, `stdlib_pip_install` |
| Users | `stdlib_user_create`, `stdlib_group_create`, `stdlib_user_add_group` |
| Services | `stdlib_service_enable`, `stdlib_service_start`, `stdlib_service_stop`, `stdlib_service_restart` |
| Permissions | `stdlib_chmod`, `stdlib_chown`, `stdlib_setfacl` |
| Secrets | `anneal_secret` (runtime resolution of secret references) |
| Escape hatch | `stdlib_exec` (arbitrary command) |

### Addition Criteria

New stdlib operations are added when:
- Multiple providers need the same primitive (not just one).
- The operation is common enough to warrant a readable name in plans.
- The operation benefits from standardized error handling or dry-run support.

Single-provider operations use `stdlib_exec` until they prove common enough to
promote.

## Consequences

### Positive

- **Readability**: `stdlib_file_write /etc/foo 0644 root:root` is clearer in a
  plan than raw `cat > /etc/foo << 'EOF'`.
- **Interchangeability**: custom providers emit the same operations as built-in
  providers. A plan reader cannot distinguish them.
- **Dry-run support**: the stdlib can be swapped for a logging implementation
  that records operations without executing them.
- **Testability**: providers can be tested by asserting they emit the correct
  stdlib calls, without needing a real system.

### Negative

- **Surface area risk**: the stdlib could grow uncontrollably. Mitigated by
  strict addition criteria and the escape hatch.
- **Expressiveness ceiling**: some operations are hard to express as a single
  stdlib call. `stdlib_exec` handles these, but plans become less readable when
  overused.
- **Learning curve**: operators need to learn stdlib operation names to read
  plans. Mitigated by keeping the stdlib small and names self-descriptive.
