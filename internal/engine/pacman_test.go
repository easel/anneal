package engine

import (
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func withMockPacman(installed map[string]bool, fn func()) {
	orig := pacmanQueryFunc
	pacmanQueryFunc = func(names []string) (map[string]bool, error) {
		result := map[string]bool{}
		for _, n := range names {
			if installed[n] {
				result[n] = true
			}
		}
		return result, nil
	}
	defer func() { pacmanQueryFunc = orig }()
	fn()
}

func TestPacmanPackagesInstallsMissing(t *testing.T) {
	withMockPacman(map[string]bool{"base": true}, func() {
		provider := pacmanPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "pacman_packages",
			Name: "base-pkgs",
			Spec: map[string]any{
				"packages": []any{"base", "git", "vim"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_pacman_install") {
			t.Fatalf("expected pacman install:\n%s", joined)
		}
		if !strings.Contains(joined, "'git'") || !strings.Contains(joined, "'vim'") {
			t.Fatalf("missing packages:\n%s", joined)
		}
		if strings.Contains(joined, "'base'") {
			t.Fatalf("base should not be installed (already present):\n%s", joined)
		}
	})
}

func TestPacmanPackagesConverged(t *testing.T) {
	withMockPacman(map[string]bool{"git": true, "vim": true}, func() {
		provider := pacmanPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "pacman_packages",
			Name: "tools",
			Spec: map[string]any{
				"packages": []any{"git", "vim"},
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

func TestPacmanPackagesAURHelper(t *testing.T) {
	withMockPacman(map[string]bool{}, func() {
		provider := pacmanPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "pacman_packages",
			Name: "aur-pkgs",
			Spec: map[string]any{
				"packages":   []any{"paru-bin", "visual-studio-code-bin"},
				"aur_helper": "paru",
				"user":       "erik",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_aur_install") {
			t.Fatalf("expected AUR install:\n%s", joined)
		}
		if !strings.Contains(joined, "'paru'") {
			t.Fatalf("expected AUR helper name in command:\n%s", joined)
		}
		if !strings.Contains(joined, "'erik'") {
			t.Fatalf("expected user in command:\n%s", joined)
		}
		// Should NOT use stdlib_pacman_install when aur_helper is set
		if strings.Contains(joined, "stdlib_pacman_install") {
			t.Fatalf("should use AUR helper, not pacman directly:\n%s", joined)
		}
	})
}

func TestPacmanPackagesAURHelperRequiresUser(t *testing.T) {
	withMockPacman(map[string]bool{}, func() {
		provider := pacmanPackagesProvider{}
		_, err := provider.Plan(manifest.ResolvedResource{
			Kind: "pacman_packages",
			Name: "aur-pkgs",
			Spec: map[string]any{
				"packages":   []any{"paru-bin"},
				"aur_helper": "paru",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing user with aur_helper")
		}
		if !strings.Contains(err.Error(), "user is required") {
			t.Fatalf("error = %q, want 'user is required'", err)
		}
	})
}

func TestPacmanPackagesValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing packages",
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
			withMockPacman(map[string]bool{}, func() {
				provider := pacmanPackagesProvider{}
				_, err := provider.Plan(manifest.ResolvedResource{
					Kind: "pacman_packages",
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

func TestPacmanPackagesBatchCount(t *testing.T) {
	withMockPacman(map[string]bool{}, func() {
		provider := pacmanPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "pacman_packages",
			Name: "many",
			Spec: map[string]any{
				"packages": []any{"a", "b", "c", "d"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "# install 4 package(s)") {
			t.Fatalf("expected batch count comment:\n%s", joined)
		}
	})
}
