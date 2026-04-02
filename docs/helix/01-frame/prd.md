---
dun:
  id: anneal.prd
  depends_on:
    - anneal.vision
---
# Product Requirements Document

## Summary

Anneal is a declarative host configuration engine — a single binary that reads
a manifest, diffs it against actual system state, and produces an executable
plan to converge the machine. It targets solo operators and small teams who
manage bare-metal or VM hosts and want Terraform-style plan/apply workflow
without the weight of enterprise configuration management.

See [Product Vision](../00-discover/product-vision.md) for positioning, target
users, and design principles.

## Problem and Goals

### Problem

Operators managing individual servers must choose between fragile shell scripts
and enterprise CM tools designed for fleets. Shell scripts lack preview,
dependency ordering, and composability. Enterprise tools require agents,
runtimes, or client-server infrastructure that is disproportionate for one to
ten hosts.

No existing tool provides:
- A preview of every change before execution.
- An executable, auditable plan artifact — not an opaque summary.
- Extensibility without requiring a specific programming language or
  recompilation.
- Zero runtime dependencies on the target host.

### Goals

1. Provide a validate → plan → apply workflow where the plan is a readable,
   executable artifact.
2. Cover common Linux host primitives out of the box: packages, users, files,
   services, storage, containers.
3. Allow extension with custom resource providers and manifest composition —
   without recompiling.
4. Ship as a single binary with no runtime dependencies beyond the OS.
5. Make 1Password a first-class secret backend alongside environment variables,
   with an extension path for other vaults.

### Success Metrics

| Metric | Target | Measurement Method | Timeline |
|--------|--------|-------------------|----------|
| Reference deployment coverage | 100% of current timbuktu shell scripts expressible as Anneal manifest | Side-by-side comparison against reference host | v1.0 |
| Plan readability | Sysadmin unfamiliar with Anneal can understand the plan | User testing with 3 operators | v1.0 |
| Extension time | Custom provider written and working in < 1 hour | Timed walkthrough | v1.0 |
| Plan speed | < 5 seconds for 50+ resource manifest | Benchmark on reference host | v1.0 |
| Convergence idempotency | Zero changes on re-apply of converged system | Automated test | v1.0 |

### Non-Goals

- Multi-host orchestration, SSH, or push-based deployment.
- Cloud resource provisioning (VMs, networks, load balancers).
- Orphan detection or drift remediation for resources removed from the manifest.
- GUI, web interface, or hosted service.
- Windows support in the initial release.
- A module registry or marketplace (day-1 modules are local files).

## Users and Scope

### Primary Persona: Solo Operator
**Role**: Home lab admin or solo infrastructure operator
**Goals**: Declarative configuration with preview for a handful of hosts
**Pain Points**: Shell scripts are fragile and have no preview; enterprise CM
tools are overkill

### Secondary Persona: Small Team Infra Lead
**Role**: Engineer responsible for a team's dedicated servers
**Goals**: Terraform-like workflow for machines alongside cloud infrastructure
**Pain Points**: No equivalent of `terraform plan` for the machines themselves

## Requirements

### Must Have (P0)

1. **Validate → plan → apply workflow.** Three distinct commands: validate
   checks the manifest without system access; plan reads state and produces an
   executable artifact; apply executes the plan.
2. **Plan as executable artifact.** The plan is composed of standard library
   operations that a sysadmin can read and understand. What you review is what
   runs.
3. **Apply validates before executing.** When given a saved plan, apply confirms
   the system still produces the same plan before executing. Drift invalidates
   the saved plan.
4. **Built-in resource providers** covering the primitives needed for both
   server and workstation provisioning. Each provider category is specified in
   its own feature:
   - FEAT-003: Package Management (apt, dnf, brew, deb, npm, pip, apt repos)
   - FEAT-004: User & Access Management (users, groups, ACLs, sudoers)
   - FEAT-005: File Management (files, templates, static copies, directories,
     symlinks, secret files, file removal)
   - FEAT-006: Service Management (systemd services/units, Docker containers)
   - FEAT-007: Storage Management (ZFS datasets, ZFS properties)
   - FEAT-008: Authentication (Kerberos KDC, principals, keytabs)
   - FEAT-009: System Utilities (hosts entries, crypttab, binary installs,
     generic command)
5. **Custom resource providers via shell scripts.** Users can add new resource
   kinds without recompiling. Custom providers implement the same contract as
   built-in providers and produce the same standard library operations.
6. **Embedded shell interpreter.** Custom providers and plan execution run in an
   embedded interpreter, not the host's shell.
7. **Manifest composition.** Manifests can include other manifests with variable
   passing. Included manifests are self-contained and independently validatable.
8. **Variable system.** Defaults, host-specific overrides, and environment
   variable overrides with clear precedence.
9. **Template expressions.** Manifest and template files support conditionals,
   loops, and functions — not just variable substitution.
10. **Dependency ordering.** Resources are ordered by a DAG with explicit
    dependencies, declaration-order tiebreaker, and cycle detection at validate
    time.
11. **Notify/trigger system.** Resources can notify trigger resources; triggers
    only fire when the notifying resource actually changed.
12. **Iterators.** Resources can expand over lists, producing multiple concrete
    resources from a single declaration.
13. **Secret management.** Secrets referenced by name, never stored in manifests,
    never shown in plans. Resolution chain: environment variables → 1Password →
    fail (or empty if optional). Auto-generation for first-run secrets. Must
    support pre-resolved secrets (resolve as unprivileged user before apply runs
    as root, since root may not have 1Password session access).
