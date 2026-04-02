package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// systemctlStateFunc reads the enabled and active state of a unit.
// Returns (enabled, active). Injectable for testing.
var systemctlStateFunc = systemctlStateReal

func systemctlStateReal(unit string) (enabled string, active string) {
	enabledCmd := exec.Command("systemctl", "is-enabled", unit)
	enabledOut, _ := enabledCmd.Output()
	enabled = strings.TrimSpace(string(enabledOut))
	if enabled == "" {
		enabled = "unknown"
	}

	activeCmd := exec.Command("systemctl", "is-active", unit)
	activeOut, _ := activeCmd.Output()
	active = strings.TrimSpace(string(activeOut))
	if active == "" {
		active = "unknown"
	}

	return enabled, active
}

// systemdServiceProvider manages the enable/start/stop state of systemd services.
type systemdServiceProvider struct{}

func (systemdServiceProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	unit, ok := resource.Spec["unit"].(string)
	if !ok || unit == "" {
		return nil, fmt.Errorf("systemd_service spec.unit is required")
	}

	state, ok := resource.Spec["state"].(string)
	if !ok || state == "" {
		return nil, fmt.Errorf("systemd_service spec.state is required")
	}

	validStates := map[string]bool{"started": true, "stopped": true, "disabled": true, "masked": true}
	if !validStates[state] {
		return nil, fmt.Errorf("systemd_service spec.state must be one of: started, stopped, disabled, masked")
	}

	enabled, active := systemctlStateFunc(unit)
	var ops []string

	switch state {
	case "started":
		if enabled != "enabled" {
			ops = append(ops, fmt.Sprintf("# enable %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_enable %s", shellQuote(unit)))
		}
		if active != "active" {
			ops = append(ops, fmt.Sprintf("# start %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_start %s", shellQuote(unit)))
		}

	case "stopped":
		if enabled != "enabled" {
			ops = append(ops, fmt.Sprintf("# enable %s (stopped but enabled)", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_enable %s", shellQuote(unit)))
		}
		if active == "active" {
			ops = append(ops, fmt.Sprintf("# stop %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_stop %s", shellQuote(unit)))
		}

	case "disabled":
		if enabled == "enabled" {
			ops = append(ops, fmt.Sprintf("# disable %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_disable %s", shellQuote(unit)))
		}
		if active == "active" {
			ops = append(ops, fmt.Sprintf("# stop %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_stop %s", shellQuote(unit)))
		}

	case "masked":
		if enabled != "masked" {
			ops = append(ops, fmt.Sprintf("# mask %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_mask %s", shellQuote(unit)))
		}
		if active == "active" {
			ops = append(ops, fmt.Sprintf("# stop %s", unit))
			ops = append(ops, fmt.Sprintf("stdlib_service_stop %s", shellQuote(unit)))
		}
	}

	return ops, nil
}

// systemdUnitProvider writes unit files to /etc/systemd/system/ and triggers daemon-reload.
type systemdUnitProvider struct{}

func (systemdUnitProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	unitName, ok := resource.Spec["unit"].(string)
	if !ok || unitName == "" {
		return nil, fmt.Errorf("systemd_unit spec.unit is required")
	}
	content, ok := resource.Spec["content"].(string)
	if !ok {
		return nil, fmt.Errorf("systemd_unit spec.content is required")
	}

	path := "/etc/systemd/system/" + unitName

	// Check if unit file already has correct content
	current, err := os.ReadFile(path)
	if err == nil && string(current) == content {
		return nil, nil // Already converged
	}

	var ops []string
	if err != nil {
		ops = append(ops, fmt.Sprintf("# new unit %s", unitName))
	} else {
		ops = append(ops, fmt.Sprintf("# update unit %s", unitName))
	}

	delim := uniqueHeredocDelimiter(content)
	ops = append(ops, fmt.Sprintf("stdlib_file_write %s '0644' 'root:root' <<'%s'\n%s\n%s",
		shellQuote(path), delim, content, delim))
	ops = append(ops, "stdlib_daemon_reload")

	return ops, nil
}
