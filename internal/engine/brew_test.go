package engine

import (
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func withMockBrew(formulae, casks map[string]bool, fn func()) {
	orig := brewListFunc
	brewListFunc = func(user string, cask bool) (map[string]bool, error) {
		if cask {
			return casks, nil
		}
		return formulae, nil
	}
	defer func() { brewListFunc = orig }()
	fn()
}

func withMockBrewTaps(taps map[string]bool, fn func()) {
	orig := brewTapListFunc
	brewTapListFunc = func(user string) (map[string]bool, error) {
		return taps, nil
	}
	defer func() { brewTapListFunc = orig }()
	fn()
}

func TestBrewPackagesInstallsMissing(t *testing.T) {
	withMockBrew(map[string]bool{"git": true}, map[string]bool{}, func() {
		provider := brewPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "brew_packages",
			Name: "dev-tools",
			Spec: map[string]any{
				"user":     "erik",
				"packages": []any{"git", "jq", "ripgrep"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_brew_install") {
			t.Fatalf("expected brew install:\n%s", joined)
		}
		if !strings.Contains(joined, "'jq'") {
			t.Fatalf("missing jq:\n%s", joined)
		}
		if !strings.Contains(joined, "'ripgrep'") {
			t.Fatalf("missing ripgrep:\n%s", joined)
		}
		// git already installed
		if strings.Contains(joined, "'git'") {
			t.Fatalf("git should not be in install (already installed):\n%s", joined)
		}
		// Should run as user
		if !strings.Contains(joined, "'erik'") {
			t.Fatalf("expected user 'erik' in install command:\n%s", joined)
		}
	})
}

func TestBrewPackagesConverged(t *testing.T) {
	withMockBrew(map[string]bool{"jq": true, "ripgrep": true}, map[string]bool{}, func() {
		provider := brewPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "brew_packages",
			Name: "tools",
			Spec: map[string]any{
				"user":     "erik",
				"packages": []any{"jq", "ripgrep"},
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

func TestBrewPackagesCask(t *testing.T) {
	withMockBrew(map[string]bool{}, map[string]bool{"firefox": true}, func() {
		provider := brewPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "brew_packages",
			Name: "browsers",
			Spec: map[string]any{
				"user":     "erik",
				"cask":     true,
				"packages": []any{"firefox", "chromium"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "--cask") {
			t.Fatalf("expected --cask flag:\n%s", joined)
		}
		if !strings.Contains(joined, "'chromium'") {
			t.Fatalf("missing chromium:\n%s", joined)
		}
		if strings.Contains(joined, "'firefox'") {
			t.Fatalf("firefox should not be installed (already present):\n%s", joined)
		}
		if !strings.Contains(joined, "cask(s)") {
			t.Fatalf("expected cask label in comment:\n%s", joined)
		}
	})
}

func TestBrewPackagesRequiresUser(t *testing.T) {
	provider := brewPackagesProvider{}
	_, err := provider.Plan(manifest.ResolvedResource{
		Kind: "brew_packages",
		Name: "test",
		Spec: map[string]any{
			"packages": []any{"jq"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("error = %q, want 'user is required'", err)
	}
}

func TestBrewPackagesValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing packages",
			spec:    map[string]any{"user": "erik"},
			wantErr: "packages is required",
		},
		{
			name:    "packages not a list",
			spec:    map[string]any{"user": "erik", "packages": "jq"},
			wantErr: "must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := brewPackagesProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "brew_packages",
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

func TestBrewTapAddsMissing(t *testing.T) {
	withMockBrewTaps(map[string]bool{"homebrew/core": true}, func() {
		provider := brewTapProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "brew_tap",
			Name: "custom-tap",
			Spec: map[string]any{
				"user": "erik",
				"tap":  "hashicorp/tap",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_brew_tap") {
			t.Fatalf("expected brew tap:\n%s", joined)
		}
		if !strings.Contains(joined, "'hashicorp/tap'") {
			t.Fatalf("missing tap name:\n%s", joined)
		}
		if !strings.Contains(joined, "'erik'") {
			t.Fatalf("expected user in command:\n%s", joined)
		}
	})
}

func TestBrewTapConverged(t *testing.T) {
	withMockBrewTaps(map[string]bool{"hashicorp/tap": true}, func() {
		provider := brewTapProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "brew_tap",
			Name: "custom-tap",
			Spec: map[string]any{
				"user": "erik",
				"tap":  "hashicorp/tap",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no ops (already tapped), got: %v", ops)
		}
	})
}

func TestBrewTapValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing user",
			spec:    map[string]any{"tap": "x/y"},
			wantErr: "user is required",
		},
		{
			name:    "missing tap",
			spec:    map[string]any{"user": "erik"},
			wantErr: "tap is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := brewTapProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "brew_tap",
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

func TestBrewTapDependencyOrdering(t *testing.T) {
	withMockBrewTaps(map[string]bool{}, func() {
		withMockBrew(map[string]bool{}, map[string]bool{}, func() {
			planner := NewPlanner()
			resources := []manifest.ResolvedResource{
				{
					Kind: "brew_packages", Name: "hashicorp-tools",
					DependsOn:        []string{"hashicorp-tap"},
					DeclarationOrder: 0,
					Spec: map[string]any{
						"user":     "erik",
						"packages": []any{"terraform"},
					},
				},
				{
					Kind: "brew_tap", Name: "hashicorp-tap",
					DeclarationOrder: 1,
					Spec: map[string]any{
						"user": "erik",
						"tap":  "hashicorp/tap",
					},
				},
			}

			plan, err := planner.BuildPlan(resources)
			if err != nil {
				t.Fatalf("BuildPlan() error = %v", err)
			}

			tapIdx, pkgIdx := -1, -1
			for i, rp := range plan.Resources {
				if rp.Name == "hashicorp-tap" {
					tapIdx = i
				}
				if rp.Name == "hashicorp-tools" {
					pkgIdx = i
				}
			}
			if tapIdx < 0 || pkgIdx < 0 {
				t.Fatal("expected both resources in plan")
			}
			if tapIdx > pkgIdx {
				t.Fatalf("tap (idx %d) should come before packages (idx %d)", tapIdx, pkgIdx)
			}
		})
	})
}
