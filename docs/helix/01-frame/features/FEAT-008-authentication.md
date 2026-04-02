---
dun:
  id: FEAT-008
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-008 - Authentication

**Feature ID**: FEAT-008
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

Authentication providers manage MIT Kerberos KDC initialization, service
principals, and keytab generation. These are required for NFSv4 with Kerberos
authentication in the reference deployment.

## Problem Statement

- **Current situation**: timbuktu's 03-kerberos-kdc.sh handles debconf
  pre-seeding, package install, template rendering, KDC initialization,
  principal creation, and keytab export in one monolithic script.
- **Pain points**: No preview of Kerberos changes. Principal creation is
  idempotent but opaque. Keytab regeneration requires manual tracking.
- **Desired outcome**: KDC init, principals, and keytabs are separate resources
  with clear dependencies and plan output.

## Requirements

### Functional Requirements

- KDC initialization is a one-time operation (only if database absent).
- Principal creation is idempotent: existing principals produce no operations.
- Keytab export specifies which principals to include and the output path.
- Debconf pre-seeding for krb5-config packages is handled before package
  installation (dependency on package provider).

### Non-Functional Requirements

- **Security**: KDC master password is a secret, resolved via the secret
  provider chain. Never in plans or logs.
- **Reliability**: keytab generation must happen after all principals are
  created (dependency ordering).

## User Stories

### US-060: Initialize Kerberos KDC [FEAT-008]
**As a** server operator setting up Kerberos authentication
**I want** the KDC database created if it doesn't exist
**So that** Kerberos infrastructure is set up automatically

**Acceptance Criteria:**
- [ ] `kerberos_kdc` provider checks for `/var/lib/krb5kdc/principal`.
- [ ] Missing database produces initialization operations.
- [ ] Existing database produces no operations.
- [ ] Master password is resolved from the secret provider chain.
- [ ] Plan shows `(secret)` for the master password.

### US-061: Create Kerberos principals [FEAT-008]
**As a** server operator
**I want** to declare service and host principals
**So that** NFS, host, and admin principals exist for authentication

**Acceptance Criteria:**
- [ ] `kerberos_principal` provider checks principal existence via `kadmin.local`.
- [ ] Missing principals produce creation operations.
- [ ] Supports `randkey` for service/host principals.
- [ ] Depends on KDC initialization.

### US-062: Generate keytab files [FEAT-008]
**As a** server operator
**I want** to declare which principals go in a keytab file
**So that** services can authenticate via the keytab

**Acceptance Criteria:**
- [ ] `kerberos_keytab` provider manages a keytab file at a specified path.
- [ ] Declares which principals to export.
- [ ] Depends on all listed principals being created.
- [ ] Keytab file mode is enforced (typically 0600).

## Edge Cases and Error Handling

- KDC already initialized: no-op, skip.
- Principal already exists: no-op (kadmin.local listprincs check).
- Keytab path directory doesn't exist: fail with clear error (or depend on a
  directory resource).
- kadmin.local not available: fail at plan time if kerberos resources exist but
  krb5-admin-server is not installed.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib, secret provider chain.
- Package management (FEAT-003): krb5 packages must be installed first.
- File management (FEAT-005): Kerberos config templates (krb5.conf, kdc.conf,
  kadm5.acl).

## Out of Scope

- Kerberos client onboarding (run on client, not server).
- Cross-realm trust.
- LDAP-backed KDC.
