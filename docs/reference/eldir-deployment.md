# Reference Deployment: eldir (Server)

This document describes the reference server deployment that validates Anneal's
built-in providers. It corresponds to the current timbuktu shell scripts
managing eldir, a bare-metal ZFS home server running Ubuntu 24.04.

## Source

- **Current implementation**: `~/Sync/timbuktu/` (numbered shell scripts + Just)
- **Detailed spec**: `~/Sync/timbuktu/docs/timbuktu-v2-spec.md`

## Resource Inventory

### Packages (FEAT-003)
- Base system packages: curl, htop, tmux, vim, acl, rsync, etc.
- Kerberos packages: krb5-kdc, krb5-admin-server, krb5-config (with debconf)
- Samba packages: samba, samba-vfs-modules
- NFS packages: nfs-kernel-server
- iSCSI packages: targetcli-iscsi, open-iscsi
- Network manager purge: NetworkManager, NetworkManager-gnome

### Users & Access (FEAT-004)
- ZFS users: erik (admin, encrypted), leif (encrypted)
- Service users: printer (groups: labianca), garage (system)
- Shared group: labianca
- POSIX ACLs on shared directories (Games, Movies, Music, etc.)

### Files (FEAT-005)
- Templates: smb.conf, exports, krb5.conf, kdc.conf, kadm5.acl, bond.netdev,
  bond.network, member.network, nfs-idmapd.conf, nfs-exports, mktxp.conf,
  prometheus.yml, garage.toml, grafana-datasource.yaml
- Static files: initramfs-hook (contains ${DESTDIR} shell vars, NOT templates)
- Secret files: garage rpc_secret, admin_token, metrics_token

### Services (FEAT-006)
- systemd: smbd, nmbd, nfs-server, rpc-gssd, krb5-kdc, krb5-admin-server,
  node_exporter, samba_exporter
- systemd units (inline): garage.service, node_exporter.service,
  samba_exporter.service
- Docker containers: pihole, github-runner x2, prometheus, grafana, mktxp

### Storage (FEAT-007)
- Pool-level properties: compression=lz4, recordsize=1M, acltype=posixacl,
  xattr=sa, atime=off, sync=standard
- Per-dataset overrides:
  - Movies/Music/TV Shows: compression=off, special_small_blocks=0
  - LaBianca/Photos: compression=off
  - Scans: compression=zstd-8
  - garage: compression=zstd
  - monitoring/prometheus: recordsize=128K
  - monitoring/grafana: recordsize=16K
  - home/*/Projects: recordsize=128K
- Encrypted datasets: home/erik, home/leif (raw keyfiles in /etc/zfs/keys/)

### Authentication (FEAT-008)
- Kerberos realm: AZGAARD.NET
- Principals: nfs/eldir.azgaard.home, host/eldir.azgaard.home, erik/admin
- Keytab: /etc/krb5.keytab (nfs + host principals)
- KDC master password: secret (1Password)

### Network & System (FEAT-009)
- Hosts entry: server_ip → kdc_host
- Crypttab: LUKS root device with Clevis/Tang auto-unlock
- Binary installs: node_exporter, garage
- Triggers: restart-samba, restart-nmb, reload-exports, restart-nfs,
  apply-nfs-sysctl, rebuild-initramfs, restart-networkd

### Secrets

| Secret | Provider | Used by |
|--------|---------|---------|
| kdc_master_password | 1Password | kerberos_kdc |
| pihole_password | 1Password (optional) | docker_container (pihole) |
| github_runner_token | 1Password | docker_container (runners) |
| garage_rpc_secret | 1Password / generate | secret_file |
| garage_admin_token | 1Password / generate | secret_file |
| garage_metrics_token | 1Password / generate | secret_file |
| mktxp_password | 1Password | template_file (mktxp.conf) |
| grafana_admin_password | 1Password | docker_container (grafana) |

## Resource Count

~65 resources total (before iterator/composite expansion):
- 5 package resources
- 8 user/group/ACL resources
- 20+ file/template resources
- 12 service resources (systemd + Docker)
- 10 ZFS property resources
- 5 Kerberos resources
- 5 network/binary/trigger resources

## Out of Scope

These exist in timbuktu but are NOT managed by Anneal:
- `deploy/zfs-migrate.sh` — destructive one-time migration
- `deploy/kerberos-client.sh` — run on KDC to onboard a client
- `deploy/iscsi-client.sh` — prints client setup instructions
- `templates/cifs-overlay@.service` — client-side systemd template
- `templates/grafana-*-dashboard.json` — imported manually into Grafana
