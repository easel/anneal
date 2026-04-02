package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

// mockDpkg returns a dpkg query function that reports the given packages as installed.
func mockDpkg(installed map[string]bool) func([]string) (map[string]bool, error) {
	return func(names []string) (map[string]bool, error) {
		result := map[string]bool{}
		for _, n := range names {
			if installed[n] {
				result[n] = true
			}
		}
		return result, nil
	}
}

func withMockDpkg(installed map[string]bool, fn func()) {
	orig := dpkgQueryFunc
	dpkgQueryFunc = mockDpkg(installed)
	defer func() { dpkgQueryFunc = orig }()
	fn()
}

func TestAptPackagesInstallsMissing(t *testing.T) {
	withMockDpkg(map[string]bool{"curl": true}, func() {
		provider := aptPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_packages",
			Name: "base-pkgs",
			Spec: map[string]any{
				"packages": []any{"curl", "wget", "git"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) == 0 {
			t.Fatal("expected install operations for missing packages")
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_apt_install") {
			t.Fatalf("expected stdlib_apt_install in ops:\n%s", joined)
		}
		// wget and git should be in the install, curl should not
		if !strings.Contains(joined, "'wget'") {
			t.Fatalf("missing 'wget' in ops:\n%s", joined)
		}
		if !strings.Contains(joined, "'git'") {
			t.Fatalf("missing 'git' in ops:\n%s", joined)
		}
		if strings.Contains(joined, "'curl'") {
			t.Fatalf("curl should NOT be in install (already installed):\n%s", joined)
		}
		// Should batch into one call
		if strings.Count(joined, "stdlib_apt_install") != 1 {
			t.Fatalf("expected single batched apt install call:\n%s", joined)
		}
	})
}

func TestAptPackagesConverged(t *testing.T) {
	withMockDpkg(map[string]bool{"curl": true, "wget": true}, func() {
		provider := aptPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_packages",
			Name: "base-pkgs",
			Spec: map[string]any{
				"packages": []any{"curl", "wget"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no operations (all installed), got: %v", ops)
		}
	})
}

func TestAptPackagesWithDebconf(t *testing.T) {
	withMockDpkg(map[string]bool{}, func() {
		provider := aptPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_packages",
			Name: "krb5",
			Spec: map[string]any{
				"packages": []any{"krb5-user"},
				"debconf": []any{
					map[string]any{
						"package":  "krb5-config",
						"question": "krb5-config/default_realm",
						"type":     "string",
						"value":    "EXAMPLE.COM",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		// Debconf should come BEFORE install
		debconfPos := strings.Index(joined, "stdlib_debconf_set")
		installPos := strings.Index(joined, "stdlib_apt_install")
		if debconfPos < 0 {
			t.Fatalf("expected debconf preseed in ops:\n%s", joined)
		}
		if installPos < 0 {
			t.Fatalf("expected apt install in ops:\n%s", joined)
		}
		if debconfPos > installPos {
			t.Fatalf("debconf must come before install:\n%s", joined)
		}
		if !strings.Contains(joined, "EXAMPLE.COM") {
			t.Fatalf("debconf value missing in ops:\n%s", joined)
		}
	})
}

func TestAptPackagesDebconfNotEmittedWhenConverged(t *testing.T) {
	// All packages already installed → no ops at all (including no debconf)
	withMockDpkg(map[string]bool{"krb5-user": true}, func() {
		provider := aptPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_packages",
			Name: "krb5",
			Spec: map[string]any{
				"packages": []any{"krb5-user"},
				"debconf": []any{
					map[string]any{
						"package":  "krb5-config",
						"question": "krb5-config/default_realm",
						"type":     "string",
						"value":    "EXAMPLE.COM",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no operations (package already installed), got: %v", ops)
		}
	})
}

func TestAptPurgeRemovesInstalled(t *testing.T) {
	withMockDpkg(map[string]bool{"telnet": true, "ftp": true}, func() {
		provider := aptPurgeProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_purge",
			Name: "remove-insecure",
			Spec: map[string]any{
				"packages": []any{"telnet", "ftp", "rsh-client"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_apt_purge") {
			t.Fatalf("expected stdlib_apt_purge in ops:\n%s", joined)
		}
		if !strings.Contains(joined, "'telnet'") {
			t.Fatalf("missing telnet in purge:\n%s", joined)
		}
		if !strings.Contains(joined, "'ftp'") {
			t.Fatalf("missing ftp in purge:\n%s", joined)
		}
		// rsh-client is not installed, should not be in purge
		if strings.Contains(joined, "'rsh-client'") {
			t.Fatalf("rsh-client should NOT be in purge (not installed):\n%s", joined)
		}
	})
}

func TestAptPurgeConverged(t *testing.T) {
	withMockDpkg(map[string]bool{}, func() {
		provider := aptPurgeProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_purge",
			Name: "remove-insecure",
			Spec: map[string]any{
				"packages": []any{"telnet", "ftp"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no operations (all absent), got: %v", ops)
		}
	})
}

func TestAptPackagesValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing packages field",
			spec:    map[string]any{},
			wantErr: "packages is required",
		},
		{
			name:    "packages not a list",
			spec:    map[string]any{"packages": "not-a-list"},
			wantErr: "must be a list",
		},
		{
			name:    "empty package name",
			spec:    map[string]any{"packages": []any{""}},
			wantErr: "non-empty strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockDpkg(map[string]bool{}, func() {
				provider := aptPackagesProvider{}
				_, err := provider.Plan(manifest.ResolvedResource{
					Kind: "apt_packages",
					Name: "test",
					Spec: tt.spec,
				})
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
			})
		})
	}
}

func TestAptPurgeValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing packages field",
			spec:    map[string]any{},
			wantErr: "packages is required",
		},
		{
			name:    "packages not a list",
			spec:    map[string]any{"packages": "not-a-list"},
			wantErr: "must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockDpkg(map[string]bool{}, func() {
				provider := aptPurgeProvider{}
				_, err := provider.Plan(manifest.ResolvedResource{
					Kind: "apt_purge",
					Name: "test",
					Spec: tt.spec,
				})
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
			})
		})
	}
}

