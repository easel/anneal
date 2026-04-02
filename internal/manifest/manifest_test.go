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

func TestIncludeBasicResolution(t *testing.T) {
	dir := t.TempDir()

	// Write a module manifest
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `
vars:
  greeting: hello from module
resources:
  - kind: file
    name: base-motd
    spec:
      path: /etc/motd
      content: base
`)

	// Write root manifest that includes the module
	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
vars:
  env: prod
includes:
  - path: modules/base.yaml
resources:
  - kind: file
    name: app-config
    spec:
      path: /etc/app.conf
      content: app
`)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should have resources from both module and root
	if len(m.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(m.Resources))
	}
	// Module resources come first
	if m.Resources[0].Name != "base-motd" {
		t.Fatalf("first resource = %q, want base-motd", m.Resources[0].Name)
	}
	if m.Resources[1].Name != "app-config" {
		t.Fatalf("second resource = %q, want app-config", m.Resources[1].Name)
	}
}

func TestIncludeWithVarOverrides(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "module.yaml"), `
vars:
  port: "8080"
  host: localhost
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: placeholder
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: module.yaml
    vars:
      port: "9090"
resources:
  - kind: file
    name: app
    spec:
      path: /tmp/app
      content: ok
`)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Include-level vars override module defaults
	if got := m.Vars["port"]; got != "9090" {
		t.Fatalf("vars[port] = %v, want 9090", got)
	}
	// Module default preserved when not overridden
	if got := m.Vars["host"]; got != "localhost" {
		t.Fatalf("vars[host] = %v, want localhost", got)
	}
}

func TestIncludeNestedResolution(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "level2.yaml"), `
vars:
  deep: deep-val
resources:
  - kind: file
    name: deep-resource
    spec:
      path: /tmp/deep
      content: deep
`)

	writeFile(t, filepath.Join(dir, "level1.yaml"), `
vars:
  mid: mid-val
includes:
  - path: level2.yaml
resources:
  - kind: file
    name: mid-resource
    spec:
      path: /tmp/mid
      content: mid
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: level1.yaml
resources:
  - kind: file
    name: root-resource
    spec:
      path: /tmp/root
      content: root
`)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(m.Resources) != 3 {
		t.Fatalf("len(Resources) = %d, want 3", len(m.Resources))
	}
	// Order: deepest first → mid → root
	names := []string{m.Resources[0].Name, m.Resources[1].Name, m.Resources[2].Name}
	want := []string{"deep-resource", "mid-resource", "root-resource"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("resource order = %v, want %v", names, want)
	}
	// All vars should be merged
	if m.Vars["deep"] != "deep-val" {
		t.Fatalf("vars[deep] = %v, want deep-val", m.Vars["deep"])
	}
	if m.Vars["mid"] != "mid-val" {
		t.Fatalf("vars[mid] = %v, want mid-val", m.Vars["mid"])
	}
}

func TestIncludeCircularDetection(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "a.yaml"), `
includes:
  - path: b.yaml
resources:
  - kind: file
    name: a
    spec:
      path: /tmp/a
      content: a
`)

	writeFile(t, filepath.Join(dir, "b.yaml"), `
includes:
  - path: a.yaml
resources:
  - kind: file
    name: b
    spec:
      path: /tmp/b
      content: b
`)

	_, err := Load(filepath.Join(dir, "a.yaml"))
	if err == nil {
		t.Fatal("Load() expected circular include error")
	}
	if !strings.Contains(err.Error(), "circular include") {
		t.Fatalf("error = %q, want containing 'circular include'", err)
	}
	// Should show the cycle path
	if !strings.Contains(err.Error(), "→") {
		t.Fatalf("error = %q, want cycle path with →", err)
	}
}

func TestIncludeSelfReferenceDetection(t *testing.T) {
	dir := t.TempDir()

	self := writeFile(t, filepath.Join(dir, "self.yaml"), `
includes:
  - path: self.yaml
resources:
  - kind: file
    name: x
    spec:
      path: /tmp/x
      content: x
`)

	_, err := Load(self)
	if err == nil {
		t.Fatal("Load() expected circular include error for self-reference")
	}
	if !strings.Contains(err.Error(), "circular include") {
		t.Fatalf("error = %q, want containing 'circular include'", err)
	}
}

func TestIncludeRootVarsOverrideModuleDefaults(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "module.yaml"), `
vars:
  greeting: module-default
resources:
  - kind: file
    name: m
    spec:
      path: /tmp/m
      content: ok
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
vars:
  greeting: root-override
includes:
  - path: module.yaml
resources:
  - kind: file
    name: r
    spec:
      path: /tmp/r
      content: ok
`)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := m.Vars["greeting"]; got != "root-override" {
		t.Fatalf("vars[greeting] = %v, want root-override (root should override module)", got)
	}
}

func TestIncludeEmptyPathError(t *testing.T) {
	dir := t.TempDir()

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: ""
resources:
  - kind: file
    name: r
    spec:
      path: /tmp/r
      content: ok
`)

	_, err := Load(root)
	if err == nil {
		t.Fatal("Load() expected error for empty include path")
	}
	if !strings.Contains(err.Error(), "include path is required") {
		t.Fatalf("error = %q, want 'include path is required'", err)
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

// writeFile creates a file at the given absolute path, creating parent directories.
func writeFile(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}
