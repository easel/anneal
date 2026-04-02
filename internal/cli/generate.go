package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// generateFragment represents a manifest fragment with a single resource.
type generateFragment struct {
	Resources []generateResource `yaml:"resources" json:"resources"`
}

type generateResource struct {
	Kind string         `yaml:"kind" json:"kind"`
	Name string         `yaml:"name" json:"name"`
	Spec map[string]any `yaml:"spec" json:"spec"`
}

func newGenerateCmd() *cobra.Command {
	var (
		kind       string
		spec       string
		fromGoal   string
		name       string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate manifest fragments from structured input or goals",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kind == "" && fromGoal == "" {
				return fmt.Errorf("either --kind with --spec or --from-goal is required")
			}
			if kind != "" && fromGoal != "" {
				return fmt.Errorf("--kind and --from-goal are mutually exclusive")
			}

			var fragment generateFragment

			if kind != "" {
				res, err := generateFromKindSpec(kind, name, spec)
				if err != nil {
					return err
				}
				fragment.Resources = []generateResource{res}
			} else {
				res, err := generateFromGoal(fromGoal, name)
				if err != nil {
					return err
				}
				fragment.Resources = []generateResource{res}
			}

			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), fragment)
			}

			data, err := yaml.Marshal(fragment)
			if err != nil {
				return fmt.Errorf("marshal YAML: %w", err)
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Provider kind for the resource")
	cmd.Flags().StringVar(&spec, "spec", "", "JSON spec for the resource")
	cmd.Flags().StringVar(&fromGoal, "from-goal", "", "Intent-based resource description (e.g., 'install nginx')")
	cmd.Flags().StringVar(&name, "name", "", "Resource name (auto-generated if omitted)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of YAML")

	return cmd
}

func generateFromKindSpec(kind, name, specJSON string) (generateResource, error) {
	if specJSON == "" {
		return generateResource{}, fmt.Errorf("--spec is required with --kind")
	}

	var specMap map[string]any
	if err := json.Unmarshal([]byte(specJSON), &specMap); err != nil {
		return generateResource{}, fmt.Errorf("invalid --spec JSON: %w", err)
	}

	if name == "" {
		name = inferName(kind, specMap)
	}

	// Apply sensible defaults based on kind.
	applyDefaults(kind, specMap)

	return generateResource{
		Kind: kind,
		Name: name,
		Spec: specMap,
	}, nil
}

func generateFromGoal(goal, name string) (generateResource, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return generateResource{}, fmt.Errorf("--from-goal cannot be empty")
	}

	// Parse "install <package>" pattern.
	installRe := regexp.MustCompile(`(?i)^install\s+(?:package\s+)?(.+)$`)
	if m := installRe.FindStringSubmatch(goal); m != nil {
		packages := strings.Fields(m[1])
		kind := detectPackageProvider()
		if name == "" {
			name = "install-" + packages[0]
		}
		spec := map[string]any{
			"packages": toAnySlice(packages),
		}
		return generateResource{
			Kind: kind,
			Name: name,
			Spec: spec,
		}, nil
	}

	// Parse "create file <path>" or "write file <path>" pattern.
	fileRe := regexp.MustCompile(`(?i)^(?:create|write)\s+file\s+(.+)$`)
	if m := fileRe.FindStringSubmatch(goal); m != nil {
		path := strings.TrimSpace(m[1])
		if name == "" {
			name = inferNameFromPath(path)
		}
		spec := map[string]any{
			"path":    path,
			"content": "",
		}
		applyDefaults("file", spec)
		return generateResource{
			Kind: "file",
			Name: name,
			Spec: spec,
		}, nil
	}

	// Parse "create directory <path>" or "ensure directory <path>" pattern.
	dirRe := regexp.MustCompile(`(?i)^(?:create|ensure)\s+(?:directory|dir)\s+(.+)$`)
	if m := dirRe.FindStringSubmatch(goal); m != nil {
		path := strings.TrimSpace(m[1])
		if name == "" {
			name = inferNameFromPath(path)
		}
		spec := map[string]any{
			"path": path,
		}
		applyDefaults("directory", spec)
		return generateResource{
			Kind: "directory",
			Name: name,
			Spec: spec,
		}, nil
	}

	return generateResource{}, fmt.Errorf("could not resolve goal: %q — try 'install <package>', 'create file <path>', or 'create directory <path>'", goal)
}

// detectPackageProvider returns the appropriate package provider kind
// based on the current OS. Injectable via osReleasePathOverride for testing.
var osReleasePath = "/etc/os-release"

func detectPackageProvider() string {
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return "apt_packages" // Default to apt if unknown
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "ID=") {
			id := strings.Trim(line[3:], `"`)
			switch id {
			case "fedora", "rhel", "centos", "rocky", "almalinux", "ol":
				return "dnf_packages"
			case "arch", "manjaro", "endeavouros":
				return "pacman_packages"
			default:
				return "apt_packages"
			}
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			idLike := strings.Trim(line[8:], `"`)
			if strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") {
				return "dnf_packages"
			}
			if strings.Contains(idLike, "arch") {
				return "pacman_packages"
			}
		}
	}
	return "apt_packages"
}

func applyDefaults(kind string, spec map[string]any) {
	switch kind {
	case "file", "template_file", "static_file", "file_copy":
		if _, ok := spec["mode"]; !ok {
			spec["mode"] = "0644"
		}
		if _, ok := spec["owner"]; !ok {
			spec["owner"] = "root:root"
		}
	case "directory":
		if _, ok := spec["mode"]; !ok {
			spec["mode"] = "0755"
		}
		if _, ok := spec["owner"]; !ok {
			spec["owner"] = "root:root"
		}
	case "kerberos_keytab":
		if _, ok := spec["mode"]; !ok {
			spec["mode"] = "0600"
		}
	}
}

func inferName(kind string, spec map[string]any) string {
	if path, ok := spec["path"].(string); ok {
		return inferNameFromPath(path)
	}
	if name, ok := spec["name"].(string); ok {
		return kind + "-" + sanitizeName(name)
	}
	return kind + "-resource"
}

func inferNameFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return sanitizeName(parts[i])
		}
	}
	return "resource"
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "resource"
	}
	return s
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
