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

func TestIncludeDiamondDeduplication(t *testing.T) {
	dir := t.TempDir()

	// Shared module included by both A and B (diamond pattern)
	writeFile(t, filepath.Join(dir, "shared.yaml"), `
vars:
  shared_var: from-shared
resources:
  - kind: file
    name: shared-resource
    spec:
      path: /tmp/shared
      content: shared
`)

	writeFile(t, filepath.Join(dir, "a.yaml"), `
includes:
  - path: shared.yaml
resources:
  - kind: file
    name: a-resource
    spec:
      path: /tmp/a
      content: a
`)

	writeFile(t, filepath.Join(dir, "b.yaml"), `
includes:
  - path: shared.yaml
resources:
  - kind: file
    name: b-resource
    spec:
      path: /tmp/b
      content: b
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: a.yaml
  - path: b.yaml
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

	// shared-resource should appear only once (from the A branch, first to reach it)
	if len(m.Resources) != 4 {
		names := make([]string, len(m.Resources))
		for i, r := range m.Resources {
			names[i] = r.Name
		}
		t.Fatalf("len(Resources) = %d, want 4 (diamond dedup); names = %v", len(m.Resources), names)
	}

	// Order: shared (via a) → a → b → root
	wantNames := []string{"shared-resource", "a-resource", "b-resource", "root-resource"}
	for i, want := range wantNames {
		if m.Resources[i].Name != want {
			t.Errorf("resource[%d].Name = %q, want %q", i, m.Resources[i].Name, want)
		}
	}

	// Shared var should be present
	if got := m.Vars["shared_var"]; got != "from-shared" {
		t.Errorf("vars[shared_var] = %v, want from-shared", got)
	}
}

func TestIncludeDuplicateResourceNameError(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "module_a.yaml"), `
resources:
  - kind: file
    name: conflicting-name
    spec:
      path: /tmp/a
      content: a
`)

	writeFile(t, filepath.Join(dir, "module_b.yaml"), `
resources:
  - kind: file
    name: conflicting-name
    spec:
      path: /tmp/b
      content: b
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: module_a.yaml
  - path: module_b.yaml
resources:
  - kind: file
    name: root-resource
    spec:
      path: /tmp/root
      content: root
`)

	_, err := Load(root)
	if err == nil {
		t.Fatal("Load() expected error for duplicate resource name")
	}
	if !strings.Contains(err.Error(), "duplicate resource name") {
		t.Fatalf("error = %q, want containing 'duplicate resource name'", err)
	}
	if !strings.Contains(err.Error(), "conflicting-name") {
		t.Fatalf("error = %q, want containing resource name 'conflicting-name'", err)
	}
}

func TestIncludeDiamondWithVarOverrides(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "shared.yaml"), `
vars:
  port: "8080"
resources:
  - kind: file
    name: shared
    spec:
      path: /tmp/shared
      content: ok
`)

	writeFile(t, filepath.Join(dir, "a.yaml"), `
includes:
  - path: shared.yaml
    vars:
      port: "9090"
resources:
  - kind: file
    name: a
    spec:
      path: /tmp/a
      content: ok
`)

	writeFile(t, filepath.Join(dir, "b.yaml"), `
includes:
  - path: shared.yaml
    vars:
      port: "7070"
resources:
  - kind: file
    name: b
    spec:
      path: /tmp/b
      content: ok
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: a.yaml
  - path: b.yaml
resources:
  - kind: file
    name: root
    spec:
      path: /tmp/root
      content: ok
`)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// First branch (a) reaches shared first, so a's override of port wins
	if got := m.Vars["port"]; got != "9090" {
		t.Errorf("vars[port] = %v, want 9090 (first branch override wins)", got)
	}
}

func TestIncludeThreeLevelDiamond(t *testing.T) {
	dir := t.TempDir()

	// D is shared at the bottom of a three-level diamond: root→A→D, root→B→C→D
	writeFile(t, filepath.Join(dir, "d.yaml"), `
vars:
  d_var: from-d
resources:
  - kind: file
    name: d-resource
    spec:
      path: /tmp/d
      content: d
`)

	writeFile(t, filepath.Join(dir, "c.yaml"), `
includes:
  - path: d.yaml
resources:
  - kind: file
    name: c-resource
    spec:
      path: /tmp/c
      content: c
`)

	writeFile(t, filepath.Join(dir, "a.yaml"), `
includes:
  - path: d.yaml
resources:
  - kind: file
    name: a-resource
    spec:
      path: /tmp/a
      content: a
`)

	writeFile(t, filepath.Join(dir, "b.yaml"), `
includes:
  - path: c.yaml
resources:
  - kind: file
    name: b-resource
    spec:
      path: /tmp/b
      content: b
`)

	root := writeFile(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: a.yaml
  - path: b.yaml
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

	// D's resources appear once (via A, the first branch to reach it)
	// Order: d (via a) → a → c (d deduped) → b → root
	if len(m.Resources) != 5 {
		names := make([]string, len(m.Resources))
		for i, r := range m.Resources {
			names[i] = r.Name
		}
		t.Fatalf("len(Resources) = %d, want 5; names = %v", len(m.Resources), names)
	}

	wantNames := []string{"d-resource", "a-resource", "c-resource", "b-resource", "root-resource"}
	for i, want := range wantNames {
		if m.Resources[i].Name != want {
			t.Errorf("resource[%d].Name = %q, want %q", i, m.Resources[i].Name, want)
		}
	}
}

func TestEachExpandsToMultipleResources(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "config-{{ .Item }}"
    each:
      - alpha
      - beta
      - gamma
    spec:
      path: "/etc/{{ .Item }}.conf"
      content: "data for {{ .Item }}"
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 3 {
		t.Fatalf("len(Resources) = %d, want 3", len(resolved.Resources))
	}

	wantNames := []string{"config-alpha", "config-beta", "config-gamma"}
	for i, want := range wantNames {
		if resolved.Resources[i].Name != want {
			t.Errorf("resource[%d].Name = %q, want %q", i, resolved.Resources[i].Name, want)
		}
	}
	// Check spec rendering with .Item
	if got := resolved.Resources[0].Spec["path"]; got != "/etc/alpha.conf" {
		t.Errorf("resource[0].spec.path = %v, want /etc/alpha.conf", got)
	}
	if got := resolved.Resources[1].Spec["content"]; got != "data for beta" {
		t.Errorf("resource[1].spec.content = %v, want 'data for beta'", got)
	}
}

func TestEachWithIndex(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "item-{{ .Index }}"
    each:
      - first
      - second
    spec:
      path: "/tmp/{{ .Index }}"
      content: "{{ .Item }} at {{ .Index }}"
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(resolved.Resources))
	}
	if resolved.Resources[0].Name != "item-0" {
		t.Errorf("resource[0].Name = %q, want item-0", resolved.Resources[0].Name)
	}
	if resolved.Resources[1].Name != "item-1" {
		t.Errorf("resource[1].Name = %q, want item-1", resolved.Resources[1].Name)
	}
	if got := resolved.Resources[0].Spec["content"]; got != "first at 0" {
		t.Errorf("content = %v, want 'first at 0'", got)
	}
}

func TestEachEmptyListProducesZeroResources(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "will-not-exist"
    each: []
    spec:
      path: /tmp/x
      content: x
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 0 {
		t.Fatalf("len(Resources) = %d, want 0 (empty each)", len(resolved.Resources))
	}
}

func TestEachWithComplexItems(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "svc-{{ .Item.name }}"
    each:
      - name: web
        port: "8080"
      - name: api
        port: "9090"
    spec:
      path: "/etc/{{ .Item.name }}.conf"
      content: "port={{ .Item.port }}"
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(resolved.Resources))
	}
	if resolved.Resources[0].Name != "svc-web" {
		t.Errorf("resource[0].Name = %q, want svc-web", resolved.Resources[0].Name)
	}
	if got := resolved.Resources[1].Spec["content"]; got != "port=9090" {
		t.Errorf("resource[1].spec.content = %v, want 'port=9090'", got)
	}
}

func TestEachMixedWithNonIteratorResources(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: static-resource
    spec:
      path: /tmp/static
      content: static
  - kind: file
    name: "iter-{{ .Item }}"
    each:
      - a
      - b
    spec:
      path: "/tmp/{{ .Item }}"
      content: iter
  - kind: file
    name: another-static
    spec:
      path: /tmp/another
      content: static
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 4 {
		t.Fatalf("len(Resources) = %d, want 4", len(resolved.Resources))
	}
	wantNames := []string{"static-resource", "iter-a", "iter-b", "another-static"}
	for i, want := range wantNames {
		if resolved.Resources[i].Name != want {
			t.Errorf("resource[%d].Name = %q, want %q", i, resolved.Resources[i].Name, want)
		}
	}
}

func TestEachWithTemplateExpressionsInItems(t *testing.T) {
	path := writeManifest(t, `
vars:
  prefix: srv
resources:
  - kind: file
    name: "config-{{ .Item }}"
    each:
      - "{{ .prefix }}-web"
      - "{{ .prefix }}-db"
    spec:
      path: "/etc/{{ .Item }}.conf"
      content: ok
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(resolved.Resources))
	}
	// Pass 1 renders each items, pass 2 renders name/spec
	if resolved.Resources[0].Name != "config-srv-web" {
		t.Errorf("resource[0].Name = %q, want config-srv-web", resolved.Resources[0].Name)
	}
	if resolved.Resources[1].Name != "config-srv-db" {
		t.Errorf("resource[1].Name = %q, want config-srv-db", resolved.Resources[1].Name)
	}
}

