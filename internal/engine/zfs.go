package engine

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// zfsDatasetExistsFunc checks whether a ZFS dataset exists.
// Injectable for testing.
var zfsDatasetExistsFunc = zfsDatasetExistsReal

func zfsDatasetExistsReal(name string) (bool, error) {
	cmd := exec.Command("zfs", "list", "-H", "-o", "name", name)
	err := cmd.Run()
	if err != nil {
		// zfs list exits non-zero when the dataset doesn't exist.
		return false, nil
	}
	return true, nil
}

// zfsGetPropertiesFunc reads properties for a dataset via zfs get.
// Returns a map of property name to value. Returns nil map if the dataset
// does not exist.
// Injectable for testing.
var zfsGetPropertiesFunc = zfsGetPropertiesReal

func zfsGetPropertiesReal(dataset string, properties []string) (map[string]string, error) {
	// Check dataset existence first.
	exists, err := zfsDatasetExistsFunc(dataset)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	propList := strings.Join(properties, ",")
	cmd := exec.Command("zfs", "get", "-H", "-o", "property,value", propList, dataset)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("zfs get %s %s: %w", propList, dataset, err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

// zfsDatasetProvider creates ZFS datasets with properties at creation time.
// Spec: name (string, required), properties (map[string]string, optional),
// encryption (string, optional), keylength (int, optional),
// keyformat (string, optional), keylocation (string, optional)
type zfsDatasetProvider struct{}

func (zfsDatasetProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	name, ok := resource.Spec["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("zfs_dataset spec.name is required")
	}

	exists, err := zfsDatasetExistsFunc(name)
	if err != nil {
		return nil, fmt.Errorf("zfs_dataset: checking %s: %w", name, err)
	}
	if exists {
		return nil, nil // Already exists — properties managed by zfs_properties
	}

	// Build create command with properties.
	var createArgs []string
	createArgs = append(createArgs, "zfs", "create", "-p")

	// Collect creation-time properties.
	props := make(map[string]string)

	if rawProps, ok := resource.Spec["properties"].(map[string]any); ok {
		for k, v := range rawProps {
			props[k] = fmt.Sprintf("%v", v)
		}
	}

	// Encryption-related fields override properties map entries.
	if enc, ok := resource.Spec["encryption"].(string); ok && enc != "" {
		props["encryption"] = enc
	}
	if kf, ok := resource.Spec["keyformat"].(string); ok && kf != "" {
		props["keyformat"] = kf
	}
	if kl, ok := resource.Spec["keylocation"].(string); ok && kl != "" {
		props["keylocation"] = kl
	}
	// keylength may come as int or float64 from YAML/JSON parsing.
	if klen, ok := resource.Spec["keylength"]; ok {
		switch v := klen.(type) {
		case int:
			props["keylength"] = fmt.Sprintf("%d", v)
		case float64:
			props["keylength"] = fmt.Sprintf("%d", int(v))
		case string:
			props["keylength"] = v
		}
	}

	// Sort property names for deterministic output.
	propNames := make([]string, 0, len(props))
	for k := range props {
		propNames = append(propNames, k)
	}
	sort.Strings(propNames)

	for _, k := range propNames {
		createArgs = append(createArgs, "-o", fmt.Sprintf("%s=%s", k, props[k]))
	}

	createArgs = append(createArgs, name)

	// Build the shell command string with proper quoting.
	var quotedArgs []string
	for _, arg := range createArgs {
		// Don't quote the zfs command itself or flags.
		if arg == "zfs" || arg == "create" || arg == "-p" || arg == "-o" {
			quotedArgs = append(quotedArgs, arg)
		} else {
			quotedArgs = append(quotedArgs, shellQuote(arg))
		}
	}

	return []string{
		fmt.Sprintf("# zfs_dataset: create %s", name),
		strings.Join(quotedArgs, " "),
	}, nil
}

// zfsPropertiesProvider manages properties on existing ZFS datasets.
// Spec: dataset (string) or datasets ([]string), properties (map, required),
// recursive (bool, optional)
type zfsPropertiesProvider struct{}

func (zfsPropertiesProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	// Determine target dataset(s).
	var datasets []string
	if ds, ok := resource.Spec["dataset"].(string); ok && ds != "" {
		datasets = append(datasets, ds)
	}
	if dsList, ok := resource.Spec["datasets"].([]any); ok {
		for _, d := range dsList {
			if s, ok := d.(string); ok && s != "" {
				datasets = append(datasets, s)
			}
		}
	}
	if len(datasets) == 0 {
		return nil, fmt.Errorf("zfs_properties spec.dataset or spec.datasets is required")
	}

	rawProps, ok := resource.Spec["properties"].(map[string]any)
	if !ok || len(rawProps) == 0 {
		return nil, fmt.Errorf("zfs_properties spec.properties is required")
	}

	// Build desired properties map.
	desired := make(map[string]string, len(rawProps))
	propNames := make([]string, 0, len(rawProps))
	for k, v := range rawProps {
		desired[k] = fmt.Sprintf("%v", v)
		propNames = append(propNames, k)
	}
	sort.Strings(propNames)

	recursive := false
	if r, ok := resource.Spec["recursive"].(bool); ok {
		recursive = r
	}

	// Properties that cannot be changed after dataset creation.
	immutableProps := map[string]bool{
		"encryption":   true,
		"volblocksize": true,
		"casesensitivity": true,
		"normalization":   true,
		"utf8only":        true,
	}

	var ops []string

	for _, dataset := range datasets {
		current, err := zfsGetPropertiesFunc(dataset, propNames)
		if err != nil {
			return nil, fmt.Errorf("zfs_properties: reading %s: %w", dataset, err)
		}
		if current == nil {
			// Dataset does not exist — warn and skip per FEAT-007.
			ops = append(ops, fmt.Sprintf("# zfs_properties: WARNING dataset %s does not exist, skipping", dataset))
			continue
		}

		for _, prop := range propNames {
			desiredVal := desired[prop]
			currentVal, exists := current[prop]

			if exists && currentVal == desiredVal {
				continue // Already converged
			}

			if immutableProps[prop] {
				ops = append(ops, fmt.Sprintf("# zfs_properties: WARNING %s=%s cannot be changed on existing dataset %s (current: %s)",
					prop, desiredVal, dataset, currentVal))
				continue
			}

			if exists {
				ops = append(ops, fmt.Sprintf("# zfs set %s: %s → %s on %s", prop, currentVal, desiredVal, dataset))
			} else {
				ops = append(ops, fmt.Sprintf("# zfs set %s=%s on %s", prop, desiredVal, dataset))
			}

			if recursive {
				ops = append(ops, fmt.Sprintf("zfs set -r %s %s",
					shellQuote(prop+"="+desiredVal), shellQuote(dataset)))
			} else {
				ops = append(ops, fmt.Sprintf("zfs set %s %s",
					shellQuote(prop+"="+desiredVal), shellQuote(dataset)))
			}
		}
	}

	if len(ops) == 0 {
		return nil, nil
	}
	return ops, nil
}
