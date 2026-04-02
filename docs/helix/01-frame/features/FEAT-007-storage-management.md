---
dun:
  id: FEAT-007
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-007 - Storage Management

**Feature ID**: FEAT-007
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

Storage management providers handle ZFS datasets and properties. ZFS is a
primary storage layer for the reference deployment and requires careful handling
of recursive vs per-dataset properties, encryption, and special vdev tuning.

## Problem Statement

- **Current situation**: timbuktu uses raw `zfs create` and `zfs set` calls
  across multiple scripts (02-zfs-optimize.sh, 04-zfs-shares.sh). Property
  tuning is scattered with complex per-dataset overrides.
- **Pain points**: No preview of property changes. Recursive vs per-dataset
  semantics are error-prone. Missing datasets cause silent `zfs set` failures.
- **Desired outcome**: ZFS datasets and properties are resources with clear
  plan output showing what will be created or changed.

## Requirements

### Functional Requirements

- Dataset creation supports properties at creation time (encryption,
  compression, recordsize).
- Property management supports single datasets, dataset lists, and recursive
  application.
- Properties on missing datasets produce a warning and skip, not a fatal error
  (matches current script behavior).
- Encryption support: key format, key location, and raw keyfile path.

### Non-Functional Requirements

- **Reliability**: `zfs list` and `zfs get` are the only read operations —
  no destructive reads.
- **Performance**: batch `zfs get` calls where possible rather than per-property
  queries.

## User Stories

### US-050: Create ZFS datasets [FEAT-007]
**As a** server operator
**I want** to declare ZFS datasets with properties
**So that** datasets are created idempotently with correct settings

**Acceptance Criteria:**
- [ ] `zfs_dataset` provider checks if dataset exists.
- [ ] Missing datasets produce create operations with declared properties.
- [ ] Existing datasets produce no operations (properties managed separately).
- [ ] Encryption is supported: key format, key location, keylength.

### US-051: Manage ZFS properties [FEAT-007]
**As a** server operator tuning ZFS performance
**I want** to declare desired properties on datasets
**So that** compression, recordsize, atime, etc. converge to declared values

**Acceptance Criteria:**
- [ ] `zfs_properties` provider reads current properties via `zfs get`.
- [ ] Changed properties produce `zfs set` operations.
- [ ] Supports single dataset, dataset list, and recursive mode.
- [ ] Properties on non-existent datasets produce a warning and skip.
- [ ] Plan shows current vs desired value for each changed property.

## Edge Cases and Error Handling

- Dataset parent doesn't exist: `zfs create -p` creates parents.
- Encrypted dataset with missing keyfile: fail at apply time with clear error.
- Property cannot be changed on existing dataset (e.g., encryption, volblocksize):
  warn in plan, do not attempt.
- Recursive property set on pool root: apply to pool and all children.
- Dataset exists but with wrong encryption settings: warn that encryption
  properties cannot be changed after creation.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib.
- stdlib operations: `stdlib_exec` wrapping `zfs create` and `zfs set` (ZFS
  commands are too specialized for dedicated stdlib ops).

## Out of Scope

- ZFS pool creation or management (zpool).
- ZFS snapshot management (handled by `anneal snapshot` command, not a resource).
- ZFS send/receive.
- zvol management (see FEAT-011: iSCSI).
