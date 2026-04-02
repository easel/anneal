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

func TestTriggerOrderedAfterNormalResources(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathT := filepath.Join(dir, "trigger")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		// Trigger declared first in the manifest
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathT, "content": "triggered"},
		},
		// Normal resource declared second, notifies the trigger
		{
			Kind: "file", Name: "config",
			Notify:           []string{"restart"},
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathA, "content": "new config"},
		},
	}

	plan, err := planner.BuildPlan(resources)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	// Normal resource should come before trigger regardless of declaration order
	if len(plan.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(plan.Resources))
	}
	if plan.Resources[0].Name != "config" {
		t.Fatalf("first resource = %q, want config (normal before trigger)", plan.Resources[0].Name)
	}
	if plan.Resources[1].Name != "restart" {
		t.Fatalf("second resource = %q, want restart (trigger after normal)", plan.Resources[1].Name)
	}
	if !plan.Resources[1].Trigger {
		t.Fatal("restart should be marked as trigger")
	}
}

func TestTriggerFiresWhenNotifierChanges(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "config")
	pathT := filepath.Join(dir, "trigger")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			Notify:           []string{"restart"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "new config"},
		},
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathT, "content": "restart action"},
		},
	}

	plan, err := planner.BuildPlan(resources)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	// Config is new (file doesn't exist) so it has changes → trigger should fire
	if plan.Resources[1].Script == "" {
		t.Fatal("trigger should have a script when notifier has changes")
	}
	if !strings.Contains(plan.Resources[1].Script, "# triggered") {
		t.Fatalf("trigger script should contain '# triggered' comment, got:\n%s", plan.Resources[1].Script)
	}
}

func TestTriggerDoesNotFireWhenNotifierConverged(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "config")
	pathT := filepath.Join(dir, "trigger")

	// Pre-create the config file so it's converged
	os.WriteFile(pathA, []byte("existing config"), 0o644)

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			Notify:           []string{"restart"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "existing config"},
		},
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathT, "content": "restart action"},
		},
	}

	plan, err := planner.BuildPlan(resources)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	// Config is converged → trigger should NOT fire
	if plan.Resources[1].Script != "" {
		t.Fatalf("trigger should not fire when notifier is converged, got script:\n%s", plan.Resources[1].Script)
	}
}

func TestTriggerMultipleNotifiersFireOnce(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathB := filepath.Join(dir, "b")
	pathT := filepath.Join(dir, "trigger")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config-a",
			Notify:           []string{"restart"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "a"},
		},
		{
			Kind: "file", Name: "config-b",
			Notify:           []string{"restart"},
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathB, "content": "b"},
		},
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 2,
			Spec: map[string]any{"path": pathT, "content": "restarted"},
		},
	}

	plan, err := planner.BuildPlan(resources)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	// Both notifiers change → trigger fires once (appears once in plan)
	triggerCount := 0
	for _, rp := range plan.Resources {
		if rp.Trigger && rp.Script != "" {
			triggerCount++
		}
	}
	if triggerCount != 1 {
		t.Fatalf("trigger should fire exactly once, got %d", triggerCount)
	}
}

func TestTriggerApplyChangedOnlyFiring(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "config")
	pathT := filepath.Join(dir, "trigger")

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			Notify:           []string{"restart"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "new config"},
		},
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathT, "content": "restart action"},
		},
	}

	result, err := planner.Apply(mock, resources, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Config changes → trigger should execute
	if len(mock.Executed) != 2 {
		t.Fatalf("executed %d scripts, want 2 (config + trigger)", len(mock.Executed))
	}
	if result.Results[0].Status != StatusApplied {
		t.Errorf("config status = %v, want applied", result.Results[0].Status)
	}
	if result.Results[1].Status != StatusApplied {
		t.Errorf("trigger status = %v, want applied", result.Results[1].Status)
	}
}

func TestTriggerApplyConvergedNotifier(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "config")
	pathT := filepath.Join(dir, "trigger")

	// Pre-create so config is converged
	os.WriteFile(pathA, []byte("existing"), 0o644)

	mock := &MockSystem{}
	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			Notify:           []string{"restart"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "existing"},
		},
		{
			Kind: "file", Name: "restart", Trigger: true,
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathT, "content": "restart action"},
		},
	}

	result, err := planner.Apply(mock, resources, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Config is converged → trigger should NOT execute
	if len(mock.Executed) != 0 {
		t.Fatalf("executed %d scripts, want 0 (nothing changed)", len(mock.Executed))
	}
	if result.Results[0].Status != StatusConverged {
		t.Errorf("config status = %v, want converged", result.Results[0].Status)
	}
	if result.Results[1].Status != StatusConverged {
		t.Errorf("trigger status = %v, want converged", result.Results[1].Status)
	}
}

func TestValidateNotifyTargetMustBeTrigger(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathB := filepath.Join(dir, "b")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			Notify:           []string{"not-a-trigger"},
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "x"},
		},
		{
			Kind: "file", Name: "not-a-trigger",
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathB, "content": "y"},
		},
	}

	err := planner.Validate(resources)
	if err == nil {
		t.Fatal("Validate() should reject notify to non-trigger resource")
	}
	if !strings.Contains(err.Error(), "not a trigger") {
		t.Fatalf("error = %q, want 'not a trigger'", err)
	}
}

func TestTriggerNeverNotifiedStaysConverged(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "config")
	pathT := filepath.Join(dir, "trigger")

	planner := NewPlanner()
	resources := []manifest.ResolvedResource{
		{
			Kind: "file", Name: "config",
			// No notify edge to the trigger
			DeclarationOrder: 0,
			Spec: map[string]any{"path": pathA, "content": "new config"},
		},
		{
			Kind: "file", Name: "unused-trigger", Trigger: true,
			DeclarationOrder: 1,
			Spec: map[string]any{"path": pathT, "content": "never fires"},
		},
	}

	plan, err := planner.BuildPlan(resources)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	// Trigger should not fire even though config has changes (no notify edge)
	for _, rp := range plan.Resources {
		if rp.Name == "unused-trigger" && rp.Script != "" {
			t.Fatalf("unused trigger should not fire, got script:\n%s", rp.Script)
		}
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
