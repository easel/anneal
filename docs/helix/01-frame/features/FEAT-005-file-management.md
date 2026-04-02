---
dun:
  id: FEAT-005
  depends_on:
    - anneal.prd
    - FEAT-001
---
# Feature Specification: FEAT-005 - File Management

**Feature ID**: FEAT-005
**Status**: Specified
**Priority**: P0
**Owner**: anneal maintainers

## Overview

File management providers handle the most common operation in host
configuration: putting the right content in the right file with the right
permissions. This feature covers inline files, templates, static copies,
directories, symlinks, file removal, and secret files.

## Problem Statement

- **Current situation**: timbuktu uses `ensure_file`, `render_template`,
  `ensure_dir` helpers. sahara uses the same plus symlink management for
  dotfiles. Templates use envsubst.
- **Pain points**: No diff preview of file changes. envsubst can't do
  conditionals or loops. No distinction between template files and static files
  that happen to contain `${}` (like shell scripts).
- **Desired outcome**: Every file operation is a resource with plan output
  showing the exact diff. Templates support real logic. Static files are copied
  verbatim without accidental template processing.

## Requirements

### Functional Requirements

- File providers must show content diffs in the plan for changed files.
- Template files use the manifest's template expression language (conditionals,
  loops, functions).
- Static files are copied verbatim — no template processing, safe for files
  containing `${VAR}` as literal content (shell scripts, etc.).
- Secret files resolve their content from the secret provider chain at apply
  time, with `(secret)` in plan output.
- File removal supports both explicit paths and glob patterns.
- Template files support an optional `validate` command that runs against the
  rendered content before writing (e.g., `nginx -t`, `testparm -s`).

### Non-Functional Requirements

- **Security**: secret file content never appears in plan output or logs.
- **Performance**: file reads use stat + content hash to detect changes without
  reading large files fully when unnecessary.

## User Stories

### US-030: Write a file with inline content [FEAT-005]
**As a** operator
**I want** to declare a file's content directly in the manifest
**So that** small config files don't need separate template files

**Acceptance Criteria:**
- [ ] `file` provider compares declared content to current file.
- [ ] Changed files produce `stdlib_file_write` with the full content.
- [ ] Plan output shows a diff of current vs desired content.
- [ ] Mode and owner are enforced.

### US-031: Render a template to a file [FEAT-005]
**As a** operator
**I want** to render a template with variables to a config file
**So that** host-specific config is generated from a shared template

**Acceptance Criteria:**
- [ ] `template_file` provider renders the template with manifest variables.
- [ ] Rendered content is compared to current file.
- [ ] Plan output shows the diff.
- [ ] Optional `validate` command runs against rendered content before plan
  reports the change as valid.

### US-032: Copy a static file [FEAT-005]
**As a** operator deploying a file that contains shell variable syntax
**I want** the file copied verbatim without template processing
**So that** `${DESTDIR}` in an initramfs hook is not misinterpreted

**Acceptance Criteria:**
- [ ] `static_file` provider copies source file without template evaluation.
- [ ] Content comparison and diff work the same as other file providers.

### US-033: Create directories [FEAT-005]
**As a** operator
**I want** to declare directories with ownership and permissions
**So that** parent directories exist before files are written to them

**Acceptance Criteria:**
- [ ] `directory` provider checks existence, mode, and owner.
- [ ] Missing directories produce `stdlib_dir_create` operations.
- [ ] Incorrect mode/owner on existing directories produce chmod/chown ops.

### US-034: Create and manage symlinks [FEAT-005]
**As a** workstation operator managing dotfiles
**I want** to declare symlinks
**So that** config files point to version-controlled sources

**Acceptance Criteria:**
- [ ] `symlink` provider checks if the link exists and points to the right
  target.
- [ ] Wrong target or missing link produces a create/update operation.
- [ ] Broken symlinks (target doesn't exist) produce a warning, not an error.

### US-035: Remove files [FEAT-005]
**As a** operator cleaning up legacy config
**I want** to declare files that should be absent
**So that** old config files are removed during convergence

**Acceptance Criteria:**
- [ ] `file_absent` provider supports both explicit path list and glob pattern.
- [ ] Present files produce `stdlib_file_remove` operations.
- [ ] Already-absent files produce no operations.

### US-036: Write secret files [FEAT-005]
**As a** operator deploying files containing credentials
**I want** to declare a secret file that resolves from 1Password
**So that** credentials are deployed without storing them in my manifest

**Acceptance Criteria:**
- [ ] `secret_file` provider resolves content from the secret provider chain.
- [ ] Plan output shows `(secret)` instead of the actual content.
- [ ] If the file exists, no-op (content comparison is skipped for secret files
  to avoid reading secrets into plan output).
- [ ] If `generate` is specified and no provider resolves, the generate command
  runs and the operator is warned.
- [ ] Mode and owner are enforced (secret files should be 0600 or similar).

### US-037: Copy files between paths [FEAT-005]
**As a** operator
**I want** to copy a file from one location to another
**So that** config files or tokens are placed where services expect them

**Acceptance Criteria:**
- [ ] `file_copy` provider compares source and destination content.
- [ ] Changed content produces `stdlib_file_copy` operations.
- [ ] Mode is enforced on the destination.

## Edge Cases and Error Handling

- Template references an undefined variable: fail at validate time.
- Template validate command fails: fail at plan time, report the validation
  error. Do not include the change in the plan.
- File_absent glob matches zero files: no operations, no error.
- Directory already exists with wrong owner: plan shows chown operation.
- Symlink target doesn't exist: warn (the target might be created by a later
  resource), do not error.
- Secret file generate command fails: fail at apply time with clear error.

## Dependencies

- Core engine (FEAT-001): provider contract, stdlib, secret provider chain.
- Manifest system (FEAT-002): template expression evaluation.

## Out of Scope

- Binary file diff (show hash only, not content diff).
- File content patching (sed-style in-place edits).
- Recursive directory management (manage individual files, not directory trees).
