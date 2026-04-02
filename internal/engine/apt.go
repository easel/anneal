package engine

import (
	"fmt"
	"os"
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

// fileExistsFunc checks whether a file exists. Injectable for testing.
var fileExistsFunc = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readFileFunc reads a file. Injectable for testing.
var readFileFunc = os.ReadFile

// dpkgVersionFunc queries the installed version of a single package.
// Returns empty string if the package is not installed.
var dpkgVersionFunc = dpkgVersionReal

func dpkgVersionReal(name string) string {
	cmd := exec.Command("dpkg-query", "-W", "-f", "${Version}", name)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// aptRepoProvider manages external apt repository signing keys and sources entries.
type aptRepoProvider struct{}

func (aptRepoProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	keyURL, _ := resource.Spec["key_url"].(string)
	keyring, _ := resource.Spec["keyring"].(string)
	sourceLine, _ := resource.Spec["source"].(string)
	sourceFile, _ := resource.Spec["source_file"].(string)

	if sourceLine == "" {
		return nil, fmt.Errorf("apt_repo spec.source is required")
	}
	if sourceFile == "" {
		return nil, fmt.Errorf("apt_repo spec.source_file is required")
	}

	var ops []string

	// Check if signing key needs to be installed
	if keyURL != "" && keyring != "" {
		if !fileExistsFunc(keyring) {
			ops = append(ops, fmt.Sprintf("# add signing key → %s", keyring))
			ops = append(ops, fmt.Sprintf("stdlib_apt_key_add %s %s", shellQuote(keyURL), shellQuote(keyring)))
		}
	}

	// Check if sources file needs to be written
	needsSource := true
	if fileExistsFunc(sourceFile) {
		current, err := readFileFunc(sourceFile)
		if err == nil && strings.TrimSpace(string(current)) == strings.TrimSpace(sourceLine) {
			needsSource = false
		}
	}

	if needsSource {
		ops = append(ops, fmt.Sprintf("# add apt source → %s", sourceFile))
		ops = append(ops, fmt.Sprintf("stdlib_apt_source_add %s %s", shellQuote(sourceFile), shellQuote(sourceLine)))
	}

	return ops, nil
}

// debInstallProvider installs a .deb package from a URL with version tracking.
type debInstallProvider struct{}

func (debInstallProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	url, ok := resource.Spec["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("deb_install spec.url is required")
	}
	pkg, ok := resource.Spec["package"].(string)
	if !ok || pkg == "" {
		return nil, fmt.Errorf("deb_install spec.package is required")
	}
	version, ok := resource.Spec["version"].(string)
	if !ok || version == "" {
		return nil, fmt.Errorf("deb_install spec.version is required")
	}

	// Check if the package is already installed at the declared version
	installedVersion := dpkgVersionFunc(pkg)
	if installedVersion == version {
		return nil, nil // Already at declared version
	}

	var ops []string
	if installedVersion == "" {
		ops = append(ops, fmt.Sprintf("# install %s %s", pkg, version))
	} else {
		ops = append(ops, fmt.Sprintf("# upgrade %s: %s → %s", pkg, installedVersion, version))
	}
	ops = append(ops, fmt.Sprintf("stdlib_deb_install %s", shellQuote(url)))
	return ops, nil
}
