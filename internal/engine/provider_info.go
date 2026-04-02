package engine

import "sort"

// ProviderInfo describes a registered provider's metadata for discovery.
type ProviderInfo struct {
	Kind           string   `json:"kind"`
	Description    string   `json:"description"`
	RequiredFields []string `json:"required_fields"`
	OptionalFields []string `json:"optional_fields,omitempty"`
}

// ProviderRegistry returns metadata for all registered providers, sorted by kind.
func ProviderRegistry() []ProviderInfo {
	infos := []ProviderInfo{
		{Kind: "file", Description: "Write inline content to a file", RequiredFields: []string{"path", "content"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "template_file", Description: "Render a Go template source file with manifest variables", RequiredFields: []string{"source", "path"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "static_file", Description: "Copy a source file verbatim without template processing", RequiredFields: []string{"source", "path"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "file_copy", Description: "Copy a file from source to destination", RequiredFields: []string{"source", "path"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "directory", Description: "Ensure a directory exists with correct mode and owner", RequiredFields: []string{"path"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "symlink", Description: "Ensure a symlink exists pointing to the correct target", RequiredFields: []string{"path", "target"}},
		{Kind: "file_absent", Description: "Ensure files are removed", OptionalFields: []string{"path", "pattern"}},
		{Kind: "apt_packages", Description: "Install Debian/Ubuntu packages via apt", RequiredFields: []string{"packages"}},
		{Kind: "apt_purge", Description: "Remove Debian/Ubuntu packages via apt purge", RequiredFields: []string{"packages"}},
		{Kind: "apt_repo", Description: "Add an APT repository with signing key", RequiredFields: []string{"name", "source_line", "key_url", "keyring_path"}},
		{Kind: "deb_install", Description: "Install a .deb package from a URL", RequiredFields: []string{"url"}},
		{Kind: "brew_packages", Description: "Install macOS/Linux Homebrew packages", RequiredFields: []string{"packages", "user"}},
		{Kind: "brew_tap", Description: "Add a Homebrew tap", RequiredFields: []string{"tap", "user"}},
		{Kind: "dnf_packages", Description: "Install Fedora/RHEL packages via dnf", RequiredFields: []string{"packages"}},
		{Kind: "pacman_packages", Description: "Install Arch Linux packages via pacman", RequiredFields: []string{"packages"}},
		{Kind: "user", Description: "Create or modify a system user", RequiredFields: []string{"name"}, OptionalFields: []string{"uid", "gid", "home", "shell", "system", "groups"}},
		{Kind: "group", Description: "Create a system group", RequiredFields: []string{"name"}, OptionalFields: []string{"gid", "system"}},
		{Kind: "user_in_group", Description: "Ensure a user belongs to a group", RequiredFields: []string{"user", "group"}},
		{Kind: "posix_acl", Description: "Set POSIX ACLs on files or directories", RequiredFields: []string{"path", "entries"}},
		{Kind: "sudoers_entry", Description: "Write a validated sudoers fragment", RequiredFields: []string{"name", "content"}},
		{Kind: "systemd_service", Description: "Manage the enable/start/stop state of systemd services", RequiredFields: []string{"name"}, OptionalFields: []string{"enabled", "state", "masked"}},
		{Kind: "systemd_unit", Description: "Write a systemd unit file and trigger daemon-reload", RequiredFields: []string{"name", "content"}, OptionalFields: []string{"mode", "owner"}},
		{Kind: "docker_container", Description: "Manage Docker container lifecycle", RequiredFields: []string{"name", "image"}, OptionalFields: []string{"ports", "volumes", "env", "network_mode", "restart_policy", "args", "health_check_url"}},
		{Kind: "hosts_entry", Description: "Manage /etc/hosts entries", RequiredFields: []string{"ip", "hostname"}, OptionalFields: []string{"aliases"}},
		{Kind: "crypttab_entry", Description: "Manage /etc/crypttab entries", RequiredFields: []string{"name", "device"}, OptionalFields: []string{"keyfile", "options"}},
		{Kind: "binary_install", Description: "Download and install a binary", RequiredFields: []string{"url", "path"}, OptionalFields: []string{"mode", "checksum"}},
		{Kind: "command", Description: "Run an arbitrary command", RequiredFields: []string{"command"}, OptionalFields: []string{"creates"}},
		{Kind: "zfs_dataset", Description: "Create ZFS datasets with properties at creation time", RequiredFields: []string{"name"}, OptionalFields: []string{"properties", "encryption", "keyformat", "keylocation", "keylength"}},
		{Kind: "zfs_properties", Description: "Manage properties on existing ZFS datasets", RequiredFields: []string{"properties"}, OptionalFields: []string{"dataset", "datasets", "recursive"}},
		{Kind: "kerberos_kdc", Description: "Initialize a Kerberos KDC database", RequiredFields: []string{"realm", "master_password"}, OptionalFields: []string{"db_path"}},
		{Kind: "kerberos_principal", Description: "Create Kerberos principals idempotently", RequiredFields: []string{"principal"}, OptionalFields: []string{"randkey", "password"}},
		{Kind: "kerberos_keytab", Description: "Export Kerberos principals to a keytab file", RequiredFields: []string{"path", "principals"}, OptionalFields: []string{"mode"}},
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Kind < infos[j].Kind
	})
	return infos
}

// ProviderKinds returns sorted list of registered provider kind names.
func ProviderKinds() []string {
	planner := NewPlanner()
	kinds := make([]string, 0, len(planner.providers))
	for kind := range planner.providers {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}
