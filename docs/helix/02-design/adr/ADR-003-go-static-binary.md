---
dun:
  id: ADR-003
  depends_on:
    - anneal.prd
    - SD-001
---
# ADR-003: Go Static Binary

**Status**: Accepted
**Date**: 2026-04-01

## Context

Anneal must ship as a single binary with no runtime dependencies beyond the OS.
The implementation language determines binary distribution, cross-compilation,
and the available ecosystem for template engines, YAML parsing, and shell
interpretation.

Options considered:

1. **Go**: static binary by default, excellent cross-compilation, good standard
   library, strong YAML/template ecosystem (text/template, Sprig), mvdan/sh for
   embedded interpreter.

2. **Rust**: static binary possible, faster runtime, but slower compile times,
   less familiar to the target user base, and no mature embedded shell
   interpreter in pure Rust.

3. **Python**: rich CM ecosystem (Ansible, SaltStack), but requires a runtime on
   the target host — violates the zero-dependency constraint.

## Decision

**Go** with `CGO_ENABLED=0` for static binaries.

Key libraries:
- **text/template + Sprig**: template expressions in manifests and template files
- **mvdan/sh**: embedded shell interpreter (ADR-002)
- **cobra**: CLI framework
- **yaml.v3**: YAML parsing

## Consequences

### Positive

- **Single binary**: `go build` produces one static executable. `scp` to deploy.
- **Cross-compilation**: `GOOS=linux GOARCH=amd64` from any dev machine.
- **Ecosystem**: mvdan/sh, Sprig, cobra, and yaml.v3 are all mature, pure Go,
  no cgo.
- **Familiar**: Go is well-known in the infrastructure/DevOps space (Terraform,
  Kubernetes, Docker are all Go).

### Negative

- **Verbosity**: Go requires more boilerplate than Python or Rust for some
  patterns (error handling, generics).
- **Binary size**: Go binaries are larger than C/Rust equivalents. Mitigated by
  `go build -ldflags="-s -w"` and acceptable for a CLI tool.
- **No runtime plugins**: Go's plugin system is fragile and platform-specific.
  Custom extensibility is via shell scripts (ADR-002), not Go plugins.
