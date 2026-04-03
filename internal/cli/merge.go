package cli

import (
	"fmt"

	"github.com/easel/anneal/internal/manifest"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newMergeCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "merge <base-manifest> <fragment>",
		Short: "Merge a manifest fragment into a base manifest",
		Long:  "Combines a base manifest with a fragment, preserving vars and existing resources. Errors on duplicate resource names.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			basePath := args[0]
			fragmentPath := args[1]

			base, err := manifest.Load(basePath)
			if err != nil {
				return fmt.Errorf("load base manifest: %w", err)
			}

			fragment, err := manifest.Load(fragmentPath)
			if err != nil {
				return fmt.Errorf("load fragment: %w", err)
			}

			merged, err := mergeManifests(base, fragment)
			if err != nil {
				return err
			}

			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), merged)
			}

			data, err := yaml.Marshal(merged)
			if err != nil {
				return fmt.Errorf("marshal YAML: %w", err)
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of YAML")
	return cmd
}

// mergeOutput is the serializable merged manifest for YAML/JSON output.
type mergeOutput struct {
	Vars      map[string]any    `yaml:"vars,omitempty" json:"vars,omitempty"`
	Resources []manifest.Resource `yaml:"resources" json:"resources"`
}

func mergeManifests(base, fragment *manifest.Manifest) (*mergeOutput, error) {
	// Check for duplicate resource names across base and fragment.
	baseNames := make(map[string]bool, len(base.Resources))
	for _, r := range base.Resources {
		baseNames[r.Name] = true
	}
	for _, r := range fragment.Resources {
		if baseNames[r.Name] {
			return nil, fmt.Errorf("duplicate resource name %q: exists in both base and fragment", r.Name)
		}
	}

	// Merge vars: start with base, overlay fragment vars.
	mergedVars := make(map[string]any)
	for k, v := range base.Vars {
		mergedVars[k] = v
	}
	for k, v := range fragment.Vars {
		mergedVars[k] = v
	}

	// Concatenate resources: base first, then fragment.
	mergedResources := make([]manifest.Resource, 0, len(base.Resources)+len(fragment.Resources))
	mergedResources = append(mergedResources, base.Resources...)
	mergedResources = append(mergedResources, fragment.Resources...)

	var vars map[string]any
	if len(mergedVars) > 0 {
		vars = mergedVars
	}

	return &mergeOutput{
		Vars:      vars,
		Resources: mergedResources,
	}, nil
}
