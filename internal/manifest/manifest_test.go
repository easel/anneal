package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidManifest(t *testing.T) {
	path := writeManifest(t, `
vars:
  env: prod
resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`)

	manifest, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(manifest.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(manifest.Resources))
	}
	if got := manifest.Resources[0].Kind; got != "file" {
		t.Fatalf("resource kind = %q, want %q", got, "file")
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: broken
    spec:
      path: [unterminated
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want syntax error")
	}
	if !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("Load() error = %q, want manifest path", err)
	}
}

func TestLoadRejectsResourceWithoutKind(t *testing.T) {
	path := writeManifest(t, `
resources:
  - name: motd
    spec:
      path: /etc/motd
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "kind is required") {
		t.Fatalf("Load() error = %q, want kind validation", err)
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
