# HELIX Execution Context

Project root: /home/erik/Projects/anneal
Tracker: /home/erik/Projects/anneal/.helix/issues.jsonl
Issue counts: ready=1, open=42, in-progress=0, closed=79

## Build/Test Commands

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

## Current Epic

ID: hx-66f6a5fe
Title: FEAT-015: Agentic Tool Interface
Acceptance: anneal providers, anneal generate, anneal plan --json, anneal validate --json, and anneal merge commands exist and pass acceptance criteria from FEAT-015 user stories US-040 through US-044

## Key Rules

- Implement all listed issues in one cycle. Claim each before work, close each after verification.
- Authority order: Vision > PRD > Specs > Architecture > Designs > Tests > Code
- Verify only changed crates/packages, not the full workspace.
- Absorb small adjacent work (<100 lines, same module) instead of creating tickets.
- Use helix tracker subcommands for all issue operations.
- Commit with issue ID in message. Push to remote.
