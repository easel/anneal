package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariablePrecedenceMatrix(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		env      map[string]string
		builtins Builtins
		wantVars map[string]any
		wantErr  string
	}{
		{
			name: "manifest defaults only",
			manifest: `
vars:
  greeting: hello
  env_name: prod
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .greeting }}"`,
			wantVars: map[string]any{
				"greeting": "hello",
				"env_name": "prod",
			},
		},
		{
			name: "ANNEAL_ env overrides manifest var",
			manifest: `
vars:
  env_name: prod
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .env_name }}"`,
			env: map[string]string{
				"ANNEAL_ENV_NAME": "staging",
			},
			wantVars: map[string]any{
				"env_name": "staging",
			},
		},
		{
			name: "bare env var does NOT override (collision protection)",
			manifest: `
vars:
  path: /app/data
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .path }}"`,
			env: map[string]string{
				"PATH": "/usr/bin:/bin",
			},
			wantVars: map[string]any{
				"path": "/app/data",
			},
		},
		{
			name: "ANNEAL_ prefix wins over bare env",
			manifest: `
vars:
  home: /app
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .home }}"`,
			env: map[string]string{
				"HOME":        "/root",
				"ANNEAL_HOME": "/override",
			},
			wantVars: map[string]any{
				"home": "/override",
			},
		},
		{
			name: "builtins available in templates",
			manifest: `
vars:
  greeting: hello
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .greeting }} from {{ .Hostname }} ({{ .Arch }})"`,
			builtins: Builtins{
				Hostname: "testhost",
				Arch:     "amd64",
			},
			wantVars: map[string]any{
				"greeting": "hello",
			},
		},
		{
			name: "env override with underscore normalization",
			manifest: `
vars:
  my-hyphenated-var: original
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: ok`,
			env: map[string]string{
				"ANNEAL_MY_HYPHENATED_VAR": "overridden",
			},
			wantVars: map[string]any{
				"my-hyphenated-var": "overridden",
			},
		},
		{
			name: "env does not introduce new vars",
			manifest: `
vars:
  known: value
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .known }}"`,
			env: map[string]string{
				"ANNEAL_UNKNOWN": "surprise",
			},
			wantVars: map[string]any{
				"known": "value",
			},
		},
		{
			name: "undefined template variable is an error",
			manifest: `
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .undefined_var }}"`,
			wantErr: "undefined_var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempManifest(t, tt.manifest)
			resolved, err := LoadResolved(path, ResolveOptions{
				Env:      tt.env,
				Builtins: tt.builtins,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadResolved() error = %v", err)
			}
			for key, want := range tt.wantVars {
				got, ok := resolved.Vars[key]
				if !ok {
					t.Errorf("vars[%q] not found", key)
					continue
				}
				if got != want {
					t.Errorf("vars[%q] = %v, want %v", key, got, want)
				}
			}
		})
	}
}

func TestFullPrecedenceChain(t *testing.T) {
	dir := t.TempDir()

	// Module with defaults
	writeFileForTest(t, filepath.Join(dir, "module.yaml"), `
vars:
  a: module-default-a
  b: module-default-b
  c: module-default-c
  d: module-default-d
resources:
  - kind: file
    name: m
    spec:
      path: /tmp/m
      content: ok
`)

	// Host vars file
	writeFileForTest(t, filepath.Join(dir, "host.yaml"), `
c: host-override-c
d: host-override-d
`)

	// Root manifest overrides b
	writeFileForTest(t, filepath.Join(dir, "root.yaml"), `
vars:
  b: root-override-b
  c: root-override-c
  d: root-override-d
includes:
  - path: module.yaml
resources:
  - kind: file
    name: r
    spec:
      path: /tmp/r
      content: ok
`)

	resolved, err := LoadResolved(filepath.Join(dir, "root.yaml"), ResolveOptions{
		Env: map[string]string{
			"ANNEAL_D": "env-override-d",
		},
		HostVarsFile: filepath.Join(dir, "host.yaml"),
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	// a: only module default (nothing overrides it)
	if got := resolved.Vars["a"]; got != "module-default-a" {
		t.Errorf("vars[a] = %v, want module-default-a", got)
	}
	// b: root overrides module default
	if got := resolved.Vars["b"]; got != "root-override-b" {
		t.Errorf("vars[b] = %v, want root-override-b", got)
	}
	// c: host file overrides root
	if got := resolved.Vars["c"]; got != "host-override-c" {
		t.Errorf("vars[c] = %v, want host-override-c", got)
	}
	// d: env overrides host file (and everything else)
	if got := resolved.Vars["d"]; got != "env-override-d" {
		t.Errorf("vars[d] = %v, want env-override-d", got)
	}
}

func TestHostVarsFileNotFound(t *testing.T) {
	path := writeTempManifest(t, `
vars:
  x: val
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: ok`)

	_, err := LoadResolved(path, ResolveOptions{
		HostVarsFile: "/nonexistent/host.yaml",
	})
	if err == nil {
		t.Fatal("expected error for missing host vars file")
	}
	if !strings.Contains(err.Error(), "host vars") {
		t.Fatalf("error = %q, want containing 'host vars'", err)
	}
}

func TestHostVarsCanIntroduceNewVars(t *testing.T) {
	dir := t.TempDir()

	writeFileForTest(t, filepath.Join(dir, "host.yaml"), `
host_specific: from-host
`)

	path := writeFileForTest(t, filepath.Join(dir, "root.yaml"), `
vars:
  base: val
resources:
  - kind: file
    name: cfg
    spec:
      path: /tmp/cfg
      content: "{{ .base }} {{ .host_specific }}"
`)

	resolved, err := LoadResolved(path, ResolveOptions{
		HostVarsFile: filepath.Join(dir, "host.yaml"),
	})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}
	if got := resolved.Vars["host_specific"]; got != "from-host" {
		t.Errorf("vars[host_specific] = %v, want from-host", got)
	}
	// Verify template rendering works with host var
	if got := resolved.Resources[0].Spec["content"]; got != "val from-host" {
		t.Errorf("content = %v, want 'val from-host'", got)
	}
}

func TestIncludeVarsInTemplates(t *testing.T) {
	dir := t.TempDir()

	writeFileForTest(t, filepath.Join(dir, "module.yaml"), `
vars:
  greeting: default
resources:
  - kind: file
    name: m
    spec:
      path: /tmp/m
      content: "{{ .greeting }}"
`)

	path := writeFileForTest(t, filepath.Join(dir, "root.yaml"), `
includes:
  - path: module.yaml
    vars:
      greeting: overridden
resources:
  - kind: file
    name: r
    spec:
      path: /tmp/r
      content: ok
`)

	resolved, err := LoadResolved(path, ResolveOptions{})
	if err != nil {
		t.Fatalf("LoadResolved() error = %v", err)
	}

	// Module resource should render with overridden var
	if got := resolved.Resources[0].Spec["content"]; got != "overridden" {
		t.Fatalf("module content = %v, want 'overridden'", got)
	}
}

func writeFileForTest(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func writeTempManifest(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "anneal.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