14. **Run-as-user support.** Resources can specify a user to run operations as.
    Required for Homebrew, npm, pip, and other tools that must not run as root.
15. **Fail-stop execution.** Stop on first error. Report what was applied, what
    was skipped, and sufficient context to diagnose the failure.
16. **Single binary install.** One file, no runtime dependencies beyond the OS.
17. **Built-in template variables.** Manifests and templates have access to
    system-derived variables: hostname, FQDN, architecture (Go, Debian, and
    kernel naming conventions), and OS version.
18. **Proof-oriented verification.** The project must ship with robust unit
    tests, golden plan tests, and Docker-based integration coverage across the
    supported Linux distribution matrix and representative configuration
    combinations. Release verification must also include a recorded basic
    operation screencast that proves the validate → plan → apply → re-apply
    flow and is exercised as a smoke test.

### Should Have (P1)

1. **Composite resources.** Resource kinds that expand into multiple primitives
   with internal dependency edges (e.g., user provisioning that creates group,
   user, home directory, and permissions as one declaration).
2. **Resource filtering.** Scope plan/apply to a subset of resources by kind or
   name pattern.
3. **Single-resource inspection.** Show current state vs desired state for one
   resource.
4. **Pre-apply snapshots.** A provider-supplied checkpoint mechanism before
   apply (e.g., filesystem snapshots).
5. **Parameterized modules.** Included manifests accept typed variables with
   defaults and can be validated in isolation.
6. **Custom secret providers.** Shell-script secret backends extending the
   resolution chain beyond env and 1Password.
7. **Plan comparison.** Diff two plan files to see what changed between runs.
8. **iSCSI provider.** ZFS zvol + LIO targetcli management for iSCSI targets
   (FEAT-010).
9. **Fact gathering.** Auto-detected system facts (OS family, distro, version,
   architecture, network interfaces, memory, disk layout) available as template
   variables. Enables manifests that adapt to the host without manual variable
   overrides. Required for multi-platform support.
10. **Inter-resource references.** A resource's spec can reference another
    resource's output (e.g., read a generated secret, capture a command's
    stdout). Needed for workflows where one resource produces a value consumed
    by another.
11. **macOS provider tier.** Homebrew (formulae/casks/taps), launchd services,
    plist management, dscl user management. Required for the sahara reference
    deployment's macOS targets.
12. **Line-in-file / block-in-file.** Manage individual lines or blocks within
    existing files without rewriting the whole file. Required for sshd_config,
    shell profiles, and similar managed-by-others files.
13. **Conditional resources.** A `when:` clause on resources for conditional
    inclusion based on facts or variables, beyond what template `{{ if }}`
    blocks can express.

### Nice to Have (P2)

1. **Module sharing.** Publish, discover, and fetch reusable manifest modules.
2. **Plan approval workflow.** Save plan, send for review, apply later.
3. **Partial apply.** Apply a subset of a plan's operations.
4. **Dry-run execution.** Execute the plan with a standard library that logs
   operations instead of performing them.
5. **Encrypted variable files.** Encrypt secrets at rest in git-committed
   variable files (analogous to Ansible Vault).

## Constraints, Assumptions, Dependencies

### Constraints

- **Local execution only**: runs on the target host, not remotely.
- **Linux-first**: Debian-family distributions primary. Provider interface must
  abstract OS primitives so other platforms can be added.
- **Single binary**: no runtime dependencies, no plugin discovery at filesystem
  level beyond the manifest directory.
- **Secrets never in plans**: plans contain secret *references*, not values.
  The secret provider chain is invoked at apply time to resolve references.
  This means a saved plan is not fully self-contained — secret providers must
  be available when the plan is executed. Secrets never appear on disk in
  cleartext.

### Assumptions

- Operators are comfortable with YAML and basic shell scripting.
- The target host has a package manager (apt), init system (systemd), and
  standard POSIX userland.
- 1Password CLI is available on hosts where 1Password secret resolution is
  needed; env var fallback covers CI and hosts without it.

### Dependencies

- Embedded shell interpreter capable of POSIX sh + common bash extensions.
- 1Password CLI (`op`) for built-in secret resolution.
- Target OS package manager and init system for built-in providers.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Standard library surface area grows uncontrollably | Medium | High | Keep stdlib minimal; escape hatch for uncommon operations |
| Plan-as-script model limits what providers can express | Medium | High | Validate against reference deployment before locking stdlib |
| Drift detection between plan and apply is too strict or too loose | High | Medium | Start with full re-plan comparison; refine scoping later |
| Embedded interpreter diverges from user expectations of "shell" | Medium | Medium | Document supported syntax; test against real-world provider scripts |
| Module composition creates unexpected interactions | Medium | Medium | Modules are validated in isolation; include graph checked for cycles |

## Success Criteria

- Two reference deployments are fully expressible as Anneal manifests:
  timbuktu/eldir (server: 50+ resources — packages, users, ZFS, Kerberos, NFS,
  Samba, Docker, monitoring) and sahara (workstation: packages, Homebrew,
  language runtimes, editors, services, Docker, security config).
- `anneal plan` output for the reference deployment is understandable by a
  sysadmin who has never used Anneal.
- Adding a user to the reference deployment is a manifest edit + `anneal apply`.
- A custom shell provider can be written, tested, and used in under an hour by
  someone familiar with shell scripting.
- `anneal plan` completes in under 5 seconds for the reference deployment.
- `anneal apply` on a converged system completes in seconds with zero changes.
- CI or release verification proves the core workflow with a reproducible
  screencast smoke run covering validate, plan, apply, and idempotent re-apply.
