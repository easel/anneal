# Agent Instructions

Anneal is a declarative host configuration engine — a single binary that reads
a manifest, diffs it against system state, and produces an executable plan to
converge the machine. See [Product Vision](docs/helix/00-discover/product-vision.md).

This project uses the **built-in HELIX tracker** for issue tracking.
Issues are stored in `.helix/issues.jsonl`.

## Quick Reference

```bash
helix tracker ready           # Find available tracked work
helix tracker show <id>       # View issue details
helix tracker update <id> --claim  # Claim work
helix tracker close <id>      # Complete work
helix run                     # Run bounded HELIX execution loop
helix check                   # Decide next HELIX action
helix design                  # Create design document
helix review                  # Fresh-eyes review of recent work
```

## Artifact Stack

The HELIX artifact hierarchy lives under `docs/helix/`:

```
docs/helix/
  00-discover/product-vision.md          # Vision, principles, positioning
  01-frame/prd.md                        # Product requirements (P0/P1/P2)
  01-frame/features/FEAT-001..009        # Feature specifications with user stories
  02-design/solution-designs/SD-001      # Architecture and technology choices
```

Authority order: Vision → PRD → Feature Specs → Solution Designs → Tests → Code.

## Reference Deployments

Anneal's built-in providers are validated against two reference deployments:

- **timbuktu** (`~/Sync/timbuktu/`): server provisioning — ZFS, Kerberos, NFS,
  Samba, Docker, monitoring stack
- **sahara** (`~/Sync/sahara/`): workstation provisioning — apt, Homebrew,
  language runtimes, editors, Docker, security config

## Build & Test

```bash
go build -o anneal .           # Build the binary
go test ./...                  # Run all tests
go vet ./...                   # Static analysis
```

### CLI Commands

`anneal validate -f <manifest>` — parse and validate without system access
`anneal plan -f <manifest>` — build execution plan (stub); `-o <file>` writes to file
`anneal apply -f <manifest>` — apply plan (stub)
`anneal providers` — list available providers and their spec schemas
`anneal generate` — generate manifest fragments from structured input or goals
`anneal merge <base> <fragment>` — merge a manifest fragment into a base manifest
`anneal version` — print version

Default manifest path: `anneal.yaml`

Global flags: `--manifest/-f` (default `anneal.yaml`), `--host-vars` (host-specific
variable overrides file). The `validate`, `plan`, and `apply` commands accept `--json`
for machine-readable output.

### Test Strategy

- Unit tests per provider against mock system interface
- Config/manifest parsing tests (`internal/manifest/`)
- CLI integration tests (`internal/cli/`)
- Golden-file plan tests (known input → expected plan output)
- Docker-based integration tests across Linux distributions