func TestAptPackagesBatchCount(t *testing.T) {
	withMockDpkg(map[string]bool{}, func() {
		provider := aptPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_packages",
			Name: "many",
			Spec: map[string]any{
				"packages": []any{"a", "b", "c", "d", "e"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "# install 5 package(s)") {
			t.Fatalf("expected batch count comment, got:\n%s", joined)
		}
	})
}

// --- apt_repo tests ---

type mockFS struct {
	files map[string]string // path → content
}

func withMockFS(files map[string]string, fn func()) {
	origExists := fileExistsFunc
	origRead := readFileFunc
	fileExistsFunc = func(path string) bool {
		_, ok := files[path]
		return ok
	}
	readFileFunc = func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, &os.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
		}
		return []byte(content), nil
	}
	defer func() {
		fileExistsFunc = origExists
		readFileFunc = origRead
	}()
	fn()
}

func withMockDpkgVersion(versions map[string]string, fn func()) {
	orig := dpkgVersionFunc
	dpkgVersionFunc = func(name string) string {
		return versions[name]
	}
	defer func() { dpkgVersionFunc = orig }()
	fn()
}

func TestAptRepoNewRepoNeedsKeyAndSource(t *testing.T) {
	withMockFS(map[string]string{}, func() {
		provider := aptRepoProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_repo",
			Name: "docker-repo",
			Spec: map[string]any{
				"key_url":     "https://download.docker.com/linux/ubuntu/gpg",
				"keyring":     "/usr/share/keyrings/docker-archive-keyring.gpg",
				"source":      "deb [signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu jammy stable",
				"source_file": "/etc/apt/sources.list.d/docker.list",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_apt_key_add") {
			t.Fatalf("expected key add:\n%s", joined)
		}
		if !strings.Contains(joined, "stdlib_apt_source_add") {
			t.Fatalf("expected source add:\n%s", joined)
		}
	})
}

func TestAptRepoConvergedProducesNoOps(t *testing.T) {
	sourceLine := "deb [signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu jammy stable"
	withMockFS(map[string]string{
		"/usr/share/keyrings/docker-archive-keyring.gpg": "keydata",
		"/etc/apt/sources.list.d/docker.list":            sourceLine,
	}, func() {
		provider := aptRepoProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_repo",
			Name: "docker-repo",
			Spec: map[string]any{
				"key_url":     "https://download.docker.com/linux/ubuntu/gpg",
				"keyring":     "/usr/share/keyrings/docker-archive-keyring.gpg",
				"source":      sourceLine,
				"source_file": "/etc/apt/sources.list.d/docker.list",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no ops (converged), got: %v", ops)
		}
	})
}

