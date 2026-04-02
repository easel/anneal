---
dun:
  id: FEAT-015
  depends_on:
    - anneal.prd
    - FEAT-001
    - FEAT-002
---

# Feature Specification: FEAT-015 — Agentic Tool Interface

## Overview

Anneal exposes a structured CLI surface optimized for AI agent loops (Claude
Code, OpenCode, Codex, custom agents). Instead of requiring agents to author
YAML manifests, understand provider internals, or navigate config file layouts,
anneal provides commands that let agents discover capabilities, express intent,
and produce reviewable plans through structured input/output.

The goal: an operator says "use anneal to install spark on myhost" and the
agent works with anneal to resolve the providers, generate manifest fragments,
validate them, and produce a plan the operator can review before execution.

## Governing Requirements

- PRD P1-14: Agentic tool interface
- Vision Principle 10: Agent-native interface
- Vision Tertiary Persona: AI Agent

## User Stories

### US-040: Provider Discovery

**As** an AI agent,
**I want** to list available providers with structured metadata,
**so that** I can determine what anneal can manage without reading documentation.

**Acceptance Criteria:**
1. `anneal providers` outputs a list of registered providers.
2. `anneal providers --json` outputs structured JSON with provider name, kind,
   required spec fields, optional spec fields, and a one-line description.
3. `anneal providers <kind>` shows detailed info for a specific provider
   including spec schema, examples, and supported platforms.
4. Output includes both built-in and discovered custom providers.

### US-041: Manifest Generation from Intent

**As** an AI agent,
**I want** to generate manifest fragments from a structured resource description,
**so that** I can produce valid manifests without understanding YAML conventions.

**Acceptance Criteria:**
1. `anneal generate --kind file --spec '{"path":"/etc/motd","content":"hello"}'`
   emits a valid YAML manifest fragment for that resource.
2. `anneal generate --from-goal "install package nginx"` resolves the
   appropriate provider (apt on Debian, dnf on Fedora, etc.) and emits a
   manifest fragment. Provider resolution uses system facts when available.
3. Generated fragments include sensible defaults (mode, owner) and can be
   piped directly to `anneal validate` or appended to an existing manifest.
4. `anneal generate --json` accepts and emits JSON instead of YAML for
   agent-to-agent pipelines.
5. Invalid or ambiguous goals produce structured errors with suggestions.

### US-042: Structured Plan Output

**As** an AI agent,
**I want** plan output in structured JSON format,
**so that** I can parse resource-level status without scraping human-readable text.

**Acceptance Criteria:**
1. `anneal plan --json` outputs a JSON object with per-resource entries
   including: name, kind, status (changed/converged/error), operations list,
   and a human-readable diff summary.
2. The JSON schema is stable and documented.
3. Human-readable plan output remains the default (no `--json`).
4. `anneal apply --json` outputs structured apply results with per-resource
   status (applied/failed/skipped/converged).

### US-043: Structured Validation Output

**As** an AI agent,
**I want** validation errors as structured records,
**so that** I can fix manifest issues programmatically.

**Acceptance Criteria:**
1. `anneal validate --json` outputs a JSON array of validation results with
   fields: level (error/warning), resource (name or index), field, message.
2. Valid manifests produce `{"valid": true, "issues": []}`.
3. Exit code reflects validation status (0 = valid, 1 = errors).

### US-044: Composable Fragment Merging

**As** an AI agent,
**I want** to merge generated fragments with existing manifests,
**so that** I can add resources to a project's configuration incrementally.

**Acceptance Criteria:**
1. `anneal merge <base-manifest> <fragment>` produces a combined manifest
   with resources from both, preserving vars and existing resources.
2. Duplicate resource names produce an error (not silent overwrite).
3. Merged output is valid YAML that passes `anneal validate`.

## Dependencies

- Requires the core engine (FEAT-001) for validate/plan/apply.
- Requires the manifest system (FEAT-002) for YAML parsing and composition.
- Benefits from fact gathering (FEAT-010) for goal-based provider resolution.
- Provider discovery depends on the provider registry being introspectable.

## Design Constraints

- All agentic commands are regular CLI subcommands — no separate daemon, API
  server, or socket interface. Agents call anneal the same way they call git
  or cargo.
- JSON output uses the same data structures as internal representations —
  no separate "API model" that could diverge.
- `anneal generate --from-goal` is best-effort and may require agent
  refinement. It should fail clearly rather than generate wrong manifests.
- The agentic interface does not bypass validation, planning, or
  drift detection. It generates inputs to the existing pipeline.

## Out of Scope

- Natural language understanding beyond structured goal strings.
- Conversational multi-turn interaction (the agent manages conversation state).
- Remote execution or agent-to-host networking.
- MCP server protocol (may be added later as a thin wrapper over CLI commands).
