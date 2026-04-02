// Package smoke provides the Tier 5 screencast smoke proof as an automated
// Go test. It exercises the core operator workflow: validate → plan → apply →
// idempotent re-plan against a representative manifest with real providers.
//
// Run with: go test -v -timeout 2m ./smoke/
package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeProof(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke proof in short mode")
	}

	repoRoot := findRepoRoot(t)
	manifestPath := filepath.Join(repoRoot, "smoke", "manifest.yaml")
	smokeRoot := t.TempDir()

	// Build the anneal binary.
	annealBin := filepath.Join(t.TempDir(), "anneal")
	build := exec.Command("go", "build", "-o", annealBin, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Set the smoke_root variable via environment.
	env := append(os.Environ(), "ANNEAL_SMOKE_ROOT="+smokeRoot)

	// Step 1: validate
	t.Run("validate", func(t *testing.T) {
		cmd := exec.Command(annealBin, "validate", "-f", manifestPath)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("validate failed: %v\n%s", err, out)
		}
	})

	// Step 2: plan (should produce operations)
	t.Run("plan-initial", func(t *testing.T) {
		cmd := exec.Command(annealBin, "plan", "-f", manifestPath)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("plan failed: %v\n%s", err, out)
		}
		output := string(out)
		if strings.Contains(output, "# plan is empty") {
			t.Fatal("initial plan should not be empty")
		}
		if !strings.Contains(output, "stdlib_dir_create") {
			t.Error("plan missing directory creation")
		}
		if !strings.Contains(output, "stdlib_file_write") {
			t.Error("plan missing file write")
		}
		if !strings.Contains(output, "stdlib_symlink") {
			t.Error("plan missing symlink")
		}
		t.Logf("plan output:\n%s", output)
	})

	// Step 3: apply
	t.Run("apply", func(t *testing.T) {
		cmd := exec.Command(annealBin, "apply", "-f", manifestPath)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("apply failed: %v\n%s", err, out)
		}
		output := string(out)
		if !strings.Contains(output, "applied") {
			t.Errorf("apply output missing 'applied': %s", output)
		}
		t.Logf("apply output:\n%s", output)
	})

	// Step 4: verify state
	t.Run("verify-state", func(t *testing.T) {
		// Directory created
		if info, err := os.Stat(filepath.Join(smokeRoot, "data")); err != nil || !info.IsDir() {
			t.Error("directory data/ not created")
		}
		// File created with correct content
		content, err := os.ReadFile(filepath.Join(smokeRoot, "data", "config.txt"))
		if err != nil {
			t.Fatalf("config.txt not created: %v", err)
		}
		if !strings.Contains(string(content), "setting_a = enabled") {
			t.Errorf("config.txt content wrong: %s", content)
		}
		// Symlink created
		target, err := os.Readlink(filepath.Join(smokeRoot, "config-link"))
		if err != nil {
			t.Fatalf("symlink not created: %v", err)
		}
		expected := filepath.Join(smokeRoot, "data", "config.txt")
		if target != expected {
			t.Errorf("symlink target = %s, want %s", target, expected)
		}
		// Command flag file created
		if _, err := os.Stat(filepath.Join(smokeRoot, "data", "flag")); err != nil {
			t.Error("command flag file not created")
		}
	})

	// Step 5: idempotency (second plan should be empty)
	t.Run("idempotency", func(t *testing.T) {
		cmd := exec.Command(annealBin, "plan", "-f", manifestPath)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("second plan failed: %v\n%s", err, out)
		}
		output := string(out)
		if !strings.Contains(output, "# plan is empty") {
			t.Fatalf("second plan should be empty (idempotency):\n%s", output)
		}
	})
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat("/home/erik/Projects/anneal/go.mod"); err == nil {
		return "/home/erik/Projects/anneal"
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
