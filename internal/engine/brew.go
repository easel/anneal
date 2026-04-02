package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// brewListFunc returns the set of installed formulae or casks.
// Injectable for testing without requiring Homebrew.
var brewListFunc = brewListReal

// brewTapListFunc returns the set of currently tapped repos.
// Injectable for testing without requiring Homebrew.
var brewTapListFunc = brewTapListReal

func brewListReal(user string, cask bool) (map[string]bool, error) {
	args := []string{"-u", user, "brew", "list", "-1"}
	if cask {
		args = append(args, "--cask")
	} else {
		args = append(args, "--formula")
	}
	cmd := exec.Command("sudo", args...)
	out, err := cmd.Output()
	if err != nil {
		// brew not installed or no packages — treat as empty
		return map[string]bool{}, nil
	}
	installed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			installed[line] = true
		}
	}
	return installed, nil
}

func brewTapListReal(user string) (map[string]bool, error) {
	cmd := exec.Command("sudo", "-u", user, "brew", "tap")
	out, err := cmd.Output()
	if err != nil {
		return map[string]bool{}, nil
	}
	taps := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			taps[line] = true
		}
	}
	return taps, nil
}

// brewPackagesProvider installs missing Homebrew formulae and casks.
type brewPackagesProvider struct{}

func (brewPackagesProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	user, _ := resource.Spec["user"].(string)
	if user == "" {
		return nil, fmt.Errorf("brew_packages spec.user is required (brew must not run as root)")
	}

	cask := false
	if rawCask, ok := resource.Spec["cask"].(bool); ok {
		cask = rawCask
	}

	rawPkgs, ok := resource.Spec["packages"]
	if !ok {
		return nil, fmt.Errorf("brew_packages spec.packages is required")
	}
	pkgList, ok := rawPkgs.([]any)
	if !ok {
		return nil, fmt.Errorf("brew_packages spec.packages must be a list")
	}

	var names []string
	for _, p := range pkgList {
		name, ok := p.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("brew_packages: package names must be non-empty strings")
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	installed, err := brewListFunc(user, cask)
	if err != nil {
		return nil, fmt.Errorf("brew_packages: checking installed state: %w", err)
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

	var ops []string
	pkgType := "formula(e)"
	if cask {
		pkgType = "cask(s)"
	}
	ops = append(ops, fmt.Sprintf("# install %d %s as %s", len(missing), pkgType, user))

	installArgs := shellQuote(user) + " " + strings.Join(quoted, " ")
	if cask {
		installArgs += " --cask"
	}
	ops = append(ops, fmt.Sprintf("stdlib_brew_install %s", installArgs))

	return ops, nil
}

// brewTapProvider manages Homebrew taps.
type brewTapProvider struct{}

func (brewTapProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	user, _ := resource.Spec["user"].(string)
	if user == "" {
		return nil, fmt.Errorf("brew_tap spec.user is required (brew must not run as root)")
	}

	tap, _ := resource.Spec["tap"].(string)
	if tap == "" {
		return nil, fmt.Errorf("brew_tap spec.tap is required")
	}

	taps, err := brewTapListFunc(user)
	if err != nil {
		return nil, fmt.Errorf("brew_tap: checking tapped repos: %w", err)
	}

	if taps[tap] {
		return nil, nil // Already tapped
	}

	return []string{
		fmt.Sprintf("# tap %s as %s", tap, user),
		fmt.Sprintf("stdlib_brew_tap %s %s", shellQuote(user), shellQuote(tap)),
	}, nil
}
