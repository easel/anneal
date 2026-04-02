package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func TestKerberosKDCProvider(t *testing.T) {
	tests := []struct {
		name        string
		dbExists    bool
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:     "absent database triggers initialization",
			dbExists: false,
			spec: map[string]any{
				"realm":           "EXAMPLE.COM",
				"master_password": "s3cret",
			},
			wantOps:     true,
			wantContent: "kdb5_util create -r 'EXAMPLE.COM'",
		},
		{
			name:     "existing database is converged",
			dbExists: true,
			spec: map[string]any{
				"realm":           "EXAMPLE.COM",
				"master_password": "s3cret",
			},
			wantOps: false,
		},
		{
			name:     "plan shows secret marker for password",
			dbExists: false,
			spec: map[string]any{
				"realm":           "EXAMPLE.COM",
				"master_password": "s3cret",
			},
			wantOps:     true,
			wantContent: "(secret)",
		},
		{
			name:     "plan does not expose password in comment",
			dbExists: false,
			spec: map[string]any{
				"realm":           "EXAMPLE.COM",
				"master_password": "s3cret",
			},
			wantOps:     true,
			wantContent: "# master_password: (secret)",
		},
		{
			name:     "missing realm returns error",
			dbExists: false,
			spec: map[string]any{
				"master_password": "s3cret",
			},
			wantErr: "spec.realm is required",
		},
		{
			name:     "missing master_password returns error",
			dbExists: false,
			spec: map[string]any{
				"realm": "EXAMPLE.COM",
			},
			wantErr: "spec.master_password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origFunc := krbKDCExistsFunc
			krbKDCExistsFunc = func(dbPath string) (bool, error) {
				return tt.dbExists, nil
			}
			t.Cleanup(func() { krbKDCExistsFunc = origFunc })

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}

			provider := kerberosKDCProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "kerberos_kdc",
				Name: "test",
				Spec: spec,
			})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if tt.wantOps && len(ops) == 0 {
				t.Fatal("expected operations but got none")
			}
			if !tt.wantOps && len(ops) > 0 {
				t.Fatalf("expected no operations but got: %v", ops)
			}
			if tt.wantContent != "" {
				joined := strings.Join(ops, "\n")
				if !strings.Contains(joined, tt.wantContent) {
					t.Fatalf("ops missing %q:\n%s", tt.wantContent, joined)
				}
			}
		})
	}
}

func TestKerberosPrincipalProvider(t *testing.T) {
	tests := []struct {
		name        string
		existing    map[string]bool // existing principals
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:     "missing principal triggers creation",
			existing: map[string]bool{},
			spec: map[string]any{
				"principal": "nfs/server.example.com@EXAMPLE.COM",
			},
			wantOps:     true,
			wantContent: "addprinc -randkey nfs/server.example.com@EXAMPLE.COM",
		},
		{
			name: "existing principal is converged",
			existing: map[string]bool{
				"nfs/server.example.com@EXAMPLE.COM": true,
			},
			spec: map[string]any{
				"principal": "nfs/server.example.com@EXAMPLE.COM",
			},
			wantOps: false,
		},
		{
			name:     "randkey true by default",
			existing: map[string]bool{},
			spec: map[string]any{
				"principal": "host/server.example.com@EXAMPLE.COM",
			},
			wantOps:     true,
			wantContent: "-randkey",
		},
		{
			name:     "randkey false with password",
			existing: map[string]bool{},
			spec: map[string]any{
				"principal": "admin@EXAMPLE.COM",
				"randkey":   false,
				"password":  "adminpass",
			},
			wantOps:     true,
			wantContent: "addprinc -pw adminpass admin@EXAMPLE.COM",
		},
		{
			name:     "randkey false with password shows secret marker",
			existing: map[string]bool{},
			spec: map[string]any{
				"principal": "admin@EXAMPLE.COM",
				"randkey":   false,
				"password":  "adminpass",
			},
			wantOps:     true,
			wantContent: "(secret)",
		},
		{
			name:     "randkey false without password returns error",
			existing: map[string]bool{},
			spec: map[string]any{
				"principal": "admin@EXAMPLE.COM",
				"randkey":   false,
			},
			wantErr: "spec.password is required when randkey is false",
		},
		{
			name:     "missing principal returns error",
			existing: map[string]bool{},
			spec:     map[string]any{},
			wantErr:  "spec.principal is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origFunc := krbListPrincsFunc
			krbListPrincsFunc = func() (map[string]bool, error) {
				return tt.existing, nil
			}
			t.Cleanup(func() { krbListPrincsFunc = origFunc })

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}

			provider := kerberosPrincipalProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "kerberos_principal",
				Name: "test",
				Spec: spec,
			})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if tt.wantOps && len(ops) == 0 {
				t.Fatal("expected operations but got none")
			}
			if !tt.wantOps && len(ops) > 0 {
				t.Fatalf("expected no operations but got: %v", ops)
			}
			if tt.wantContent != "" {
				joined := strings.Join(ops, "\n")
				if !strings.Contains(joined, tt.wantContent) {
					t.Fatalf("ops missing %q:\n%s", tt.wantContent, joined)
				}
			}
		})
	}
}

