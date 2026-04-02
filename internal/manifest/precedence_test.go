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

func writeTempManifest(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "anneal.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
