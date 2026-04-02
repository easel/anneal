---
dun:
  id: ADR-002
  depends_on:
    - ADR-001
    - SD-001
---
# ADR-002: Embedded Shell Interpreter (mvdan/sh)

**Status**: Accepted
**Date**: 2026-04-01

## Context

Anneal's plan-as-script model (ADR-001) requires executing shell scripts for
both plan execution and custom providers. The interpreter must produce identical
behavior across Linux distributions regardless of what shell is installed on the
host.

Options considered:

1. **System shell** (`/bin/sh` or `/bin/bash`): zero binary size overhead, but
   varies across distros (dash on Ubuntu, bash 3.2 on macOS, busybox on Alpine).
   Custom providers would behave differently on different hosts.

2. **Embedded busybox**: full POSIX shell + coreutils. Requires cgo for
   cross-compilation, violating the "no cgo" constraint.

3. **mvdan/sh**: pure Go POSIX shell interpreter with bash extensions. Powers
   `shfmt`, which parses every bash script on GitHub. Battle-tested, no cgo.

4. **Starlark or Lua**: embedded scripting languages. Would require providers to
   learn a new language instead of writing shell scripts.

## Decision

**mvdan/sh** — a pure Go POSIX shell interpreter with bash extensions.

## Consequences

### Positive

- **Portability**: identical behavior on every host, regardless of installed
  shell.
- **Single binary**: pure Go, no cgo. The interpreter compiles into the Anneal
  binary.
- **Familiar**: providers are shell scripts, not a new DSL. Operators already
  know shell.
- **Battle-tested**: powers shfmt, which is used across the Go ecosystem.
- **Stdlib as builtins**: stdlib operations are registered as interpreter
  built-in functions, making them available without sourcing a library file.

### Negative

- **Binary size**: adds ~2-3 MB to the binary.
- **Not 100% bash**: mvdan/sh supports most bash extensions (arrays, `[[ ]]`,
  `local`, process substitution) but not everything. Exotic bash features may
  not work. Mitigated by maintaining a compatibility test suite.
- **Performance**: interpreted shell is slower than native execution for
  compute-heavy operations. Acceptable because providers mostly exec external
  commands (apt, zfs, docker) — the interpreter overhead is negligible compared
  to I/O.

### Rejected Alternatives

- **System shell**: rejected because behavior varies across distros, violating
  the portability principle.
- **Embedded busybox**: rejected because it requires cgo.
- **Starlark/Lua**: rejected because "if you can write a shell script, you can
  write a provider" is a core product principle. Requiring a new language
  defeats the purpose.
