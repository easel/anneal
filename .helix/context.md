# HELIX Execution Context

Project root: /home/erik/Projects/anneal
Tracker: /home/erik/Projects/anneal/.helix/issues.jsonl
Issue counts: ready=35, open=50, in-progress=1, closed=94

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

## Key Rules

- Implement all listed issues in one cycle. Claim each before work, close each after verification.
- Authority order: Vision > PRD > Specs > Architecture > Designs > Tests > Code
- Verify only changed crates/packages, not the full workspace.
- Absorb small adjacent work (<100 lines, same module) instead of creating tickets.
- Use helix tracker subcommands for all issue operations.
- Commit with issue ID in message. Push to remote.
