package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erik/anneal/internal/manifest"
)

func TestPlannerValidateSortsDiamondsByDeclarationOrder(t *testing.T) {
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("root", nil, "/tmp/root", "root", 0),
		fileResource("left", []string{"root"}, "/tmp/left", "left", 1),
		fileResource("right", []string{"root"}, "/tmp/right", "right", 2),
		fileResource("leaf", []string{"left", "right"}, "/tmp/leaf", "leaf", 3),
	}

	plan, err := planner.Build(resources)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	rootPos := strings.Index(plan, "/tmp/root")
	leftPos := strings.Index(plan, "/tmp/left")
	rightPos := strings.Index(plan, "/tmp/right")
	leafPos := strings.Index(plan, "/tmp/leaf")

	if !(rootPos < leftPos && leftPos < rightPos && rightPos < leafPos) {
		t.Fatalf("plan order incorrect:\n%s", plan)
	}
}

func TestPlannerValidateRejectsCycles(t *testing.T) {
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("a", []string{"b"}, "/tmp/a", "a", 0),
		fileResource("b", []string{"a"}, "/tmp/b", "b", 1),
	}

	_, err := planner.Build(resources)
	if err == nil {
		t.Fatal("Build() error = nil, want cycle error")
	}
	if !strings.Contains(err.Error(), "dependency cycle detected") {
		t.Fatalf("Build() error = %q, want cycle detection", err)
	}
	if !strings.Contains(err.Error(), "a -> b -> a") {
		t.Fatalf("Build() error = %q, want cycle path", err)
	}
}

func TestPlannerBuildOmitsConvergedFileResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "motd")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner := NewPlanner()
	plan, err := planner.Build([]manifest.ResolvedResource{
		fileResource("motd", nil, path, "hello", 0),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan != "" {
		t.Fatalf("plan = %q, want empty", plan)
	}
}

func fileResource(name string, dependsOn []string, path string, content string, order int) manifest.ResolvedResource {
	return manifest.ResolvedResource{
		Kind:             "file",
		Name:             name,
		DependsOn:        dependsOn,
		DeclarationOrder: order,
		Spec: map[string]any{
			"path":    path,
			"content": content,
		},
	}
}
