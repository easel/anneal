package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// dpkgQueryFunc is the function used to check installed package state.
// It is a variable so tests can replace it without requiring dpkg.
var dpkgQueryFunc = dpkgInstalledReal

// dpkgInstalledReal queries the dpkg database for installed package state.
func dpkgInstalledReal(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return map[string]bool{}, nil
	}
	args := append([]string{"-W", "-f", "${Status} ${Package}\n"}, names...)
	cmd := exec.Command("dpkg-query", args...)
	out, _ := cmd.Output()
	// dpkg-query returns exit 1 if any package is unknown — that's expected.
	// We parse stdout for packages that ARE installed.

	installed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "install ok installed <package>"
		if strings.HasPrefix(line, "install ok installed ") {
			pkg := strings.TrimPrefix(line, "install ok installed ")
			installed[pkg] = true
		}
	}
	return installed, nil
}

// aptPackagesProvider installs missing Debian packages in batch.
type aptPackagesProvider struct{}

func (aptPackagesProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	rawPkgs, ok := resource.Spec["packages"]
	if !ok {
		return nil, fmt.Errorf("apt_packages spec.packages is required")
	}
	pkgList, ok := rawPkgs.([]any)
	if !ok {
		return nil, fmt.Errorf("apt_packages spec.packages must be a list")
	}

	var names []string
	for _, p := range pkgList {
		name, ok := p.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("apt_packages: package names must be non-empty strings")
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	installed, err := dpkgQueryFunc(names)
	if err != nil {
		return nil, fmt.Errorf("apt_packages: checking installed state: %w", err)
	}

	var missing []string
	for _, name := range names {
		if !installed[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return nil, nil // All packages already installed
	}

	var ops []string

	// Handle debconf preseeding before install
	if rawDebconf, ok := resource.Spec["debconf"]; ok {
		debconfList, ok := rawDebconf.([]any)
		if !ok {
			return nil, fmt.Errorf("apt_packages spec.debconf must be a list")
		}
		for _, entry := range debconfList {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("apt_packages spec.debconf entries must be objects")
			}
			pkg, _ := entryMap["package"].(string)
			question, _ := entryMap["question"].(string)
			vtype, _ := entryMap["type"].(string)
			value, _ := entryMap["value"].(string)
			if pkg == "" || question == "" || vtype == "" {
				return nil, fmt.Errorf("apt_packages debconf entries require package, question, and type")
			}
			preseed := fmt.Sprintf("%s %s %s %s", pkg, question, vtype, value)
			ops = append(ops, fmt.Sprintf("stdlib_debconf_set %s", shellQuote(preseed)))
		}
	}

	// Batch all missing packages into one install call
	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = shellQuote(m)
	}
	ops = append(ops, fmt.Sprintf("# install %d package(s)", len(missing)))
	ops = append(ops, fmt.Sprintf("stdlib_apt_install %s", strings.Join(quoted, " ")))

	return ops, nil
}

// aptPurgeProvider removes installed Debian packages.
type aptPurgeProvider struct{}

func (aptPurgeProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	rawPkgs, ok := resource.Spec["packages"]
	if !ok {
		return nil, fmt.Errorf("apt_purge spec.packages is required")
	}
	pkgList, ok := rawPkgs.([]any)
	if !ok {
		return nil, fmt.Errorf("apt_purge spec.packages must be a list")
	}

	var names []string
	for _, p := range pkgList {
		name, ok := p.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("apt_purge: package names must be non-empty strings")
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	installed, err := dpkgQueryFunc(names)
	if err != nil {
		return nil, fmt.Errorf("apt_purge: checking installed state: %w", err)
	}

	var present []string
	for _, name := range names {
		if installed[name] {
			present = append(present, name)
		}
	}

	if len(present) == 0 {
		return nil, nil // All packages already absent
	}

	quoted := make([]string, len(present))
	for i, p := range present {
		quoted[i] = shellQuote(p)
	}
	return []string{
		fmt.Sprintf("# purge %d package(s)", len(present)),
		fmt.Sprintf("stdlib_apt_purge %s", strings.Join(quoted, " ")),
	}, nil
}
