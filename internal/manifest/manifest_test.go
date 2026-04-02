package manifest

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestLoadResolvedAppliesEnvOverridesAndBuiltins(t *testing.T) {
	path := writeManifest(t, `
vars:
  env_name: prod
  greeting: hello
resources:
  - kind: file
    name: "{{ .Hostname }}-motd"
    depends_on:
      - bootstrap
    spec:
      path: "/tmp/{{ .DebArch }}/{{ .env_name }}"
      content: "{{ .greeting }} from {{ .Hostname }} on {{ .KernelArch }}"
`)

	resolved, err := LoadResolved(path, ResolveOptions{
		Env: map[string]string{
			"ANNEAL_ENV_NAME": "staging",
			"ANNEAL_GREETING": "bonjour",
		},
		Builtins: Builtins{
			Hostname:   "timbuktu",
			DebArch:    "amd64",
			KernelArch: "x86_64",
		},
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	resource := resolved.Resources[0]
	if resource.Name != "timbuktu-motd" {
		t.Fatalf("resource name = %q, want %q", resource.Name, "timbuktu-motd")
	}
	if got := resource.Spec["path"]; got != "/tmp/amd64/staging" {
		t.Fatalf("spec.path = %v, want %q", got, "/tmp/amd64/staging")
	}
	if got := resource.Spec["content"]; got != "bonjour from timbuktu on x86_64" {
		t.Fatalf("spec.content = %v, want %q", got, "bonjour from timbuktu on x86_64")
	}
	if got := resolved.Vars["env_name"]; got != "staging" {
		t.Fatalf("resolved vars env_name = %v, want %q", got, "staging")
	}
	if !reflect.DeepEqual(resource.DependsOn, []string{"bootstrap"}) {
		t.Fatalf("depends_on = %#v, want bootstrap", resource.DependsOn)
	}
}

func TestLoadResolvedRejectsUndefinedTemplateVariable(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: motd
    spec:
      path: /tmp/motd
      content: "{{ .missing }}"
`)

	_, err := LoadResolved(path, ResolveOptions{})
	if err == nil {
		t.Fatal("LoadResolved() error = nil, want template resolution error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("LoadResolved() error = %q, want missing variable message", err)
	}
}

func TestEnvOverrideRequiresAnnealPrefix(t *testing.T) {
	path := writeManifest(t, `
vars:
  path: /app/data
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .path }}"
`)

	// Bare PATH in env must NOT override manifest var "path"
	resolved, err := LoadResolved(path, ResolveOptions{
		Env: map[string]string{
			"PATH": "/usr/bin:/bin",
		},
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}
	if got := resolved.Vars["path"]; got != "/app/data" {
		t.Fatalf("vars[path] = %v, want /app/data (bare PATH should not collide)", got)
	}

	// ANNEAL_PATH should override
	resolved2, err := LoadResolved(path, ResolveOptions{
		Env: map[string]string{
			"PATH":        "/usr/bin:/bin",
			"ANNEAL_PATH": "/overridden",
		},
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}
	if got := resolved2.Vars["path"]; got != "/overridden" {
		t.Fatalf("vars[path] = %v, want /overridden", got)
	}
}

func TestBuiltinsWithDefaultsFQDNUsesCallerHostname(t *testing.T) {
	b := Builtins{Hostname: "custom-host"}
	got := b.withDefaults()
	if got.FQDN != "custom-host" {
		t.Fatalf("FQDN = %q, want %q (caller-supplied Hostname)", got.FQDN, "custom-host")
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