func TestEachWithBuiltinsInItems(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "host-{{ .Item }}"
    each:
      - "{{ .Hostname }}-app"
    spec:
      path: "/tmp/{{ .Item }}"
      content: ok
`)

	resolved, err := LoadResolved(path, ResolveOptions{
		Builtins: Builtins{Hostname: "myhost"},
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(resolved.Resources))
	}
	if resolved.Resources[0].Name != "host-myhost-app" {
		t.Errorf("resource[0].Name = %q, want host-myhost-app", resolved.Resources[0].Name)
	}
}

func TestEachDependsOnWithItem(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: "dep-{{ .Item }}"
    each:
      - alpha
      - beta
    depends_on:
      - "base-{{ .Item }}"
    spec:
      path: /tmp/x
      content: ok
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	if len(resolved.Resources) != 2 {
		t.Fatalf("len(Resources) = %d, want 2", len(resolved.Resources))
	}
	if !reflect.DeepEqual(resolved.Resources[0].DependsOn, []string{"base-alpha"}) {
		t.Errorf("resource[0].DependsOn = %v, want [base-alpha]", resolved.Resources[0].DependsOn)
	}
	if !reflect.DeepEqual(resolved.Resources[1].DependsOn, []string{"base-beta"}) {
		t.Errorf("resource[1].DependsOn = %v, want [base-beta]", resolved.Resources[1].DependsOn)
	}
}

func TestEachDeclarationOrder(t *testing.T) {
	path := writeManifest(t, `
resources:
  - kind: file
    name: first
    spec:
      path: /tmp/first
      content: ok
  - kind: file
    name: "iter-{{ .Item }}"
    each:
      - a
      - b
      - c
    spec:
      path: /tmp/x
      content: ok
  - kind: file
    name: last
    spec:
      path: /tmp/last
      content: ok
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	// DeclarationOrder should be sequential across expanded resources
	for i, r := range resolved.Resources {
		if r.DeclarationOrder != i {
			t.Errorf("resource[%d] (%s) DeclarationOrder = %d, want %d", i, r.Name, r.DeclarationOrder, i)
		}
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
