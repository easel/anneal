---
dun:
  id: FEAT-006
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-006 - Service Management

**Feature ID**: FEAT-006
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

Service management providers handle systemd services, inline systemd units, and
Docker containers. Services are the primary consumer of notify/trigger — config
file changes trigger service restarts.

## Problem Statement

- **Current situation**: timbuktu uses `ensure_service` for systemd and raw
  `docker run` blocks with stop/rm/run cycles. sahara uses the same patterns.
- **Pain points**: No preview of service state changes. Docker container config
  changes require manual stop/rm/run. No single view of "what services will
  restart if I change this config."
- **Desired outcome**: Services are resources. Config changes trigger restarts
  via notify/trigger. Docker container convergence is automatic — config change
  means recreate.

## Requirements

### Functional Requirements

- systemd providers must handle enable/disable and start/stop/restart as
  independent operations (a service can be enabled but stopped).
- Docker container provider detects config drift (image, ports, volumes, env,
  args) and recreates the container when any config changes.
- Inline systemd units write the unit file, run daemon-reload, and set the
  desired state — all as one resource with internal ordering.

### Non-Functional Requirements

- **Reliability**: service restart triggered by config change must only fire if
  the config actually changed (not on every apply).
- **Security**: Docker container env vars containing secrets must render as
  `(secret)` in plans.

## User Stories

### US-040: Manage systemd service state [FEAT-006]
**As a** server operator
**I want** to declare the desired state of a systemd service
**So that** services are enabled and started during convergence

**Acceptance Criteria:**
- [ ] `systemd_service` provider reads current enabled/active state.
- [ ] Supports states: started, stopped, disabled, masked.
- [ ] State changes produce `stdlib_service_enable/start/stop/restart` ops.
- [ ] Already-correct state produces no operations.

### US-041: Deploy inline systemd units [FEAT-006]
**As a** server operator deploying custom services
**I want** to declare a systemd unit file inline in my manifest
**So that** the unit file, daemon-reload, and service state are one resource

**Acceptance Criteria:**
- [ ] `systemd_unit` provider writes the unit file to /etc/systemd/system/.
- [ ] Changed unit content triggers daemon-reload before state change.
- [ ] Plan shows the unit file diff and the resulting state change.

### US-042: Manage Docker containers [FEAT-006]
**As a** server operator running containerized services
**I want** to declare Docker containers with their full config
**So that** containers are recreated when config changes

**Acceptance Criteria:**
- [ ] `docker_container` provider reads running container config.
- [ ] Config drift (image, ports, volumes, env, args, network_mode,
  restart_policy) produces stop + remove + run operations.
- [ ] No config drift produces no operations.
- [ ] Env vars referencing secrets render as `(secret)` in plans.
- [ ] Optional health check: poll an HTTP URL after container start.

## Edge Cases and Error Handling

- Service doesn't exist (unit file missing): plan shows unit-not-found error
  rather than trying to start a nonexistent service.
- Docker container image not available locally: pull is part of the run
  operation; failure is an apply-time error.
- Docker daemon not running: fail at plan time with clear error.
- Container restart with health check: if health check fails after restart,
  report the failure but do not roll back (fail-stop).

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib, notify/trigger.
- File management (FEAT-005): systemd_unit uses file-write for the unit file.

## Out of Scope

- Docker Compose or multi-container orchestration.
- Container image building.
- Podman (can be added as a separate provider later).
- systemd timers (use generic command or template_file + systemd_service).