func TestAptRepoKeyExistsButSourceDiffers(t *testing.T) {
	withMockFS(map[string]string{
		"/usr/share/keyrings/docker.gpg":      "keydata",
		"/etc/apt/sources.list.d/docker.list": "old source line",
	}, func() {
		provider := aptRepoProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_repo",
			Name: "docker-repo",
			Spec: map[string]any{
				"key_url":     "https://example.com/gpg",
				"keyring":     "/usr/share/keyrings/docker.gpg",
				"source":      "new source line",
				"source_file": "/etc/apt/sources.list.d/docker.list",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		// Key exists, should not re-add
		if strings.Contains(joined, "stdlib_apt_key_add") {
			t.Fatalf("should not re-add existing key:\n%s", joined)
		}
		// Source differs, should update
		if !strings.Contains(joined, "stdlib_apt_source_add") {
			t.Fatalf("should update source file:\n%s", joined)
		}
	})
}

func TestAptRepoWithoutKey(t *testing.T) {
	withMockFS(map[string]string{}, func() {
		provider := aptRepoProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "apt_repo",
			Name: "local-repo",
			Spec: map[string]any{
				"source":      "deb file:///opt/repo ./",
				"source_file": "/etc/apt/sources.list.d/local.list",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if strings.Contains(joined, "stdlib_apt_key_add") {
			t.Fatalf("should not add key when key_url not specified:\n%s", joined)
		}
		if !strings.Contains(joined, "stdlib_apt_source_add") {
			t.Fatalf("should add source:\n%s", joined)
		}
	})
}

func TestAptRepoValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing source",
			spec:    map[string]any{"source_file": "/etc/apt/sources.list.d/x.list"},
			wantErr: "source is required",
		},
		{
			name:    "missing source_file",
			spec:    map[string]any{"source": "deb http://example.com ./"},
			wantErr: "source_file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := aptRepoProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "apt_repo",
				Name: "test",
				Spec: tt.spec,
			})
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// --- deb_install tests ---

func TestDebInstallNewPackage(t *testing.T) {
	withMockDpkgVersion(map[string]string{}, func() {
		provider := debInstallProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "deb_install",
			Name: "grafana",
			Spec: map[string]any{
				"url":     "https://dl.grafana.com/oss/release/grafana_10.0.0_amd64.deb",
				"package": "grafana",
				"version": "10.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_deb_install") {
			t.Fatalf("expected deb install:\n%s", joined)
		}
		if !strings.Contains(joined, "# install grafana 10.0.0") {
			t.Fatalf("expected install comment:\n%s", joined)
		}
	})
}

func TestDebInstallConvergedVersion(t *testing.T) {
	withMockDpkgVersion(map[string]string{"grafana": "10.0.0"}, func() {
		provider := debInstallProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "deb_install",
			Name: "grafana",
			Spec: map[string]any{
				"url":     "https://dl.grafana.com/oss/release/grafana_10.0.0_amd64.deb",
				"package": "grafana",
				"version": "10.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no ops (already at version), got: %v", ops)
		}
	})
}

func TestDebInstallVersionUpgrade(t *testing.T) {
	withMockDpkgVersion(map[string]string{"grafana": "9.5.0"}, func() {
		provider := debInstallProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "deb_install",
			Name: "grafana",
			Spec: map[string]any{
				"url":     "https://dl.grafana.com/oss/release/grafana_10.0.0_amd64.deb",
				"package": "grafana",
				"version": "10.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "# upgrade grafana: 9.5.0 → 10.0.0") {
			t.Fatalf("expected upgrade comment:\n%s", joined)
		}
		if !strings.Contains(joined, "stdlib_deb_install") {
			t.Fatalf("expected deb install:\n%s", joined)
		}
	})
}

func TestDebInstallValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing url",
			spec:    map[string]any{"package": "pkg", "version": "1.0"},
			wantErr: "url is required",
		},
		{
			name:    "missing package",
			spec:    map[string]any{"url": "http://x/y.deb", "version": "1.0"},
			wantErr: "package is required",
		},
		{
			name:    "missing version",
			spec:    map[string]any{"url": "http://x/y.deb", "package": "pkg"},
			wantErr: "version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := debInstallProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "deb_install",
				Name: "test",
				Spec: tt.spec,
			})
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestAptRepoDependencyOrdering verifies repo resources are ordered before packages via DAG.
func TestAptRepoDependencyOrdering(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")

	withMockFS(map[string]string{}, func() {
		withMockDpkg(map[string]bool{}, func() {
			planner := NewPlanner()
			resources := []manifest.ResolvedResource{
				{
					Kind: "apt_packages", Name: "docker-pkgs",
					DependsOn:        []string{"docker-repo"},
					DeclarationOrder: 0,
					Spec: map[string]any{
						"packages": []any{"docker-ce"},
					},
				},
				{
					Kind: "apt_repo", Name: "docker-repo",
					DeclarationOrder: 1,
					Spec: map[string]any{
						"source":      "deb https://download.docker.com/linux/ubuntu jammy stable",
						"source_file": filepath.Join(dir, "docker.list"),
					},
				},
			}

			plan, err := planner.BuildPlan(resources)
			if err != nil {
				t.Fatalf("BuildPlan() error = %v", err)
			}

			_ = pathA
			// Repo should come before packages in the plan
			repoIdx, pkgIdx := -1, -1
			for i, rp := range plan.Resources {
				if rp.Name == "docker-repo" {
					repoIdx = i
				}
				if rp.Name == "docker-pkgs" {
					pkgIdx = i
				}
			}
			if repoIdx < 0 || pkgIdx < 0 {
				t.Fatal("expected both resources in plan")
			}
			if repoIdx > pkgIdx {
				t.Fatalf("repo (idx %d) should come before packages (idx %d)", repoIdx, pkgIdx)
			}
		})
	})
}
