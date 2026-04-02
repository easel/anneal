---
dun:
  id: FEAT-009
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-009 - Network & System

**Feature ID**: FEAT-009
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

Network and system providers handle hosts file entries, LUKS/crypttab
configuration, binary installation from URLs, and the generic command resource
used for triggers and one-off operations.

## Problem Statement

- **Current situation**: timbuktu manages /etc/hosts entries, LUKS crypttab
  entries, and binary downloads (node_exporter, Garage) via custom shell
  functions. The command resource is implicit — service restarts and exportfs
  reloads are scattered throughout scripts.
- **Pain points**: Binary version tracking is ad-hoc (marker files). Hosts file
  entries are checked with grep. Crypttab management uses UUID lookup. No
  preview for any of these.
- **Desired outcome**: All of these are declarative resources with plan output.
  The command resource is the explicit escape hatch and trigger target.

## Requirements

### Functional Requirements

- Hosts entries are idempotent by IP+hostname combination.
- Crypttab entries are managed by LUKS device UUID, not device path.
- Binary install tracks version via a marker file alongside the binary.
- Binary install supports URL template expressions for architecture and version.
- Binary install supports tar/zip extraction with configurable strip and file
  selection.
- The command resource serves as both escape hatch (arbitrary shell) and trigger
  target (notify/trigger pattern).

### Non-Functional Requirements

- **Reliability**: binary download failures must not corrupt the existing binary
  (atomic write: download to temp, verify, move).
- **Performance**: binary version check is local (read marker file), not a
  network request.

## User Stories

### US-070: Manage /etc/hosts entries [FEAT-009]
**As a** server operator
**I want** to declare hosts file entries
**So that** hostname resolution is managed alongside other config

**Acceptance Criteria:**
- [ ] `hosts_entry` provider reads /etc/hosts and matches by IP.
- [ ] Missing entries produce append operations.
- [ ] Existing entries with wrong hostname produce update operations.
- [ ] Supports aliases (multiple names per IP).

### US-071: Manage crypttab entries [FEAT-009]
**As a** server operator with LUKS-encrypted root
**I want** to declare crypttab entries by UUID
**So that** LUKS unlock configuration is managed declaratively

**Acceptance Criteria:**
- [ ] `crypttab_entry` provider reads /etc/crypttab.
- [ ] Matches entries by LUKS device UUID.
- [ ] Missing entries produce append operations.
- [ ] Supports Clevis/Tang unlock options.

### US-072: Install binaries from URLs [FEAT-009]
**As a** operator deploying tools that aren't in a package repo
**I want** to declare a binary with a download URL and version
**So that** the binary is downloaded, extracted, and version-tracked

**Acceptance Criteria:**
- [ ] `binary_install` provider reads a `.version` marker file.
- [ ] Version mismatch produces download + extract + install operations.
- [ ] URL supports template expressions for `{{ .Arch }}`, `{{ .Version }}`, etc.
- [ ] Extraction supports tar.gz and zip with strip-components and file
  selection.
- [ ] Install is atomic: download to temp, then move to final path.

### US-073: Run arbitrary commands as triggers [FEAT-009]
**As a** operator
**I want** a generic command resource that fires on notify
**So that** service restarts, cache reloads, and other side effects are triggered
by config changes

**Acceptance Criteria:**
- [ ] `command` resource with `trigger: true` only executes when notified.
- [ ] `command` resource without `trigger` always executes (escape hatch).
- [ ] Plan output shows the command that will run.
- [ ] Multiple resources can notify the same trigger — it fires once.

## Edge Cases and Error Handling

- Binary download URL returns 404: fail at apply time with URL and HTTP status.
- Tar archive missing the expected file after extraction: fail with clear error.
- /etc/hosts has duplicate entries for the same IP: provider manages one entry,
  does not deduplicate others.
- Crypttab UUID not found on system: warn at plan time (device might not be
  attached).
- Command trigger that is never notified: never executes, no warning.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib, notify/trigger system.
- Manifest system (FEAT-002): template expressions for URL construction.

## Out of Scope

- Network interface configuration (use template_file for systemd-networkd
  configs).
- Firewall rules (use command resource or custom provider).
- DNS configuration beyond /etc/hosts.
