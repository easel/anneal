package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Vars      map[string]any `yaml:"vars"`
	Resources []Resource     `yaml:"resources"`
}

type Resource struct {
	Kind string         `yaml:"kind"`
	Name string         `yaml:"name"`
	Spec map[string]any `yaml:"spec"`
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}
	if err := ensureSingleDocument(decoder, path); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}

	return &manifest, nil
}

func ensureSingleDocument(decoder *yaml.Decoder, path string) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("load manifest %s: %w", path, err)
	}
	return fmt.Errorf("load manifest %s: multiple YAML documents are not supported", path)
}

func (m *Manifest) Validate() error {
	for idx, resource := range m.Resources {
		if resource.Kind == "" {
			return fmt.Errorf("resource %d: kind is required", idx)
		}
		if resource.Name == "" {
			return fmt.Errorf("resource %d: name is required", idx)
		}
		if resource.Spec == nil {
			return fmt.Errorf("resource %d (%s): spec is required", idx, resource.Name)
		}
	}

	return nil
}
