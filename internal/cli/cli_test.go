package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	stdout, stderr, code := runCLI(t, "1.2.3", "version")
	if code != ExitCodeSuccess {
		t.Fatalf("version exit code = %d, want %d", code, ExitCodeSuccess)
	}
	if got := strings.TrimSpace(stdout); got != "1.2.3" {
		t.Fatalf("version output = %q, want %q", got, "1.2.3")
	}
	if stderr != "" {
		t.Fatalf("version stderr = %q, want empty", stderr)
	}
}

func TestValidateSuccess(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "validate", "--manifest", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("validate exit code = %d, want %d", code, ExitCodeSuccess)
	}
	if !strings.Contains(stdout, "is valid") {
		t.Fatalf("validate stdout = %q, want success message", stdout)
	}
	if stderr != "" {
		t.Fatalf("validate stderr = %q, want empty", stderr)
	}
}

func TestValidateManifestError(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind file
`)

	_, stderr, code := runCLI(t, "dev", "validate", "--manifest", manifestPath)
	if code != ExitCodeRuntimeError {
		t.Fatalf("validate exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, filepath.Base(manifestPath)) {
		t.Fatalf("validate stderr = %q, want manifest path", stderr)
	}
}

func TestPlanSkeletonHasDeterministicExitCode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "plan", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("plan exit code = %d, want %d", code, ExitCodeSuccess)
	}
	if !strings.Contains(stdout, "stdlib_file_write") {
		t.Fatalf("plan stdout = %q, want stdlib file write", stdout)
	}
	if stderr != "" {
		t.Fatalf("plan stderr = %q, want empty", stderr)
	}
}

func TestValidateRejectsDependencyCycles(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: a
    depends_on: [b]
    spec:
      path: /tmp/a
      content: a
  - kind: file
    name: b
    depends_on: [a]
    spec:
      path: /tmp/b
      content: b
`)

	_, stderr, code := runCLI(t, "dev", "validate", "--manifest", manifestPath)
	if code != ExitCodeRuntimeError {
		t.Fatalf("validate exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "dependency cycle detected") {
		t.Fatalf("validate stderr = %q, want cycle detection error", stderr)
	}
}

func TestRootHelpListsCommandSurface(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "--help")
	if code != ExitCodeSuccess {
		t.Fatalf("help exit code = %d, want %d", code, ExitCodeSuccess)
	}
	for _, subcommand := range []string{"validate", "plan", "apply", "version"} {
		if !strings.Contains(stdout, subcommand) {
			t.Fatalf("help output missing %q: %s", subcommand, stdout)
		}
	}
}

func TestUsageErrorsReturnUsageExitCode(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "validate", "--missing-flag")
	if code != ExitCodeUsageError {
		t.Fatalf("usage exit code = %d, want %d", code, ExitCodeUsageError)
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("usage stderr = %q, want unknown flag error", stderr)
	}
}

func TestApplyConvergesFileResource(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello world
`)

	stdout, stderr, code := runCLI(t, "dev", "apply", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("apply exit code = %d, want %d\nstdout: %s\nstderr: %s", code, ExitCodeSuccess, stdout, stderr)
	}
	if !strings.Contains(stdout, "applied: motd") {
		t.Fatalf("apply stdout missing applied status: %s", stdout)
	}

	// Verify the file was actually written
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("file content = %q, want %q", string(data), "hello world")
	}
}

func TestApplyIdempotentRerun(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	// First apply
	_, _, code := runCLI(t, "dev", "apply", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("first apply exit code = %d", code)
	}

	// Second apply should show converged, not applied
	stdout, _, code := runCLI(t, "dev", "apply", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("second apply exit code = %d", code)
	}
	if !strings.Contains(stdout, "converged: motd") {
		t.Fatalf("second apply should show converged: %s", stdout)
	}
}

func TestApplyWithSavedPlanDriftDetection(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	// Save the plan output
	planOut, _, code := runCLI(t, "dev", "plan", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("plan exit code = %d", code)
	}
	planFile := filepath.Join(dir, "plan.sh")
	os.WriteFile(planFile, []byte(planOut), 0o644)

	// Apply with the saved plan (should succeed since nothing changed)
	stdout, _, code := runCLI(t, "dev", "apply", "-f", manifestPath, "--plan", planFile)
	if code != ExitCodeSuccess {
		t.Fatalf("apply with plan exit code = %d\nstdout: %s", code, stdout)
	}

	// Now mutate the target so system state drifts from when the plan was saved.
	// The file now exists with different content, so re-planning will produce a
	// different script (content-changed instead of new-file), causing drift.
	if err := os.WriteFile(target, []byte("different"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Apply with the stale saved plan should detect drift and abort.
	_, stderr, code := runCLI(t, "dev", "apply", "-f", manifestPath, "--plan", planFile)
	if code != ExitCodeRuntimeError {
		t.Fatalf("apply with stale plan exit code = %d, want %d\nstderr: %s", code, ExitCodeRuntimeError, stderr)
	}
	if !strings.Contains(stderr, "plan drift detected") {
		t.Fatalf("apply stderr = %q, want drift detection error", stderr)
	}

	// Verify nothing was executed — the file should still have the mutated content.
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "different" {
		t.Fatalf("file content = %q, want %q (drift should prevent execution)", string(data), "different")
	}
}

// --- JSON output tests ---

func TestValidateJSONValid(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "validate", "--json", "--manifest", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}

	var result validateOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if !result.Valid {
		t.Fatalf("valid = false, want true")
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %v, want empty", result.Issues)
	}
}

func TestValidateJSONInvalid(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: a
    depends_on: [b]
    spec:
      path: /tmp/a
      content: a
  - kind: file
    name: b
    depends_on: [a]
    spec:
      path: /tmp/b
      content: b
`)

	stdout, stderr, code := runCLI(t, "dev", "validate", "--json", "--manifest", manifestPath)
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty in JSON mode, got: %s", stderr)
	}

	var result validateOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues but got none")
	}
	if !strings.Contains(result.Issues[0].Message, "dependency cycle") {
		t.Fatalf("issue message = %q, want cycle error", result.Issues[0].Message)
	}
}

func TestPlanJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "plan", "--json", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}

	var result planOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources count = %d, want 1", len(result.Resources))
	}
	r := result.Resources[0]
	if r.Name != "motd" {
		t.Fatalf("name = %q, want %q", r.Name, "motd")
	}
	if r.Kind != "file" {
		t.Fatalf("kind = %q, want %q", r.Kind, "file")
	}
	if r.Status != "changed" {
		t.Fatalf("status = %q, want %q", r.Status, "changed")
	}
	if !strings.Contains(r.Operations, "stdlib_file_write") {
		t.Fatalf("operations missing stdlib_file_write: %s", r.Operations)
	}
}

func TestPlanJSONConverged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	os.WriteFile(target, []byte("hello"), 0o644)

	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, _, code := runCLI(t, "dev", "plan", "--json", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result planOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources count = %d, want 1", len(result.Resources))
	}
	if result.Resources[0].Status != "converged" {
		t.Fatalf("status = %q, want %q", result.Resources[0].Status, "converged")
	}
}

func TestPlanOutputFlag(t *testing.T) {
	target := filepath.Join(t.TempDir(), "motd")
	outFile := filepath.Join(t.TempDir(), "plan.sh")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "plan", "-o", outFile, "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout should be empty when -o is used, got %q", stdout)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if !strings.Contains(string(data), "stdlib_file_write") {
		t.Fatalf("output file missing stdlib_file_write: %s", string(data))
	}
}

func TestPlanOutputFlagJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "motd")
	outFile := filepath.Join(t.TempDir(), "plan.json")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "plan", "--json", "-o", outFile, "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout should be empty when -o is used, got %q", stdout)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	var result planOutput
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, string(data))
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources count = %d, want 1", len(result.Resources))
	}
}

func TestApplyJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	stdout, stderr, code := runCLI(t, "dev", "apply", "--json", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, ExitCodeSuccess, stdout, stderr)
	}

	var result applyOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if !result.Success {
		t.Fatal("success = false, want true")
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources count = %d, want 1", len(result.Resources))
	}
	r := result.Resources[0]
	if r.Name != "motd" {
		t.Fatalf("name = %q, want %q", r.Name, "motd")
	}
	if r.Status != "applied" {
		t.Fatalf("status = %q, want %q", r.Status, "applied")
	}
}

func TestApplyJSONIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: `+target+`
      content: hello
`)

	// First apply
	runCLI(t, "dev", "apply", "-f", manifestPath)

	// Second apply with --json — should show converged
	stdout, _, code := runCLI(t, "dev", "apply", "--json", "-f", manifestPath)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result applyOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if !result.Success {
		t.Fatal("success = false, want true")
	}
	if result.Resources[0].Status != "converged" {
		t.Fatalf("status = %q, want %q", result.Resources[0].Status, "converged")
	}
}

// --- Providers command tests ---

func TestProvidersListAll(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dev", "providers")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}
	// Should list known providers
	for _, kind := range []string{"file", "directory", "apt_packages", "docker_container", "command"} {
		if !strings.Contains(stdout, kind) {
			t.Errorf("providers output missing %q", kind)
		}
	}
}

func TestProvidersListJSON(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "providers", "--json")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if len(result) == 0 {
		t.Fatal("expected provider entries but got none")
	}
	// Verify structure of first entry
	first := result[0]
	for _, field := range []string{"kind", "description", "required_fields"} {
		if _, ok := first[field]; !ok {
			t.Errorf("provider entry missing field %q", field)
		}
	}
}

func TestProvidersDetailHuman(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "providers", "file")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "Provider: file") {
		t.Fatalf("output missing provider name: %s", stdout)
	}
	if !strings.Contains(stdout, "path") {
		t.Fatalf("output missing required field 'path': %s", stdout)
	}
	if !strings.Contains(stdout, "content") {
		t.Fatalf("output missing required field 'content': %s", stdout)
	}
}

func TestProvidersDetailJSON(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "providers", "--json", "file")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if result["kind"] != "file" {
		t.Fatalf("kind = %v, want 'file'", result["kind"])
	}
}

func TestProvidersUnknownKind(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "providers", "nonexistent")
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "unknown provider kind") {
		t.Fatalf("stderr = %q, want unknown kind error", stderr)
	}
}

func TestDefaultOutputUnchanged(t *testing.T) {
	manifestPath := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)

	// Validate without --json should produce human-readable output
	stdout, _, _ := runCLI(t, "dev", "validate", "--manifest", manifestPath)
	if !strings.Contains(stdout, "is valid") {
		t.Fatalf("default validate should be human-readable: %s", stdout)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatal("default output should not be JSON")
	}
}

func writeManifest(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "anneal.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func runCLI(t *testing.T, version string, args ...string) (string, string, int) {
	t.Helper()
	var stdout strings.Builder
	var stderr strings.Builder
	code := Execute(args, &stdout, &stderr, version)
	return stdout.String(), stderr.String(), code
}
