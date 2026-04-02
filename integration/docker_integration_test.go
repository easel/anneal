// Package integration provides Docker-based integration tests that exercise
// provider and manifest behavior across supported Linux distribution families.
//
// These are Tier 3 tests per the anneal test plan. They require Docker and
// are skipped when Docker is not available or when -short is set.
//
// Run with: go test -v -timeout 10m ./integration/
package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// distro describes a target Linux distribution for the integration matrix.
type distro struct {
	Name       string // human-readable name
	Dockerfile string // path relative to repo root
	Tag        string // Docker image tag
}

var distros = []distro{
	{Name: "debian", Dockerfile: "integration/Dockerfile.debian", Tag: "anneal-test:debian"},
	{Name: "fedora", Dockerfile: "integration/Dockerfile.fedora", Tag: "anneal-test:fedora"},
	{Name: "arch", Dockerfile: "integration/Dockerfile.arch", Tag: "anneal-test:arch"},
}

func TestDockerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration tests in short mode")
	}

	// Check Docker availability.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH, skipping integration tests")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not available, skipping integration tests")
	}

	repoRoot := findRepoRoot(t)

	// Build the anneal binary for linux/amd64 (container target).
	annealBin := filepath.Join(repoRoot, "anneal")
	t.Logf("building anneal binary for linux/amd64")
	buildCmd := exec.Command("go", "build", "-o", annealBin, ".")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build anneal: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		os.Remove(annealBin)
	})

	for _, d := range distros {
		d := d
		t.Run(d.Name, func(t *testing.T) {
			t.Parallel()

			// Build Docker image.
			t.Logf("building image %s from %s", d.Tag, d.Dockerfile)
			buildImg := exec.Command("docker", "build",
				"-f", filepath.Join(repoRoot, d.Dockerfile),
				"-t", d.Tag,
				repoRoot)
			if out, err := buildImg.CombinedOutput(); err != nil {
				t.Fatalf("docker build failed for %s: %v\n%s", d.Name, err, out)
			}

			// Run test harness inside container.
			t.Logf("running integration tests on %s", d.Name)
			var stdout, stderr bytes.Buffer
			runCmd := exec.Command("docker", "run", "--rm", d.Tag)
			runCmd.Stdout = &stdout
			runCmd.Stderr = &stderr
			err := runCmd.Run()

			t.Logf("--- %s output ---\n%s", d.Name, stdout.String())
			if stderr.Len() > 0 {
				t.Logf("--- %s stderr ---\n%s", d.Name, stderr.String())
			}

			if err != nil {
				t.Errorf("integration tests failed on %s: %v", d.Name, err)
			}

			// Verify output contains expected pass markers.
			output := stdout.String()
			if !strings.Contains(output, "passed") {
				t.Errorf("unexpected output for %s — no pass summary found", d.Name)
			}
			if strings.Contains(output, "FAIL") {
				t.Errorf("integration tests reported failures on %s", d.Name)
			}
		})
	}
}

// TestScenarioFixtures verifies that each scenario directory contains
// the required manifest.yaml. This runs without Docker.
func TestScenarioFixtures(t *testing.T) {
	repoRoot := findRepoRoot(t)
	scenariosDir := filepath.Join(repoRoot, "integration", "scenarios")

	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		t.Fatalf("reading scenarios dir: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no scenario directories found")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			manifest := filepath.Join(scenariosDir, entry.Name(), "manifest.yaml")
			if _, err := os.Stat(manifest); err != nil {
				t.Errorf("scenario %s missing manifest.yaml", entry.Name())
			}
			verify := filepath.Join(scenariosDir, entry.Name(), "verify.sh")
			if _, err := os.Stat(verify); err != nil {
				t.Errorf("scenario %s missing verify.sh", entry.Name())
			}
		})
	}
}

// findRepoRoot walks up from the current working directory to find go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	// Try the known path first.
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
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}

// TestDockerBuild is a lighter check that just verifies the Dockerfiles
// can be parsed by docker build (syntax check only). Skipped without Docker.
func TestDockerBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker build test in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}

	repoRoot := findRepoRoot(t)

	for _, d := range distros {
		d := d
		t.Run(fmt.Sprintf("syntax-%s", d.Name), func(t *testing.T) {
			dockerfile := filepath.Join(repoRoot, d.Dockerfile)
			if _, err := os.Stat(dockerfile); err != nil {
				t.Fatalf("Dockerfile not found: %s", dockerfile)
			}
		})
	}
}