func TestKerberosKeytabProvider(t *testing.T) {
	tests := []struct {
		name        string
		keytabExist bool // whether the keytab file already exists
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:        "absent keytab triggers creation",
			keytabExist: false,
			spec: map[string]any{
				"path":       "PLACEHOLDER",
				"principals": []any{"nfs/server.example.com@EXAMPLE.COM"},
			},
			wantOps:     true,
			wantContent: "# kerberos_keytab: create",
		},
		{
			name:        "existing keytab triggers regeneration",
			keytabExist: true,
			spec: map[string]any{
				"path":       "PLACEHOLDER",
				"principals": []any{"nfs/server.example.com@EXAMPLE.COM"},
			},
			wantOps:     true,
			wantContent: "# kerberos_keytab: regenerate",
		},
		{
			name:        "ktadd command for each principal",
			keytabExist: false,
			spec: map[string]any{
				"path": "PLACEHOLDER",
				"principals": []any{
					"nfs/server.example.com@EXAMPLE.COM",
					"host/server.example.com@EXAMPLE.COM",
				},
			},
			wantOps:     true,
			wantContent: "ktadd",
		},
		{
			name:        "principals are sorted",
			keytabExist: false,
			spec: map[string]any{
				"path": "PLACEHOLDER",
				"principals": []any{
					"nfs/server@EXAMPLE.COM",
					"host/server@EXAMPLE.COM",
				},
			},
			wantOps:     true,
			wantContent: "host/server@EXAMPLE.COM",
		},
		{
			name:        "enforces file mode",
			keytabExist: false,
			spec: map[string]any{
				"path":       "PLACEHOLDER",
				"principals": []any{"nfs/server@EXAMPLE.COM"},
			},
			wantOps:     true,
			wantContent: "chmod '0600'",
		},
		{
			name:        "custom mode",
			keytabExist: false,
			spec: map[string]any{
				"path":       "PLACEHOLDER",
				"principals": []any{"nfs/server@EXAMPLE.COM"},
				"mode":       "0400",
			},
			wantOps:     true,
			wantContent: "chmod '0400'",
		},
		{
			name:        "missing path returns error",
			keytabExist: false,
			spec: map[string]any{
				"principals": []any{"nfs/server@EXAMPLE.COM"},
			},
			wantErr: "spec.path is required",
		},
		{
			name:        "missing principals returns error",
			keytabExist: false,
			spec: map[string]any{
				"path": "PLACEHOLDER",
			},
			wantErr: "spec.principals is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			keytabPath := filepath.Join(dir, "krb5.keytab")

			if tt.keytabExist {
				if err := os.WriteFile(keytabPath, []byte("keytab"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			spec := make(map[string]any)
			for k, v := range tt.spec {
				if v == "PLACEHOLDER" {
					spec[k] = keytabPath
				} else {
					spec[k] = v
				}
			}
			spec["_skip_dir_check"] = true

			provider := kerberosKeytabProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "kerberos_keytab",
				Name: "test",
				Spec: spec,
			})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if tt.wantOps && len(ops) == 0 {
				t.Fatal("expected operations but got none")
			}
			if !tt.wantOps && len(ops) > 0 {
				t.Fatalf("expected no operations but got: %v", ops)
			}
			if tt.wantContent != "" {
				joined := strings.Join(ops, "\n")
				if !strings.Contains(joined, tt.wantContent) {
					t.Fatalf("ops missing %q:\n%s", tt.wantContent, joined)
				}
			}
		})
	}
}
