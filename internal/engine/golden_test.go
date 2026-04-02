package engine

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "update golden files")

// goldenFixture represents a single resource entry in a golden test yaml file.
type goldenFixture struct {
	Kind      string         `yaml:"kind"`
	Name      string         `yaml:"name"`
	DependsOn []string       `yaml:"depends_on"`
	Notify    []string       `yaml:"notify"`
	Trigger   bool           `yaml:"trigger"`
	Spec      map[string]any `yaml:"spec"`
	Vars      map[string]any `yaml:"vars"`
}

func TestGoldenFiles(t *testing.T) {
	entries, err := filepath.Glob(filepath.Join("testdata", "golden", "*.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no golden test fixtures found")
	}

	for _, yamlPath := range entries {
		name := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml")
		goldenPath := strings.TrimSuffix(yamlPath, ".yaml") + ".golden"

		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Read and parse the yaml fixture.
			yamlData, err := os.ReadFile(yamlPath)
			if err != nil {
				t.Fatalf("read yaml: %v", err)
			}

			// Replace TMPDIR placeholder in yaml.
			yamlStr := strings.ReplaceAll(string(yamlData), "{{TMPDIR}}", tmpDir)

			var fixtures []goldenFixture
			if err := yaml.Unmarshal([]byte(yamlStr), &fixtures); err != nil {
				t.Fatalf("unmarshal yaml: %v", err)
			}

			// For converged tests, pre-create files that should already exist.
			if name == "converged" {
				for _, f := range fixtures {
					path, _ := f.Spec["path"].(string)
					content, _ := f.Spec["content"].(string)
					if path != "" && content != "" {
						dir := filepath.Dir(path)
						if err := os.MkdirAll(dir, 0o755); err != nil {
							t.Fatalf("mkdir: %v", err)
						}
						if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
							t.Fatalf("write converged file: %v", err)
						}
					}
				}
			}

			// Build ResolvedResource slice.
			resources := make([]manifest.ResolvedResource, len(fixtures))
			for i, f := range fixtures {
				resources[i] = manifest.ResolvedResource{
					Kind:             f.Kind,
					Name:             f.Name,
					DependsOn:        f.DependsOn,
					Notify:           f.Notify,
					Trigger:          f.Trigger,
					Spec:             f.Spec,
					Vars:             f.Vars,
					DeclarationOrder: i,
				}
			}

			planner := NewPlanner()
			plan, err := planner.BuildPlan(resources)
			if err != nil {
				t.Fatalf("BuildPlan: %v", err)
			}
			got := plan.Script()

			// Read golden file and apply TMPDIR substitution.
			goldenData, err := os.ReadFile(goldenPath)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("read golden: %v", err)
			}
			want := strings.ReplaceAll(string(goldenData), "{{TMPDIR}}", tmpDir)

			if *update {
				// Write actual output back, replacing tmpDir with placeholder.
				output := strings.ReplaceAll(got, tmpDir, "{{TMPDIR}}")
				if err := os.WriteFile(goldenPath, []byte(output), 0o644); err != nil {
					t.Fatalf("update golden: %v", err)
				}
				t.Logf("updated %s", goldenPath)
				return
			}

			if got != want {
				t.Errorf("plan output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
			}
		})
	}
}
