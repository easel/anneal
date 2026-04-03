package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeBasicCombine(t *testing.T) {
	base := writeManifest(t, `
vars:
  domain: example.com
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
resources:
  - kind: directory
    name: data-dir
    spec:
      path: /srv/data
`)

	stdout, stderr, code := runCLI(t, "dev", "merge", base, fragment)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeSuccess, stderr)
	}
	if !strings.Contains(stdout, "motd") {
		t.Fatalf("output missing base resource 'motd': %s", stdout)
	}
	if !strings.Contains(stdout, "data-dir") {
		t.Fatalf("output missing fragment resource 'data-dir': %s", stdout)
	}
	if !strings.Contains(stdout, "domain") {
		t.Fatalf("output missing base var 'domain': %s", stdout)
	}
}

func TestMergeDuplicateResourceError(t *testing.T) {
	base := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: world
`)

	_, stderr, code := runCLI(t, "dev", "merge", base, fragment)
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "duplicate resource name") {
		t.Fatalf("stderr = %q, want duplicate resource error", stderr)
	}
	if !strings.Contains(stderr, "motd") {
		t.Fatalf("stderr should mention the duplicate name 'motd': %s", stderr)
	}
}

func TestMergePreservesVars(t *testing.T) {
	base := writeManifest(t, `
vars:
  domain: example.com
  port: 8080
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
vars:
  app_name: myapp
resources:
  - kind: directory
    name: app-dir
    spec:
      path: /opt/myapp
`)

	stdout, _, code := runCLI(t, "dev", "merge", base, fragment)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "domain") {
		t.Fatalf("output missing base var 'domain': %s", stdout)
	}
	if !strings.Contains(stdout, "app_name") {
		t.Fatalf("output missing fragment var 'app_name': %s", stdout)
	}
}

func TestMergeOutputPassesValidate(t *testing.T) {
	base := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
resources:
  - kind: directory
    name: data-dir
    spec:
      path: /srv/data
`)

	// Run merge and capture output.
	stdout, _, code := runCLI(t, "dev", "merge", base, fragment)
	if code != ExitCodeSuccess {
		t.Fatalf("merge exit code = %d", code)
	}

	// Write the merged output to a file and validate it.
	mergedPath := filepath.Join(t.TempDir(), "merged.yaml")
	if err := os.WriteFile(mergedPath, []byte(stdout), 0o644); err != nil {
		t.Fatalf("write merged file: %v", err)
	}

	_, stderr, code := runCLI(t, "dev", "validate", "-f", mergedPath)
	if code != ExitCodeSuccess {
		t.Fatalf("validate of merged output failed: exit code = %d\nstderr: %s\nmerged output:\n%s", code, stderr, stdout)
	}
}

func TestMergeJSONOutput(t *testing.T) {
	base := writeManifest(t, `
vars:
  domain: example.com
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
resources:
  - kind: directory
    name: data-dir
    spec:
      path: /srv/data
`)

	stdout, _, code := runCLI(t, "dev", "merge", "--json", base, fragment)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, stdout)
	}

	resources, ok := result["resources"].([]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("expected 2 resources, got: %v", result["resources"])
	}

	vars, ok := result["vars"].(map[string]any)
	if !ok {
		t.Fatalf("expected vars object, got: %v", result["vars"])
	}
	if vars["domain"] != "example.com" {
		t.Fatalf("vars.domain = %v, want 'example.com'", vars["domain"])
	}
}

func TestMergeNoVarsOmitted(t *testing.T) {
	base := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)
	fragment := writeFragment(t, `
resources:
  - kind: directory
    name: data-dir
    spec:
      path: /srv/data
`)

	stdout, _, code := runCLI(t, "dev", "merge", base, fragment)
	if code != ExitCodeSuccess {
		t.Fatalf("exit code = %d", code)
	}
	// When neither manifest has vars, the output should not contain a vars key.
	if strings.Contains(stdout, "vars:") {
		t.Fatalf("output should not contain vars when none are defined: %s", stdout)
	}
}

func TestMergeRequiresTwoArgs(t *testing.T) {
	_, stderr, code := runCLI(t, "dev", "merge")
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d\nstderr: %s", code, ExitCodeRuntimeError, stderr)
	}
	if !strings.Contains(stderr, "accepts 2 arg(s)") {
		t.Fatalf("stderr should mention arg count: %s", stderr)
	}
}

func TestMergeInvalidBasePath(t *testing.T) {
	fragment := writeFragment(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)

	_, stderr, code := runCLI(t, "dev", "merge", "/nonexistent/base.yaml", fragment)
	if code != ExitCodeRuntimeError {
		t.Fatalf("exit code = %d, want %d", code, ExitCodeRuntimeError)
	}
	if !strings.Contains(stderr, "base manifest") {
		t.Fatalf("stderr should mention base manifest: %s", stderr)
	}
}

func writeFragment(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fragment.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write fragment: %v", err)
	}
	return path
}
