package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
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

func TestPlannerBuildHeredocDelimiterInjection(t *testing.T) {
	// Content that contains the default delimiter must not cause early termination
	malicious := "before\nANNEAL_EOF\nrm -rf /\n"
	path := filepath.Join(t.TempDir(), "evil")

	planner := NewPlanner()
	plan, err := planner.Build([]manifest.ResolvedResource{
		fileResource("evil", nil, path, malicious, 0),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// The plan must use a different delimiter so ANNEAL_EOF in content is inert
	if strings.Contains(plan, "<<'ANNEAL_EOF'\n") {
		t.Fatal("plan used ANNEAL_EOF delimiter despite content containing it")
	}
	// The full malicious content must appear verbatim inside the heredoc
	if !strings.Contains(plan, malicious) {
		t.Fatal("plan does not contain the full original content")
	}
}

func TestPlannerBuildShellQuotesPathModeOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "has spaces")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{{
		Kind:             "file",
		Name:             "quoted",
		DeclarationOrder: 0,
		Spec: map[string]any{
			"path":    path,
			"content": "ok",
			"mode":    "0600",
			"owner":   "user's:group",
		},
	}}

	plan, err := planner.Build(resources)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Path with spaces must be single-quoted
	if !strings.Contains(plan, "'"+path+"'") {
		t.Fatalf("path not quoted in plan:\n%s", plan)
	}
	// Owner with apostrophe must be safely quoted
	if !strings.Contains(plan, `'user'\''s:group'`) {
		t.Fatalf("owner not properly quoted in plan:\n%s", plan)
	}
}

func TestApplyExecutesResourcesInOrder(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathB := filepath.Join(dir, "b")

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("a", nil, pathA, "content-a", 0),
		fileResource("b", []string{"a"}, pathB, "content-b", 1),
	}

	result, err := planner.Apply(mock, resources, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Failed() {
		t.Fatalf("Apply() failed:\n%s", result.Summary())
	}
	if len(mock.Executed) != 2 {
		t.Fatalf("executed %d scripts, want 2", len(mock.Executed))
	}
	if !strings.Contains(mock.Executed[0], pathA) {
		t.Fatalf("first script should reference %s, got:\n%s", pathA, mock.Executed[0])
	}
	if !strings.Contains(mock.Executed[1], pathB) {
		t.Fatalf("second script should reference %s, got:\n%s", pathB, mock.Executed[1])
	}
	// Check result statuses
	if result.Results[0].Status != StatusApplied || result.Results[1].Status != StatusApplied {
		t.Fatalf("expected both applied:\n%s", result.Summary())
	}
}

func TestApplyFailStopSkipsRemaining(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathB := filepath.Join(dir, "b")
	pathC := filepath.Join(dir, "c")

	mock := &MockSystem{
		FailOn: map[string]error{
			pathB: fmt.Errorf("permission denied"),
		},
	}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("a", nil, pathA, "a", 0),
		fileResource("b", nil, pathB, "b", 1),
		fileResource("c", nil, pathC, "c", 2),
	}

	result, err := planner.Apply(mock, resources, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Failed() {
		t.Fatal("Apply() should have failed")
	}

	if result.Results[0].Status != StatusApplied {
		t.Fatalf("resource a: status = %v, want applied", result.Results[0].Status)
	}
	if result.Results[1].Status != StatusFailed {
		t.Fatalf("resource b: status = %v, want failed", result.Results[1].Status)
	}
	if result.Results[2].Status != StatusSkipped {
		t.Fatalf("resource c: status = %v, want skipped", result.Results[2].Status)
	}

	summary := result.Summary()
	if !strings.Contains(summary, "FAILED: b") {
		t.Fatalf("summary missing failure info:\n%s", summary)
	}
	if !strings.Contains(summary, "skipped: c") {
		t.Fatalf("summary missing skip info:\n%s", summary)
	}
}

func TestApplyConvergedResourcesAreTracked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")
	os.WriteFile(path, []byte("hello"), 0o644)

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("motd", nil, path, "hello", 0),
	}

	result, err := planner.Apply(mock, resources, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Failed() {
		t.Fatalf("Apply() failed:\n%s", result.Summary())
	}
	if len(mock.Executed) != 0 {
		t.Fatalf("executed %d scripts, want 0 (already converged)", len(mock.Executed))
	}
	if result.Results[0].Status != StatusConverged {
		t.Fatalf("status = %v, want converged", result.Results[0].Status)
	}
}

func TestApplyDriftDetectionAborts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("motd", nil, path, "hello", 0),
	}

	// Save a plan that doesn't match current state
	savedScript := "#!/bin/sh\nset -e\n\necho different plan\n"

	_, err := planner.Apply(mock, resources, savedScript)
	if err == nil {
		t.Fatal("Apply() should fail on drift")
	}
	if !strings.Contains(err.Error(), "plan drift detected") {
		t.Fatalf("Apply() error = %q, want drift detection", err)
	}
	if len(mock.Executed) != 0 {
		t.Fatal("drift detection should prevent any execution")
	}
}

func TestApplyDriftDetectionPassesOnMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motd")

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		fileResource("motd", nil, path, "hello", 0),
	}

	// Build the actual plan script, then apply with it as saved
	plan, _ := planner.Build(resources)

	result, err := planner.Apply(mock, resources, plan)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Failed() {
		t.Fatalf("Apply() failed:\n%s", result.Summary())
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
