# Reference Deployment: sahara (Workstation)

This document describes the reference workstation deployment that validates
Anneal's package management, file, and service providers across multiple OS
families. It corresponds to the current sahara provisioning scripts.

## Source

- **Current implementation**: `~/Sync/sahara/` (Makefile + numbered shell scripts)

## Resource Inventory

### Packages (FEAT-003)
- **System packages** (apt/dnf): ~75 packages from Ubuntufile/Fedfile — core
  tools, dev libraries, networking, compression, build essentials
- **Homebrew**: ~120 formulae/casks — language runtimes (node, python, ruby,
  rust), cloud tools (kubectl, helm, terraform, aws-cli), dev tools (aider,
  codex, neovim), security tools (checkov, semgrep, trivy)
- **External apt repos**: Tailscale, 1Password CLI
- **Global npm packages**: typescript, prettier, etc.
- **Global pip packages**: CLI tools
- **Editors**: Claude Code (curl install), Zed (curl install)

### Users & Access (FEAT-004)
- Sudoers entry for the provisioning user
- PAM configuration (pam_rssh for SSH-agent-based sudo)

### Files (FEAT-005)
- Font installation (Nerd Fonts to ~/.local/share/fonts or system fonts dir)
- Editor config files
- User config (11-user-config.sh)

### Services (FEAT-006)
- Docker installation and daemon configuration
- systemd services enabled by sahara

### Network & System (FEAT-009)
- Binary installs: editors and tools not in package repos

## Multi-OS Support

sahara provisions across OS families:

| OS | Package source | Script |
|----|---------------|--------|
| Ubuntu/Debian | Ubuntufile (apt) | 00-system-packages.sh |
| Fedora/RHEL | Fedfile (dnf) | 00-system-packages.sh |
| Arch | Archfile (pacman) | 00-system-packages.sh |
| macOS | Brewfile only | 02-homebrew.sh |

This validates Anneal's multi-package-manager provider support (FEAT-003).

## Resource Count

~30 resources (before iterator expansion over package lists):
- 8+ package resources (apt, dnf, brew, npm, pip, apt_repo, deb_install)
- 3 user/access resources
- 5+ file resources (fonts, configs)
- 3 service resources
- 5+ binary install resources

## Key Differences from eldir

| Aspect | eldir (server) | sahara (workstation) |
|--------|---------------|---------------------|
| Package managers | apt only | apt + dnf + brew + npm + pip |
| Storage | ZFS (heavy) | None |
| Auth | Kerberos | PAM/sudoers only |
| Containers | Docker (6 containers) | Docker (daemon only) |
| Network | Bond, NFS, Samba | None |
| Users | Multiple with encrypted home | Single user |
| Secrets | 8 secrets (1Password) | Minimal |
| Primary validation | Storage, auth, services | Multi-OS packages, files |
