# Anneal

Declarative host configuration engine. One binary, executable plans, shell-extensible providers.

```
anneal validate   # Check manifest syntax, references, dependency cycles
anneal plan       # Read system state → produce executable plan script
anneal apply      # Re-validate plan → execute
```

**Status**: Design phase. See [Product Vision](docs/helix/00-discover/product-vision.md)
and [PRD](docs/helix/01-frame/prd.md).

## Design Principles

1. **Idempotent convergence** — same manifest twice = no changes the second time
2. **Plan is the artifact** — readable shell script, not an opaque diff
3. **Standard library is the contract** — built-in and custom providers emit the same ops
4. **Two-tier providers** — compiled core + shell scripts in embedded interpreter
5. **Manifests compose** — include, parameterize, share modules

See the full [design principles](docs/helix/00-discover/product-vision.md#design-principles).
