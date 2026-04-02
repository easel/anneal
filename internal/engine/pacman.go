package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// pacmanQueryFunc checks installed state via pacman. Injectable for testing.
var pacmanQueryFunc = pacmanInstalledReal

func pacmanInstalledReal(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return map[string]bool{}, nil
	}
	// pacman -Qq lists installed package names, one per line.
	// We query all installed then filter, because pacman -Qi exits 1
	// if any package is missing.
	cmd := exec.Command("pacman", "-Qq")
	out, err := cmd.Output()
	if err != nil {
		return map[string]bool{}, nil
	}
	allInstalled := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			allInstalled[line] = true
		}
	}
	result := map[string]bool{}
	for _, name := range names {
		if allInstalled[name] {
			result[name] = true
		}
	}
	return result, nil
}

// pacmanPackagesProvider installs missing Arch Linux packages.
// Supports AUR packages via a configured AUR helper (paru/yay).
type pacmanPackagesProvider struct{}

func (pacmanPackagesProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	rawPkgs, ok := resource.Spec["packages"]
	if !ok {
		return nil, fmt.Errorf("pacman_packages spec.packages is required")
	}
	pkgList, ok := rawPkgs.([]any)
	if !ok {
		return nil, fmt.Errorf("pacman_packages spec.packages must be a list")
	}

	var names []string
	for _, p := range pkgList {
		name, ok := p.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("pacman_packages: package names must be non-empty strings")
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	installed, err := pacmanQueryFunc(names)
	if err != nil {
		return nil, fmt.Errorf("pacman_packages: checking installed state: %w", err)
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

	// Check for AUR helper configuration
	aurHelper, _ := resource.Spec["aur_helper"].(string)
	aurUser, _ := resource.Spec["user"].(string)

	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = shellQuote(m)
	}

	if aurHelper != "" {
		// AUR packages — run through helper as unprivileged user
		if aurUser == "" {
			return nil, fmt.Errorf("pacman_packages spec.user is required when using aur_helper")
		}
		return []string{
			fmt.Sprintf("# install %d AUR package(s) via %s as %s", len(missing), aurHelper, aurUser),
			fmt.Sprintf("stdlib_aur_install %s %s %s", shellQuote(aurUser), shellQuote(aurHelper), strings.Join(quoted, " ")),
		}, nil
	}

	return []string{
		fmt.Sprintf("# install %d package(s)", len(missing)),
		fmt.Sprintf("stdlib_pacman_install %s", strings.Join(quoted, " ")),
	}, nil
}
