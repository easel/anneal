package cli

import (
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
