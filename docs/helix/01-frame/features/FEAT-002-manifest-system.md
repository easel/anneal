---
dun:
  id: FEAT-002
  depends_on:
    - anneal.prd
---
# Feature Specification: FEAT-002 - Manifest System

**Feature ID**: FEAT-002
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

The manifest system is how operators declare desired host state. It covers the
manifest format, variable resolution, template expressions, include/module
composition, resource declarations, dependency ordering, iterators, and the
notify/trigger system.

## Problem Statement

- **Current situation**: Host configuration is either imperative scripts or
  tool-specific DSLs (Ansible YAML+Jinja2, Puppet DSL, Chef Ruby).
- **Pain points**: No standard way to compose reusable configuration fragments.
  Variable layering is ad-hoc. Template languages vary from envsubst to Jinja2.
  Dependencies are implicit in script ordering.
- **Desired outcome**: A manifest format that is declarative, composable, and
  familiar to anyone who has used Terraform or Kubernetes YAML.

## Requirements

### Functional Requirements

- Manifests are YAML files declaring resources with `kind`, `name`, `spec`, and
  optional `depends_on`, `notify`, `trigger`, and `each` fields.
- Variables are declared in the manifest and resolved through a clear precedence
  chain: module defaults → root manifest → host-specific overrides → environment
  variables.
- Template expressions in manifests and template files support conditionals,
  loops, and functions.
- Manifests can include other manifests. Included manifests accept variables
  from the including manifest that override their defaults.
- Resources declare ordering via `depends_on` edges and declaration order. DAG
  construction, topological sort, and cycle detection are the engine's
  responsibility (FEAT-001); the manifest system provides the declarations.
- The `each` field expands a resource over a list, producing multiple concrete
  resources before dependency resolution.
- Resources with `notify` mark a trigger as pending when they change. Trigger
  resources (`trigger: true`) execute after all normal resources, only if
  notified. Multiple notifiers → trigger fires once.
- Composite resource kinds (P1) expand into multiple primitive resources with
  internal dependency edges. The expansion is visible in the plan.
- Secrets are referenced by name in resource specs. Secret values are never
  stored in the manifest.

### Non-Functional Requirements

- **Familiarity**: a Terraform or Kubernetes user should be able to read a
  manifest and understand its structure without documentation.
- **Validation speed**: `anneal validate` on a 100-resource manifest with
  includes completes in under 1 second.
- **Error quality**: validation errors cite the file, line, and specific problem
  (undefined variable, unknown kind, missing required field).

## User Stories

### US-005: Compose a host from modules [FEAT-002]
**As a** server operator managing multiple hosts
**I want** to share configuration modules across hosts
**So that** I don't duplicate manifest content for common patterns

**Acceptance Criteria:**
- [ ] A manifest can include another manifest file.
- [ ] Variables can be passed to the included manifest.
- [ ] Included manifests can include further manifests (nesting).
- [ ] `anneal validate` detects circular includes.

### US-006: Override variables per host [FEAT-002]
**As a** operator with multiple hosts sharing a base manifest
**I want** host-specific variable overrides
**So that** the same modules work across hosts with different IPs, hostnames, etc.

**Acceptance Criteria:**
- [ ] A host-specific variable file overrides module and root defaults.
- [ ] Environment variables override all manifest-level variables.
- [ ] The precedence chain is documented and deterministic.

### US-007: Iterate over a list [FEAT-002]
**As a** operator configuring multiple similar resources
**I want** to declare one resource with `each` and have it expand
**So that** I avoid repeating the same resource declaration

**Acceptance Criteria:**
- [ ] `each` expands a resource into multiple concrete resources.
- [ ] The iterator value is accessible in the resource's `name` and `spec`.
- [ ] Expansion happens before dependency resolution.

### US-008: Trigger a restart on config change [FEAT-002]
**As a** operator deploying a config file for a service
**I want** the service to restart only if the config actually changed
**So that** I avoid unnecessary service disruption

**Acceptance Criteria:**
- [ ] A resource with `notify: restart-x` marks the trigger as pending only when
  the resource changes.
- [ ] A trigger resource executes only if notified.
- [ ] Triggers run after all normal resources complete.

## Edge Cases and Error Handling

- An included manifest references a variable not passed by the including
  manifest and has no default: validate error with file and variable name.
- Two resources in different included manifests have the same name: validate
  error identifying both locations.
- `each` over an empty list: produces zero resources (no error).
- A trigger is declared but never notified: never executes (no warning needed).
- A `notify` references a resource that is not `trigger: true`: validate error.

## Success Metrics

- Manifest for the reference deployment (timbuktu/eldir) validates in < 1s.
- An operator can add a new host by creating a variable override file — no
  manifest duplication.
- Module inclusion reduces manifest size for the reference deployment by > 50%
  compared to a flat single-file manifest.

## Constraints and Assumptions

- The template expression language must be determined in the design phase. The
  requirement is expressiveness (conditionals, loops, functions), not a specific
  implementation.
- Module resolution is local filesystem only in v1 (no remote fetching).
- Manifest format is YAML — no alternative format support in v1.

## Dependencies

- Core engine (FEAT-001): consumes the parsed, expanded manifest.

## Out of Scope

- Module registry or remote module fetching (P2).
- Schema generation or manifest IDE support.
- Migration tooling from Ansible/Puppet/Chef manifests.
