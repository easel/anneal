package engine

import (
	"fmt"
	"os"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// hostsEntryProvider manages /etc/hosts entries.
// Spec: ip (string, required), hostname (string, required), aliases ([]string, optional)
type hostsEntryProvider struct{}

func (hostsEntryProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	ip, ok := resource.Spec["ip"].(string)
	if !ok || ip == "" {
		return nil, fmt.Errorf("hosts_entry spec.ip is required")
	}
	hostname, ok := resource.Spec["hostname"].(string)
	if !ok || hostname == "" {
		return nil, fmt.Errorf("hosts_entry spec.hostname is required")
	}

	var aliases []string
	if rawAliases, ok := resource.Spec["aliases"]; ok {
		if aliasList, ok := rawAliases.([]any); ok {
			for _, a := range aliasList {
				if s, ok := a.(string); ok {
					aliases = append(aliases, s)
				}
			}
		}
	}

	// Build the desired hosts line.
	parts := []string{ip, hostname}
	parts = append(parts, aliases...)
	desiredLine := strings.Join(parts, "\t")

	// Check if the line already exists in /etc/hosts.
	hostsPath := "/etc/hosts"
	if override, ok := resource.Spec["_hosts_path"].(string); ok {
		hostsPath = override
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("hosts_entry: reading %s: %w", hostsPath, err)
	}

	// Check if the exact line already exists.
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == desiredLine {
			return nil, nil // Already converged
		}
	}

	// Remove any existing lines for this hostname, then append the desired line.
	return []string{
		fmt.Sprintf("# hosts_entry: %s -> %s", hostname, ip),
		fmt.Sprintf("stdlib_hosts_entry %s %s %s", shellQuote(hostsPath), shellQuote(hostname), shellQuote(desiredLine)),
	}, nil
}

// crypttabEntryProvider manages /etc/crypttab entries.
// Spec: name (string, required), device (string, required), keyfile (string, optional, default "none"), options (string, optional, default "luks")
type crypttabEntryProvider struct{}

func (crypttabEntryProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	name, ok := resource.Spec["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("crypttab_entry spec.name is required")
	}
	device, ok := resource.Spec["device"].(string)
	if !ok || device == "" {
		return nil, fmt.Errorf("crypttab_entry spec.device is required")
	}

	keyfile := "none"
	if kf, ok := resource.Spec["keyfile"].(string); ok && kf != "" {
		keyfile = kf
	}
	options := "luks"
	if opts, ok := resource.Spec["options"].(string); ok && opts != "" {
		options = opts
	}

	desiredLine := fmt.Sprintf("%s\t%s\t%s\t%s", name, device, keyfile, options)

	crypttabPath := "/etc/crypttab"
	if override, ok := resource.Spec["_crypttab_path"].(string); ok {
		crypttabPath = override
	}

	content, err := os.ReadFile(crypttabPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("crypttab_entry: reading %s: %w", crypttabPath, err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == desiredLine {
			return nil, nil // Already converged
		}
	}

	return []string{
		fmt.Sprintf("# crypttab_entry: %s", name),
		fmt.Sprintf("stdlib_crypttab_entry %s %s %s", shellQuote(crypttabPath), shellQuote(name), shellQuote(desiredLine)),
	}, nil
}

// binaryInstallProvider downloads and installs a binary.
// Spec: url (string, required), path (string, required), mode (string, optional, default "0755"), checksum (string, optional, "sha256:...")
type binaryInstallProvider struct{}

func (binaryInstallProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	url, ok := resource.Spec["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("binary_install spec.url is required")
	}
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("binary_install spec.path is required")
	}

	mode := "0755"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}

	checksum := ""
	if cs, ok := resource.Spec["checksum"].(string); ok {
		checksum = cs
	}

	// If the binary already exists and a checksum is specified, verify it.
	if _, err := os.Stat(path); err == nil {
		if checksum != "" {
			// Binary exists — check checksum to decide if it needs replacing.
			return []string{
				fmt.Sprintf("# binary_install: verify checksum for %s", path),
				fmt.Sprintf("stdlib_binary_install %s %s %s %s", shellQuote(url), shellQuote(path), shellQuote(mode), shellQuote(checksum)),
			}, nil
		}
		// Binary exists, no checksum to verify — converged.
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		return metadataOps(path, info, mode, "root:root"), nil
	}

	// Binary does not exist — install it.
	var ops []string
	ops = append(ops, fmt.Sprintf("# binary_install: %s", path))
	ops = append(ops, fmt.Sprintf("stdlib_binary_install %s %s %s %s", shellQuote(url), shellQuote(path), shellQuote(mode), shellQuote(checksum)))
	return ops, nil
}

// commandProvider runs an arbitrary command.
// Spec: command (string, required), creates (string, optional — only runs if creates path doesn't exist)
type commandProvider struct{}

func (commandProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	command, ok := resource.Spec["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command spec.command is required")
	}

	creates := ""
	if c, ok := resource.Spec["creates"].(string); ok {
		creates = c
	}

	// If creates is specified and the path exists, the command is already converged.
	if creates != "" {
		if _, err := os.Stat(creates); err == nil {
			return nil, nil // Already converged
		}
	}

	var ops []string
	ops = append(ops, fmt.Sprintf("# command: %s", resource.Name))
	if creates != "" {
		// Wrap in a guard so the command only runs if the creates path is absent.
		ops = append(ops, fmt.Sprintf("stdlib_command_creates %s %s", shellQuote(creates), shellQuote(command)))
	} else {
		ops = append(ops, command)
	}
	return ops, nil
}
