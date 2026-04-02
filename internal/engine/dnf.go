package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// rpmQueryFunc checks installed state via rpm. Injectable for testing.
var rpmQueryFunc = rpmInstalledReal

func rpmInstalledReal(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return map[string]bool{}, nil
	}
	args := append([]string{"-q", "--qf", "%{NAME}\n"}, names...)
	cmd := exec.Command("rpm", args...)
	out, _ := cmd.Output()
	// rpm returns exit 1 if any package is not installed — expected.

	installed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "package ") {
			// "package X is not installed" lines are filtered out
			installed[line] = true
		}
	}
	return installed, nil
}

// dnfPackagesProvider installs missing RPM packages via dnf.
type dnfPackagesProvider struct{}

func (dnfPackagesProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	rawPkgs, ok := resource.Spec["packages"]
	if !ok {
		return nil, fmt.Errorf("dnf_packages spec.packages is required")
	}
	pkgList, ok := rawPkgs.([]any)
	if !ok {
		return nil, fmt.Errorf("dnf_packages spec.packages must be a list")
	}

	var names []string
	for _, p := range pkgList {
		name, ok := p.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("dnf_packages: package names must be non-empty strings")
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	installed, err := rpmQueryFunc(names)
	if err != nil {
		return nil, fmt.Errorf("dnf_packages: checking installed state: %w", err)
	}

	var missing []string
	for _, name := range names {
		if !installed[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return nil, nil
	}

	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = shellQuote(m)
	}
	return []string{
		fmt.Sprintf("# install %d package(s)", len(missing)),
		fmt.Sprintf("stdlib_dnf_install %s", strings.Join(quoted, " ")),
	}, nil
}
