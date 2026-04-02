package engine

import (
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
