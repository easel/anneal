package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func TestValidateShellProvider_AllFunctions(t *testing.T) {
	sp := &ShellProvider{
		Kind:       "myresource",
		ScriptPath: "/tmp/providers/myresource.sh",
		Script: `
read() {
  echo "current-state"
}

diff() {
  cat
}

emit() {
  cat
}
`,
	}
	if err := ValidateShellProvider(sp); err != nil {
		t.Fatalf("expected valid provider, got error: %v", err)
	}
}

func TestValidateShellProvider_MissingFunctions(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{
			name:    "missing all functions",
			script:  "#!/bin/sh\necho hello\n",
			wantErr: "read(), diff(), emit()",
		},
		{
			name: "missing emit",
			script: `
read() { echo ok; }
diff() { cat; }
`,
			wantErr: "emit()",
		},
		{
			name: "missing read and diff",
			script: `
emit() { cat; }
`,
			wantErr: "read(), diff()",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp := &ShellProvider{
				Kind:       "test",
				ScriptPath: "/tmp/providers/test.sh",
				Script:     tc.script,
			}
			err := ValidateShellProvider(sp)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestDiscoverShellProviders(t *testing.T) {
	// Set up a temp directory with a manifest and providers/ subdirectory.
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "anneal.yaml")
	os.WriteFile(manifestPath, []byte("resources: []\n"), 0644)

	providersDir := filepath.Join(tmp, "providers")
	os.MkdirAll(providersDir, 0755)

	// Write two provider scripts.
	os.WriteFile(filepath.Join(providersDir, "alpha.sh"), []byte(`
read() { echo ok; }
diff() { cat; }
emit() { cat; }
`), 0644)
	os.WriteFile(filepath.Join(providersDir, "beta.sh"), []byte(`
read() { echo state; }
diff() { cat; }
emit() { cat; }
`), 0644)

	// Write a non-.sh file that should be ignored.
	os.WriteFile(filepath.Join(providersDir, "README.md"), []byte("ignored"), 0644)

	providers, err := DiscoverShellProviders(manifestPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0].Kind != "alpha" {
		t.Errorf("expected first provider kind 'alpha', got %q", providers[0].Kind)
	}
	if providers[1].Kind != "beta" {
		t.Errorf("expected second provider kind 'beta', got %q", providers[1].Kind)
	}
}

func TestDiscoverShellProviders_NoDir(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "anneal.yaml")
	os.WriteFile(manifestPath, []byte("resources: []\n"), 0644)

	// No providers/ directory — should return nil, nil.
	providers, err := DiscoverShellProviders(manifestPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if providers != nil {
		t.Errorf("expected nil providers, got %v", providers)
	}
}

func TestShellProviderPlan_Converged(t *testing.T) {
	// A provider that outputs nothing from emit (system is converged).
	sp := &ShellProvider{
		Kind:       "converged_test",
		ScriptPath: "/tmp/providers/converged_test.sh",
		Script: `
read() {
  echo "current"
}

diff() {
  # No differences — output nothing.
  true
}

emit() {
  cat
}
`,
	}

	resource := manifest.ResolvedResource{
		Kind: "converged_test",
		Name: "test-resource",
		Spec: map[string]any{"key": "value"},
	}

	ops, err := sp.Plan(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ops != nil {
		t.Errorf("expected nil ops for converged resource, got %v", ops)
	}
}

func TestShellProviderPlan_EmitsOperations(t *testing.T) {
	// A provider that detects drift and emits an operation.
	sp := &ShellProvider{
		Kind:       "echo_test",
		ScriptPath: "/tmp/providers/echo_test.sh",
		Script: `
read() {
  echo "old-value"
}

diff() {
  echo "needs-update"
}

emit() {
  echo "echo applying-changes"
}
`,
	}

	resource := manifest.ResolvedResource{
		Kind: "echo_test",
		Name: "test-resource",
		Spec: map[string]any{"value": "new-value"},
	}

	ops, err := sp.Plan(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected operations, got none")
	}
	if !strings.Contains(ops[0], "applying-changes") {
		t.Errorf("expected ops to contain 'applying-changes', got %q", ops[0])
	}
}

func TestShellProviderPlan_SpecPassedAsEnv(t *testing.T) {
	// A provider that uses ANNEAL_SPEC_PATH from the environment.
	sp := &ShellProvider{
		Kind:       "env_test",
		ScriptPath: "/tmp/providers/env_test.sh",
		Script: `
read() {
  echo "none"
}

diff() {
  echo "changed"
}

emit() {
  echo "echo path=$ANNEAL_SPEC_PATH"
}
`,
	}

	resource := manifest.ResolvedResource{
		Kind: "env_test",
		Name: "test-resource",
		Spec: map[string]any{"path": "/etc/myconfig"},
	}

	ops, err := sp.Plan(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected operations, got none")
	}
	if !strings.Contains(ops[0], "path=/etc/myconfig") {
		t.Errorf("expected ops to reference path, got %q", ops[0])
	}
}

func TestShellProviderPlan_HyphenatedSpecKeys(t *testing.T) {
	// A provider that reads a hyphenated spec key via the sanitized env var name.
	sp := &ShellProvider{
		Kind:       "env_test",
		ScriptPath: "/tmp/providers/env_test.sh",
		Script: `
read() {
  echo "none"
}

diff() {
  echo "changed"
}

emit() {
  echo "echo mykey=$ANNEAL_SPEC_MY_KEY otherkey=$ANNEAL_SPEC_OTHER_KEY"
}
`,
	}

	resource := manifest.ResolvedResource{
		Kind: "env_test",
		Name: "test-resource",
		Spec: map[string]any{
			"my-key":    "hello",
			"other.key": "world",
		},
	}

	ops, err := sp.Plan(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected operations, got none")
	}
	if !strings.Contains(ops[0], "mykey=hello") {
		t.Errorf("expected ops to contain hyphenated key value, got %q", ops[0])
	}
	if !strings.Contains(ops[0], "otherkey=world") {
		t.Errorf("expected ops to contain dotted key value, got %q", ops[0])
	}
}

func TestSanitizeEnvKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MY_KEY", "MY_KEY"},
		{"MY-KEY", "MY_KEY"},
		{"OTHER.KEY", "OTHER_KEY"},
		{"A B C", "A_B_C"},
		{"KEY123", "KEY123"},
		{"123KEY", "123KEY"},
	}
	for _, tc := range tests {
		got := sanitizeEnvKey(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeEnvKey(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRegisterProvider_DuplicateKind(t *testing.T) {
	planner := NewPlanner()
	sp := &ShellProvider{Kind: "file", ScriptPath: "/tmp/file.sh", Script: ""}

	err := planner.RegisterProvider("file", sp)
	if err == nil {
		t.Fatal("expected error for duplicate kind, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterProvider_NewKind(t *testing.T) {
	planner := NewPlanner()
	sp := &ShellProvider{
		Kind:       "custom_thing",
		ScriptPath: "/tmp/custom_thing.sh",
		Script: `
read() { echo ok; }
diff() { cat; }
emit() { cat; }
`,
	}

	if err := planner.RegisterProvider("custom_thing", sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The provider should now be usable in validation.
	err := planner.Validate([]manifest.ResolvedResource{
		{Kind: "custom_thing", Name: "test"},
	})
	if err != nil {
		t.Errorf("custom provider should be valid in planner, got: %v", err)
	}
}

func TestShellProviderAppearsInPlan(t *testing.T) {
	// End-to-end: register a custom provider and build a plan.
	planner := NewPlanner()
	sp := &ShellProvider{
		Kind:       "greeting",
		ScriptPath: "/tmp/providers/greeting.sh",
		Script: `
read() {
  echo "none"
}

diff() {
  echo "needs-greeting"
}

emit() {
  echo "echo hello-world"
}
`,
	}
	if err := planner.RegisterProvider("greeting", sp); err != nil {
		t.Fatal(err)
	}

	plan, err := planner.BuildPlan([]manifest.ResolvedResource{
		{Kind: "greeting", Name: "say-hello", Spec: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	if len(plan.Resources) != 1 {
		t.Fatalf("expected 1 resource in plan, got %d", len(plan.Resources))
	}
	rp := plan.Resources[0]
	if rp.Name != "say-hello" {
		t.Errorf("expected name 'say-hello', got %q", rp.Name)
	}
	if rp.Kind != "greeting" {
		t.Errorf("expected kind 'greeting', got %q", rp.Kind)
	}
	if !strings.Contains(rp.Script, "hello-world") {
		t.Errorf("expected script to contain 'hello-world', got %q", rp.Script)
	}

	// Script output should look identical to built-in providers.
	fullScript := plan.Script()
	if !strings.Contains(fullScript, "#!/bin/sh") {
		t.Error("plan script missing shebang")
	}
	if !strings.Contains(fullScript, "hello-world") {
		t.Errorf("full plan script missing custom provider output: %s", fullScript)
	}
}

func TestProviderRegistryWithCustom(t *testing.T) {
	customs := []*ShellProvider{
		{Kind: "zzz_custom", ScriptPath: "/tmp/providers/zzz_custom.sh"},
	}
	registry := ProviderRegistryWithCustom(customs)

	// Find the custom provider in the sorted registry.
	found := false
	for _, info := range registry {
		if info.Kind == "zzz_custom" {
			found = true
			if !info.Custom {
				t.Error("expected Custom=true for shell provider")
			}
			break
		}
	}
	if !found {
		t.Error("custom provider not found in registry")
	}

	// Verify it's sorted: last entry should be zzz_custom.
	last := registry[len(registry)-1]
	if last.Kind != "zzz_custom" {
		t.Errorf("expected last entry to be 'zzz_custom', got %q", last.Kind)
	}
}
