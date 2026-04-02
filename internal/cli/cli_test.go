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
