---
dun:
  id: anneal.implementation-plan
  depends_on:
    - anneal.prd
    - SD-001
---
# Implementation Plan

## Phasing Strategy

Implementation is phased to deliver value incrementally. Each phase produces a
working `anneal plan` + `anneal apply` for a progressively larger set of
resources. The reference deployments (timbuktu server, sahara workstation) are
the validation targets.

## Phase 1: Core Engine

**Goal**: `anneal validate`, `anneal plan`, and `anneal apply` work end-to-end
for a minimal manifest with one resource kind.

**Delivers**:
- CLI skeleton (cobra): validate, plan, apply, version
- YAML manifest parser with schema validation
- Variable resolution: manifest vars → env var overrides
- Template expression evaluation (text/template + Sprig)
- Resource interface and provider contract (read/diff/emit)
- DAG construction with topological sort and cycle detection
- Plan script assembly from stdlib operations
- Embedded interpreter (mvdan/sh) for plan execution
- Apply with re-plan + compare validation
- Fail-stop error handling with summary output
- `System` interface with real and mock implementations
- One provider to prove the pipeline: `file` (inline content)

**Test coverage**:
- CLI integration tests (validate/plan/apply)
- Config loader unit tests (YAML, variables, templates)
- Table-driven manifest/configuration matrix tests for parser and validation
  behavior
- DAG tests (sort, cycles, diamonds, tiebreakers)
- Plan golden-file tests (known input → expected output)
- Interpreter conformance tests (mvdan/sh basics)
- A minimal screencast smoke fixture proving validate → plan → apply →
  idempotent re-apply for the first working manifest

## Phase 2: File Providers (FEAT-005)

**Goal**: all file management providers work, covering the most common resource
type.

**Delivers**:
- `template_file` with validate command
- `static_file` (verbatim copy)
- `file_copy`
- `file_absent` (paths and globs)
- `directory`
- `symlink`
- Content diff in plan output
- Mode and ownership enforcement

## Phase 3: Manifest Composition (FEAT-002)

**Goal**: manifests can include other manifests with variable passing.

**Delivers**:
- Include graph resolution with cycle detection
- Variable precedence: module defaults → root vars → host file → env vars
- Two-pass template evaluation (iterator expansion, then variable interpolation)
- Iterator (`each`) expansion
- Notify/trigger system

## Phase 4: Package Providers (FEAT-003)

**Goal**: all package management providers work.

**Delivers**:
- `apt_packages`, `apt_purge`
- `apt_repo` (signing key + sources list)
- `deb_install` (URL + version tracking)
- `dnf_packages`
- `brew_packages` (formulae + casks)
- `npm_packages`, `pip_packages`
- Docker-matrix integration harness covering package providers across supported
  Linux distribution families and representative package scenarios

## Phase 5: User, Service, Network Providers (FEAT-004, FEAT-006, FEAT-009)

**Goal**: user management, systemd, Docker containers, and network primitives.

**Delivers**:
- `user`, `group`, `user_in_group`
- `posix_acl`, `sudoers_entry`
- `systemd_service`, `systemd_unit`
- `docker_container` (stop/rm/run cycle, health checks)
- `hosts_entry`, `crypttab_entry`
- `binary_install` (URL + extract + version marker)
- `command` (escape hatch + trigger target)

## Phase 6: Secret Management

**Goal**: secrets work end-to-end.

**Delivers**:
- Secret reference syntax in resource specs
- Secret provider chain: env vars → 1Password CLI
- `anneal_secret` runtime resolution in plan scripts
- `secret_file` provider (write, generate, warn)
- `(secret)` rendering in plan output and logs
- Auto-generation with operator warning

## Phase 7: Storage and Auth Providers (FEAT-007, FEAT-008)

**Goal**: ZFS and Kerberos providers for the server reference deployment.

**Delivers**:
- `zfs_dataset` (create with properties, encryption)
- `zfs_properties` (set, recursive, skip-if-missing)
- `kerberos_kdc` (initialize if absent)
- `kerberos_principal` (idempotent creation)
- `kerberos_keytab` (export principals to file)

## Phase 8: Composites and Polish (FEAT-010)

**Goal**: composite resources and remaining P1 features.

**Delivers**:
- `user_home` composite (group → user → directory → permissions)
- Resource filtering (`--filter`)
- `anneal show` (single resource inspection)
- `anneal snapshot` (pre-apply checkpoint hook)
- Plan comparison (`anneal diff plan1.sh plan2.sh`)

## Phase 9: Reference Deployment Migration

**Goal**: both reference deployments fully expressed as Anneal manifests.

**Delivers**:
- Complete timbuktu/eldir manifest (server)
- Complete sahara manifest (workstation)
- Template migration from envsubst to text/template
- Side-by-side validation against existing shell scripts
- Documentation: getting started guide, provider reference
- Reproducible screencast and smoke-test script for the documented operator
  workflow

## Validation Milestones

| After Phase | Validation |
|-------------|-----------|
| 1 | `anneal plan` produces a valid plan for a single-file manifest and the screencast smoke fixture proves the basic workflow |
| 3 | `anneal plan` works for a multi-file manifest with includes and iterators, covered by unit and golden tests |
| 5 | The sahara workstation manifest covers packages, users, files, services, and provider integration passes in the Docker OS matrix |
| 7 | The timbuktu server manifest covers ZFS, Kerberos, NFS, Samba, Docker, with specialized integration proof where Docker-only coverage is insufficient |
| 9 | Both reference deployments converge from fresh OS install and the release screencast smoke proof passes |
