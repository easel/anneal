---
dun:
  id: anneal.vision
---
# Product Vision: Anneal

Anneal is a declarative host configuration engine. One static binary reads a
manifest, diffs it against actual system state, and produces an executable plan
that converges the machine — Terraform's plan/apply workflow, applied to
OS-level configuration.

The name comes from metallurgical annealing: heating metal and cooling it under
controlled conditions to reach a stable, desired crystalline state. Anneal does
the same for machines — converging chaotic system state into a declared target,
idempotently.

## Problem

Configuration management for individual servers is stuck between shell scripts
that are easy to start and hard to maintain, and enterprise CM tools that bring
more machinery than the problem warrants. Neither gives you a plan/apply
workflow for a single machine with zero runtime dependencies.

## Target Users

**Primary — solo operator / home lab admin.** Manages a handful of bare-metal
or VM hosts. Wants declarative configuration with preview but doesn't want to
install Ansible or run a Puppet server.

**Secondary — small team with dedicated infra hosts.** A team with a few "pet"
servers alongside cloud infrastructure. Already uses Terraform for cloud but has
nothing equivalent for the machines themselves.

## Positioning

### What Anneal Is Not

- Not a fleet orchestrator — no SSH, no push model, no inventory files.
- Not a cloud provisioner — starts where Terraform and cloud-init leave off.
- Not a package manager — orchestrates packages, users, files, services,
  containers, storage, and more.
- Not a framework — shell providers are the extension point; if you can write a
  shell script, you can write an Anneal provider.

### Adjacent Tools

- **Nix/NixOS**: replaces the entire OS packaging model. All-or-nothing.
- **cloud-init**: first-boot only. No convergence, no plan/apply.
- **mgmt**: reactive execution model vs Anneal's convergent approach.
- **Ignition/Butane**: tied to immutable OS images. First-boot only.
- **Ansible**: agentless but requires Python. Check mode is partial and opt-in.

## Design Principles

1. **Idempotent convergence.** Every run reads actual system state and converges
   toward the declared state. Applying the same manifest twice produces no
   changes the second time.
2. **Plan is the artifact.** The plan is an executable script composed of
   standard library operations. What you review is what will run. A converged
   system produces an empty plan.
3. **Standard library is the contract.** Both built-in and custom providers emit
   the same stdlib operations. The stdlib is what makes plans readable and
   providers interchangeable.
4. **Apply validates before executing.** Re-reads state, confirms the manifest
   still produces the same plan. Material drift aborts; immaterial drift does
   not.
5. **Fail-stop.** Stop on first error. Log every mutation with enough context to
   diagnose. No magic rollback.
6. **Two-tier providers.** Core providers built in for performance. Custom
   providers as shell scripts in an embedded interpreter. Same contract, same
   stdlib output, no dependency on the host's shell.
7. **Manifests compose.** Reusable modules can be parameterized, published,
   shared, and included — like Terraform modules for OS-level configuration.
8. **Templates are code.** Conditionals, loops, functions — not just variable
   substitution.
9. **Secrets stay in the vault.** Referenced by name, never stored in manifests,
   never shown in plans. Environment variables and 1Password built in; custom
   secret providers via shell scripts.
