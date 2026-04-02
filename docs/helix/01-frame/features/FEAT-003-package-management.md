---
dun:
  id: FEAT-003
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-003 - Package Management

**Feature ID**: FEAT-003
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

Package management providers give Anneal the ability to install, remove, and
manage software packages across Linux distributions and Homebrew. This is the
most fundamental provider category — nearly every host manifest starts with
packages.

## Problem Statement

- **Current situation**: timbuktu uses `ensure_packages` (apt wrapper) and
  sahara uses separate Ubuntufile/Fedfile/Brewfile formats with custom installer
  scripts. Global language packages (npm, pip) have their own ad-hoc scripts.
- **Pain points**: No preview of what will be installed. No single format
  spanning apt, dnf, brew, and language-level packages. External repos require
  manual setup before package install.
- **Desired outcome**: One manifest declares all packages regardless of source.
  `anneal plan` shows exactly what will be installed. External repos are
  resources like any other.

## Requirements

### Functional Requirements

- Providers must be idempotent: already-installed packages produce no plan
  operations.
- Package lists support template expressions for conditional inclusion.
- External apt repositories are declared as resources with key and source
  management, ordered before the packages that depend on them.

### Non-Functional Requirements

- **Performance**: package state reads should use local cache (dpkg, rpm db,
  brew list) rather than network queries.
- **Reliability**: partial apt failures (one package missing from repo) must
  produce clear errors naming the failing package.

## User Stories

### US-010: Install system packages [FEAT-003]
**As a** server operator
**I want** to declare required system packages in my manifest
**So that** they are installed idempotently on every apply

**Acceptance Criteria:**
- [ ] `apt_packages` provider reads installed packages from dpkg database.
- [ ] Missing packages produce `stdlib_apt_install` operations in the plan.
- [ ] Already-installed packages produce no operations.
- [ ] Multiple packages are batched into a single apt call.

### US-011: Remove unwanted packages [FEAT-003]
**As a** server operator hardening a system
**I want** to declare packages that should be absent
**So that** unwanted software is purged during convergence

**Acceptance Criteria:**
- [ ] `apt_purge` provider checks if packages are installed.
- [ ] Installed packages produce `stdlib_apt_purge` operations.
- [ ] Already-absent packages produce no operations.

### US-012: Add external apt repositories [FEAT-003]
**As a** operator installing software from third-party repos
**I want** to declare external apt sources as resources
**So that** repo setup is managed alongside the packages that need it

**Acceptance Criteria:**
- [ ] `apt_repo` provider manages signing key and sources list entry.
- [ ] Resources depending on the repo are ordered after it via DAG.
- [ ] Already-configured repos produce no operations.

### US-013: Install Homebrew packages [FEAT-003]
**As a** workstation operator using Homebrew
**I want** to declare brew formulae and casks in my manifest
**So that** my development tools are managed alongside system config

**Acceptance Criteria:**
- [ ] `brew_packages` provider reads installed formulae/casks from `brew list`.
- [ ] Missing packages produce `stdlib_brew_install` operations.
- [ ] Casks are distinguished from formulae in the resource spec.
- [ ] Homebrew itself is treated as a prerequisite (error if not installed).
- [ ] All brew operations run as the unprivileged user, not root.

### US-013b: Manage Homebrew taps [FEAT-003]
**As a** workstation operator
**I want** to declare Homebrew taps as resources
**So that** third-party taps are configured before formulae that depend on them

**Acceptance Criteria:**
- [ ] `brew_tap` provider checks currently tapped repos.
- [ ] Missing taps produce `stdlib_brew_tap` operations.
- [ ] Resources depending on a tap are ordered after it via DAG.

### US-014: Install packages from .deb files [FEAT-003]
**As a** operator installing vendor-provided .deb packages
**I want** to declare a .deb URL with version tracking
**So that** the package is downloaded and installed when a new version is declared

**Acceptance Criteria:**
- [ ] `deb_install` provider tracks installed version.
- [ ] Version change produces download + `stdlib_deb_install` operations.
- [ ] URL supports template expressions for architecture and OS version.

### US-015: Manage Fedora/RHEL packages [FEAT-003]
**As a** operator running Fedora or RHEL
**I want** dnf package management equivalent to apt
**So that** the same manifest patterns work across distributions

**Acceptance Criteria:**
- [ ] `dnf_packages` provider reads from rpm database.
- [ ] Missing packages produce `stdlib_dnf_install` operations.
- [ ] Batched into a single dnf call.

### US-016: Install global npm packages [FEAT-003]
**As a** developer provisioning a workstation
**I want** to declare global npm packages
**So that** CLI tools like typescript, prettier, etc. are managed

**Acceptance Criteria:**
- [ ] `npm_packages` provider reads from `npm list -g`.
- [ ] Missing packages produce `stdlib_npm_install` operations.

### US-017: Install global Python packages [FEAT-003]
**As a** developer provisioning a workstation
**I want** to declare global Python CLI tools
**So that** Python tools are managed alongside other packages

**Acceptance Criteria:**
- [ ] `python_packages` provider uses pipx by default (PEP 668 compliance —
  raw `pip install -g` is broken on modern Ubuntu/Fedora).
- [ ] Reads from `pipx list` to check installed state.
- [ ] Missing packages produce `stdlib_pipx_install` operations.
- [ ] All operations run as the unprivileged user, not root.

### US-018: Pre-seed debconf before package install [FEAT-003]
**As a** server operator installing packages that prompt for config
**I want** to declare debconf pre-seeding values
**So that** packages like krb5-config install non-interactively

**Acceptance Criteria:**
- [ ] `apt_packages` supports an optional `debconf` field with preseed values.
- [ ] Debconf values are set before the package install operation.
- [ ] Already-set debconf values produce no operations.

### US-019: Manage Arch Linux packages [FEAT-003]
**As a** operator running Arch Linux
**I want** pacman package management
**So that** the same manifest patterns work on Arch

**Acceptance Criteria:**
- [ ] `pacman_packages` provider reads from pacman database.
- [ ] Missing packages produce `stdlib_pacman_install` operations.
- [ ] Supports AUR packages via a configured AUR helper (paru/yay).

## Edge Cases and Error Handling

- Package not found in repository: fail with clear error naming the package and
  repo, not a generic apt failure.
- Homebrew not installed: error at validate time (if brew resources exist) or
  plan time, not a silent skip.
- apt repo resource missing its signing key: fail at plan time with guidance.
- dnf/apt used on wrong distro: validate detects OS family mismatch if possible,
  otherwise clear error at plan time.
- Version conflicts between system packages and brew: out of scope — operator
  resolves manually.

## Success Metrics

- All packages from timbuktu's deploy scripts and sahara's Ubuntufile/Fedfile/
  Brewfile are expressible as Anneal resources.
- `anneal plan` for a 50-package manifest completes in < 2 seconds.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib.
- stdlib operations: `stdlib_apt_install`, `stdlib_apt_purge`,
  `stdlib_deb_install`, `stdlib_dnf_install`, `stdlib_brew_install`,
  `stdlib_brew_tap`, `stdlib_npm_install`, `stdlib_pipx_install`,
  `stdlib_pacman_install`.
- Run-as-user support (PRD P0-14): brew, npm, pipx must run as unprivileged user.

## Out of Scope

- Package version pinning with constraint ranges (e.g., `>= 1.2, < 2.0`).
- Automatic Homebrew installation (bootstrap is a custom provider or script_install).
- pip/npm project-local dependencies (only global packages).
- Coursier/Scala packages (custom provider).
