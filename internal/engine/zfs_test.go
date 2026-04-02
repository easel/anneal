package engine

import (
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func TestZfsDatasetProvider(t *testing.T) {
	tests := []struct {
		name        string
		exists      bool // whether dataset already exists
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:   "missing dataset triggers create",
			exists: false,
			spec: map[string]any{
				"name": "tank/data",
			},
			wantOps:     true,
			wantContent: "zfs create -p 'tank/data'",
		},
		{
			name:   "existing dataset is converged",
			exists: true,
			spec: map[string]any{
				"name": "tank/data",
			},
			wantOps: false,
		},
		{
			name:   "create with properties",
			exists: false,
			spec: map[string]any{
				"name": "tank/data",
				"properties": map[string]any{
					"compression": "lz4",
					"recordsize":  "128k",
				},
			},
			wantOps:     true,
			wantContent: "compression=lz4",
		},
		{
			name:   "create with encryption fields",
			exists: false,
			spec: map[string]any{
				"name":        "tank/secret",
				"encryption":  "aes-256-gcm",
				"keyformat":   "raw",
				"keylocation": "file:///root/key",
			},
			wantOps:     true,
			wantContent: "encryption=aes-256-gcm",
		},
		{
			name:   "create with keylength",
			exists: false,
			spec: map[string]any{
				"name":       "tank/secret",
				"encryption": "on",
				"keylength":  256,
			},
			wantOps:     true,
			wantContent: "keylength=256",
		},
		{
			name:   "keylength as float64",
			exists: false,
			spec: map[string]any{
				"name":       "tank/secret",
				"encryption": "on",
				"keylength":  float64(256),
			},
			wantOps:     true,
			wantContent: "keylength=256",
		},
		{
			name:   "create uses -p flag",
			exists: false,
			spec: map[string]any{
				"name": "tank/a/b/c",
			},
			wantOps:     true,
			wantContent: "zfs create -p",
		},
		{
			name:   "properties are sorted deterministically",
			exists: false,
			spec: map[string]any{
				"name": "tank/data",
				"properties": map[string]any{
					"compression": "lz4",
					"atime":       "off",
				},
			},
			wantOps:     true,
			wantContent: "-o 'atime=off' -o 'compression=lz4'",
		},
		{
			name:    "missing name returns error",
			exists:  false,
			spec:    map[string]any{},
			wantErr: "spec.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inject mock for dataset existence check.
			origFunc := zfsDatasetExistsFunc
			zfsDatasetExistsFunc = func(name string) (bool, error) {
				return tt.exists, nil
			}
			t.Cleanup(func() { zfsDatasetExistsFunc = origFunc })

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}

			provider := zfsDatasetProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "zfs_dataset",
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

func TestZfsPropertiesProvider(t *testing.T) {
	tests := []struct {
		name        string
		current     map[string]string // nil means dataset doesn't exist
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name: "changed property emits set",
			current: map[string]string{
				"compression": "off",
			},
			spec: map[string]any{
				"dataset": "tank/data",
				"properties": map[string]any{
					"compression": "lz4",
				},
			},
			wantOps:     true,
			wantContent: "zfs set 'compression=lz4' 'tank/data'",
		},
		{
			name: "matching property is converged",
			current: map[string]string{
				"compression": "lz4",
			},
			spec: map[string]any{
				"dataset": "tank/data",
				"properties": map[string]any{
					"compression": "lz4",
				},
			},
			wantOps: false,
		},
		{
			name:    "missing dataset warns and skips",
			current: nil, // dataset doesn't exist
			spec: map[string]any{
				"dataset": "tank/missing",
				"properties": map[string]any{
					"compression": "lz4",
				},
			},
			wantOps:     true,
			wantContent: "WARNING dataset tank/missing does not exist",
		},
		{
			name: "recursive flag uses -r",
			current: map[string]string{
				"compression": "off",
			},
			spec: map[string]any{
				"dataset": "tank",
				"properties": map[string]any{
					"compression": "lz4",
				},
				"recursive": true,
			},
			wantOps:     true,
			wantContent: "zfs set -r",
		},
		{
			name: "immutable property warns",
			current: map[string]string{
				"encryption": "off",
			},
			spec: map[string]any{
				"dataset": "tank/data",
				"properties": map[string]any{
					"encryption": "aes-256-gcm",
				},
			},
			wantOps:     true,
			wantContent: "WARNING encryption=aes-256-gcm cannot be changed",
		},
		{
			name: "multiple datasets",
			current: map[string]string{
				"atime": "on",
			},
			spec: map[string]any{
				"datasets": []any{"tank/a", "tank/b"},
				"properties": map[string]any{
					"atime": "off",
				},
			},
			wantOps:     true,
			wantContent: "atime=off",
		},
		{
			name: "plan shows current vs desired",
			current: map[string]string{
				"compression": "off",
			},
			spec: map[string]any{
				"dataset": "tank/data",
				"properties": map[string]any{
					"compression": "lz4",
				},
			},
			wantOps:     true,
			wantContent: "off → lz4",
		},
		{
			name:    "missing dataset and properties returns error",
			current: nil,
			spec: map[string]any{
				"properties": map[string]any{
					"compression": "lz4",
				},
			},
			wantErr: "spec.dataset or spec.datasets is required",
		},
		{
			name:    "missing properties returns error",
			current: nil,
			spec: map[string]any{
				"dataset": "tank/data",
			},
			wantErr: "spec.properties is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inject mock for zfs get.
			origFunc := zfsGetPropertiesFunc
			zfsGetPropertiesFunc = func(dataset string, properties []string) (map[string]string, error) {
				if tt.current == nil {
					return nil, nil
				}
				// Return only the requested properties from current.
				result := make(map[string]string)
				for _, p := range properties {
					if v, ok := tt.current[p]; ok {
						result[p] = v
					}
				}
				return result, nil
			}
			t.Cleanup(func() { zfsGetPropertiesFunc = origFunc })

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}

			provider := zfsPropertiesProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "zfs_properties",
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
