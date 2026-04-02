---
dun:
  id: FEAT-004
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-004 - User & Access Management

**Feature ID**: FEAT-004
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

User and access management providers handle system users, groups, group
membership, POSIX ACLs, and sudo configuration. These are foundational — most
other resources (files, services, storage) depend on users and groups existing.

## Problem Statement

- **Current situation**: timbuktu uses `ensure_user`, `ensure_group`,
  `ensure_user_in_group` helpers and raw `setfacl` calls. sahara uses
  `ensure_file` for sudoers entries.
- **Pain points**: Adding a user requires touching multiple scripts. ACLs are
  set imperatively with no preview. Sudo config is a raw file write with no
  validation.
- **Desired outcome**: Users, groups, ACLs, and sudo entries are declarative
  resources with plan output showing exactly what changes.

## Requirements

### Functional Requirements

- User and group providers must be idempotent: existing users/groups with
  correct properties produce no operations.
- User creation supports: name, primary group, shell, system flag, home
  directory flag.
- POSIX ACL provider supports both access ACLs and default ACLs on directories.
- Sudoers provider validates syntax before writing (visudo -c equivalent).

### Non-Functional Requirements

- **Security**: sudoers entries must be syntax-validated before writing to
  prevent lockouts.
- **Reliability**: user creation must create the primary group first (implicit
  dependency).

## User Stories

### US-020: Create system users [FEAT-004]
**As a** server operator
**I want** to declare users in my manifest
**So that** user accounts are created idempotently with correct properties

**Acceptance Criteria:**
- [ ] `user` provider reads from /etc/passwd.
- [ ] Missing users produce `stdlib_user_create` operations.
- [ ] Existing users with correct properties produce no operations.
- [ ] Supports name, primary group, shell, system flag.

### US-021: Create system groups [FEAT-004]
**As a** server operator
**I want** to declare groups in my manifest
**So that** groups exist before users and ACLs that reference them

**Acceptance Criteria:**
- [ ] `group` provider reads from /etc/group.
- [ ] Missing groups produce `stdlib_group_create` operations.

### US-022: Manage group membership [FEAT-004]
**As a** server operator
**I want** to add users to supplementary groups
**So that** access control is managed declaratively

**Acceptance Criteria:**
- [ ] `user_in_group` provider checks current group membership.
- [ ] Missing membership produces `stdlib_user_add_group` operations.
- [ ] Does not remove users from groups not declared (additive only).

### US-023: Set POSIX ACLs [FEAT-004]
**As a** server operator managing shared directories
**I want** to declare POSIX ACLs on paths
**So that** fine-grained access is managed alongside the directory

**Acceptance Criteria:**
- [ ] `posix_acl` provider reads current ACLs via `getfacl`.
- [ ] Changed ACLs produce `stdlib_setfacl` operations.
- [ ] Supports both access ACLs and default ACLs for inheritance.

### US-024: Configure sudo access [FEAT-004]
**As a** workstation operator
**I want** to declare sudoers entries
**So that** sudo configuration is managed and syntax-validated

**Acceptance Criteria:**
- [ ] `sudoers_entry` provider manages files in /etc/sudoers.d/.
- [ ] Content is validated (equivalent to `visudo -c`) before writing.
- [ ] Invalid syntax fails at plan time, not apply time.

## Edge Cases and Error Handling

- User exists with wrong primary group: plan shows a modify operation.
- Group referenced by a user doesn't exist: dependency ordering ensures group
  is created first. If the group resource is missing from the manifest, validate
  warns.
- ACL set on a path that doesn't exist: fail at plan time with clear error.
- Sudoers syntax error: fail at plan time, never write invalid sudoers.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib.
- File management (FEAT-005): sudoers provider uses file-write primitives.

## Out of Scope

- PAM module configuration beyond sudoers.
- LDAP/AD user management.
- SSH authorized_keys management (use file or template provider).
