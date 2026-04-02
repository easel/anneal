package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKindSpec(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dev", "generate",
		"--kind", "file",
		"--spec", `{"path":"/etc/motd","content":"hello"}`)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "kind: file") {
		t.Fatalf("output missing kind: %s", stdout)
	}
	if !strings.Contains(stdout, "/etc/motd") {
		t.Fatalf("output missing path: %s", stdout)
	}
	if !strings.Contains(stdout, "hello") {
		t.Fatalf("output missing content: %s", stdout)
	}
	// Should have sensible defaults
	if !strings.Contains(stdout, "0644") {
		t.Fatalf("output missing default mode: %s", stdout)
	}
	if !strings.Contains(stdout, "root:root") {
		t.Fatalf("output missing default owner: %s", stdout)
	}
}

func TestGenerateKindSpecJSON(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "generate",
		"--json",
		"--kind", "file",
		"--spec", `{"path":"/etc/motd","content":"hello"}`)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result generateFragment
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources count = %d, want 1", len(result.Resources))
	}
	if result.Resources[0].Kind != "file" {
		t.Fatalf("kind = %q, want %q", result.Resources[0].Kind, "file")
	}
}

func TestGenerateAutoName(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "generate",
		"--kind", "file",
		"--spec", `{"path":"/etc/motd","content":"hello"}`)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "name: motd") {
		t.Fatalf("auto-generated name missing: %s", stdout)
	}
}

func TestGenerateCustomName(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "generate",
		"--kind", "file",
		"--name", "my-motd",
		"--spec", `{"path":"/etc/motd","content":"hello"}`)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "name: my-motd") {
		t.Fatalf("custom name missing: %s", stdout)
	}
}

func TestGenerateFromGoalInstall(t *testing.T) {
	// Override os-release for testing
	dir := t.TempDir()
	osReleasePath = filepath.Join(dir, "os-release")
	os.WriteFile(osReleasePath, []byte("ID=ubuntu\nVERSION_ID=\"22.04\"\n"), 0o644)
	t.Cleanup(func() { osReleasePath = "/etc/os-release" })

	stdout, _, code := runCLI(t, "dev", "generate",
		"--from-goal", "install nginx")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "apt_packages") {
		t.Fatalf("should resolve to apt_packages on Ubuntu: %s", stdout)
	}
	if !strings.Contains(stdout, "nginx") {
		t.Fatalf("output missing package name: %s", stdout)
	}
}

func TestGenerateFromGoalInstallFedora(t *testing.T) {
	dir := t.TempDir()
	osReleasePath = filepath.Join(dir, "os-release")
	os.WriteFile(osReleasePath, []byte("ID=fedora\nVERSION_ID=\"41\"\n"), 0o644)
	t.Cleanup(func() { osReleasePath = "/etc/os-release" })

	stdout, _, code := runCLI(t, "dev", "generate",
		"--from-goal", "install package nginx")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "dnf_packages") {
		t.Fatalf("should resolve to dnf_packages on Fedora: %s", stdout)
	}
}

func TestGenerateFromGoalInstallArch(t *testing.T) {
	dir := t.TempDir()
	osReleasePath = filepath.Join(dir, "os-release")
	os.WriteFile(osReleasePath, []byte("ID=arch\n"), 0o644)
	t.Cleanup(func() { osReleasePath = "/etc/os-release" })

	stdout, _, code := runCLI(t, "dev", "generate",
		"--from-goal", "install nginx")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "pacman_packages") {
		t.Fatalf("should resolve to pacman_packages on Arch: %s", stdout)
	}
}

func TestGenerateFromGoalCreateFile(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "generate",
		"--from-goal", "create file /etc/myapp.conf")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "kind: file") {
		t.Fatalf("should resolve to file kind: %s", stdout)
	}
	if !strings.Contains(stdout, "/etc/myapp.conf") {
		t.Fatalf("output missing path: %s", stdout)
	}
}

func TestGenerateFromGoalCreateDir(t *testing.T) {
	stdout, _, code := runCLI(t, "dev", "generate",
		"--from-goal", "create directory /var/data")
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "kind: directory") {
		t.Fatalf("should resolve to directory kind: %s", stdout)
	}
	if !strings.Contains(stdout, "/var/data") {
		t.Fatalf("output missing path: %s", stdout)
	}
}

func TestGenerateInvalidGoal(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "generate",
		"--from-goal", "do something weird")
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "could not resolve goal") {
		t.Fatalf("stderr = %q, want goal resolution error", stderr)
	}
}

func TestGenerateNoFlags(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "generate")
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "--kind") || !strings.Contains(stderr, "--from-goal") {
		t.Fatalf("stderr = %q, want usage hint", stderr)
	}
}

func TestGenerateInvalidSpecJSON(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "generate",
		"--kind", "file",
		"--spec", `{invalid}`)
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "invalid --spec JSON") {
		t.Fatalf("stderr = %q, want JSON error", stderr)
	}
}

func TestGenerateOutputValidates(t *testing.T) {
	// Generate a fragment and verify it can be validated by anneal
	dir := t.TempDir()
	target := filepath.Join(dir, "motd")

	stdout, _, code := runCLI(t, "dev", "generate",
		"--kind", "file",
		"--spec", `{"path":"`+target+`","content":"hello"}`)
	if code != ExitCodeSuccess {
		t.Fatalf("generate exit code = %d", code)
	}

	// Write the generated YAML to a file and validate it
	manifestPath := filepath.Join(dir, "anneal.yaml")
	os.WriteFile(manifestPath, []byte(stdout), 0o644)

	_, _, vcode := runCLI(t, "dev", "validate", "-f", manifestPath)
	if vcode != ExitCodeSuccess {
		t.Fatalf("generated fragment failed validation")
	}
}
