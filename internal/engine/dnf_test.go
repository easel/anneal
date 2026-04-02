package engine

import (
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func withMockRpm(installed map[string]bool, fn func()) {
	orig := rpmQueryFunc
	rpmQueryFunc = func(names []string) (map[string]bool, error) {
		result := map[string]bool{}
		for _, n := range names {
			if installed[n] {
				result[n] = true
			}
		}
		return result, nil
	}
	defer func() { rpmQueryFunc = orig }()
	fn()
}

func TestDnfPackagesInstallsMissing(t *testing.T) {
	withMockRpm(map[string]bool{"curl": true}, func() {
		provider := dnfPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "dnf_packages",
			Name: "base",
			Spec: map[string]any{
				"packages": []any{"curl", "wget", "git"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_dnf_install") {
			t.Fatalf("expected dnf install:\n%s", joined)
		}
		if !strings.Contains(joined, "'wget'") || !strings.Contains(joined, "'git'") {
			t.Fatalf("missing packages in install:\n%s", joined)
		}
		if strings.Contains(joined, "'curl'") {
			t.Fatalf("curl should not be installed (already present):\n%s", joined)
		}
		// Should be batched into one call
		if strings.Count(joined, "stdlib_dnf_install") != 1 {
			t.Fatalf("expected single batched call:\n%s", joined)
		}
	})
}

func TestDnfPackagesConverged(t *testing.T) {
	withMockRpm(map[string]bool{"curl": true, "wget": true}, func() {
		provider := dnfPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "dnf_packages",
			Name: "base",
			Spec: map[string]any{
				"packages": []any{"curl", "wget"},
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

func TestDnfPackagesValidation(t *testing.T) {
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
		{
			name:    "empty package name",
			spec:    map[string]any{"packages": []any{""}},
			wantErr: "non-empty strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockRpm(map[string]bool{}, func() {
				provider := dnfPackagesProvider{}
				_, err := provider.Plan(manifest.ResolvedResource{
					Kind: "dnf_packages",
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

func TestDnfPackagesBatchCount(t *testing.T) {
	withMockRpm(map[string]bool{}, func() {
		provider := dnfPackagesProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "dnf_packages",
			Name: "many",
			Spec: map[string]any{
				"packages": []any{"a", "b", "c"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "# install 3 package(s)") {
			t.Fatalf("expected batch count comment:\n%s", joined)
		}
	})
}
