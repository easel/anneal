package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func TestHostsEntryProvider(t *testing.T) {
	tests := []struct {
		name        string
		existing    string // existing hosts file content ("" means file absent)
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:     "absent file creates entry",
			existing: "",
			spec: map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "myhost",
			},
			wantOps:     true,
			wantContent: "stdlib_hosts_entry",
		},
		{
			name:     "existing matching line is converged",
			existing: "192.168.1.1\tmyhost\n",
			spec: map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "myhost",
			},
			wantOps: false,
		},
		{
			name:     "different IP triggers update",
			existing: "10.0.0.1\tmyhost\n",
			spec: map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "myhost",
			},
			wantOps:     true,
			wantContent: "192.168.1.1",
		},
		{
			name:     "aliases included in desired line",
			existing: "",
			spec: map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "myhost",
				"aliases":  []any{"alias1", "alias2"},
			},
			wantOps:     true,
			wantContent: "192.168.1.1\tmyhost\talias1\talias2",
		},
		{
			name:     "matching line with aliases is converged",
			existing: "192.168.1.1\tmyhost\talias1\n",
			spec: map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "myhost",
				"aliases":  []any{"alias1"},
			},
			wantOps: false,
		},
		{
			name:     "missing ip returns error",
			existing: "",
			spec: map[string]any{
				"hostname": "myhost",
			},
			wantErr: "spec.ip is required",
		},
		{
			name:     "missing hostname returns error",
			existing: "",
			spec: map[string]any{
				"ip": "192.168.1.1",
			},
			wantErr: "spec.hostname is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			hostsPath := filepath.Join(dir, "hosts")

			if tt.existing != "" {
				if err := os.WriteFile(hostsPath, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}
			spec["_hosts_path"] = hostsPath

			provider := hostsEntryProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "hosts_entry",
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

func TestCrypttabEntryProvider(t *testing.T) {
	tests := []struct {
		name        string
		existing    string
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:     "absent file creates entry",
			existing: "",
			spec: map[string]any{
				"name":   "crypt0",
				"device": "/dev/sda2",
			},
			wantOps:     true,
			wantContent: "stdlib_crypttab_entry",
		},
		{
			name:     "matching line is converged",
			existing: "crypt0\t/dev/sda2\tnone\tluks\n",
			spec: map[string]any{
				"name":   "crypt0",
				"device": "/dev/sda2",
			},
			wantOps: false,
		},
		{
			name:     "different device triggers update",
			existing: "crypt0\t/dev/sda3\tnone\tluks\n",
			spec: map[string]any{
				"name":   "crypt0",
				"device": "/dev/sda2",
			},
			wantOps:     true,
			wantContent: "/dev/sda2",
		},
		{
			name:     "custom keyfile and options",
			existing: "",
			spec: map[string]any{
				"name":    "crypt0",
				"device":  "/dev/sda2",
				"keyfile": "/root/key",
				"options": "luks,discard",
			},
			wantOps:     true,
			wantContent: "crypt0\t/dev/sda2\t/root/key\tluks,discard",
		},
		{
			name:     "matching custom line is converged",
			existing: "crypt0\t/dev/sda2\t/root/key\tluks,discard\n",
			spec: map[string]any{
				"name":    "crypt0",
				"device":  "/dev/sda2",
				"keyfile": "/root/key",
				"options": "luks,discard",
			},
			wantOps: false,
		},
		{
			name:     "missing name returns error",
			existing: "",
			spec: map[string]any{
				"device": "/dev/sda2",
			},
			wantErr: "spec.name is required",
		},
		{
			name:     "missing device returns error",
			existing: "",
			spec: map[string]any{
				"name": "crypt0",
			},
			wantErr: "spec.device is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			crypttabPath := filepath.Join(dir, "crypttab")

			if tt.existing != "" {
				if err := os.WriteFile(crypttabPath, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}
			spec["_crypttab_path"] = crypttabPath

			provider := crypttabEntryProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "crypttab_entry",
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

func TestBinaryInstallProvider(t *testing.T) {
	tests := []struct {
		name        string
		existing    bool // whether the binary already exists
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name:     "absent binary triggers install",
			existing: false,
			spec: map[string]any{
				"url":  "https://example.com/bin",
				"path": "PLACEHOLDER",
			},
			wantOps:     true,
			wantContent: "stdlib_binary_install",
		},
		{
			name:     "existing binary without checksum is converged",
			existing: true,
			spec: map[string]any{
				"url":  "https://example.com/bin",
				"path": "PLACEHOLDER",
			},
			wantOps: false,
		},
		{
			name:     "existing binary with checksum triggers verify",
			existing: true,
			spec: map[string]any{
				"url":      "https://example.com/bin",
				"path":     "PLACEHOLDER",
				"checksum": "sha256:abc123",
			},
			wantOps:     true,
			wantContent: "sha256:abc123",
		},
		{
			name:     "custom mode in output",
			existing: false,
			spec: map[string]any{
				"url":  "https://example.com/bin",
				"path": "PLACEHOLDER",
				"mode": "0700",
			},
			wantOps:     true,
			wantContent: "'0700'",
		},
		{
			name:     "missing url returns error",
			existing: false,
			spec: map[string]any{
				"path": "PLACEHOLDER",
			},
			wantErr: "spec.url is required",
		},
		{
			name:     "missing path returns error",
			existing: false,
			spec: map[string]any{
				"url": "https://example.com/bin",
			},
			wantErr: "spec.path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := filepath.Join(dir, "mybin")

			if tt.existing {
				if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			spec := make(map[string]any)
			for k, v := range tt.spec {
				if v == "PLACEHOLDER" {
					spec[k] = binPath
				} else {
					spec[k] = v
				}
			}

			provider := binaryInstallProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "binary_install",
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

func TestCommandProvider(t *testing.T) {
	tests := []struct {
		name        string
		creates     bool // whether the creates path already exists
		spec        map[string]any
		wantOps     bool
		wantContent string
		wantErr     string
	}{
		{
			name: "command without creates always runs",
			spec: map[string]any{
				"command": "echo hello",
			},
			wantOps:     true,
			wantContent: "echo hello",
		},
		{
			name:    "creates path absent triggers command",
			creates: false,
			spec: map[string]any{
				"command": "touch /tmp/flag",
				"creates": "PLACEHOLDER",
			},
			wantOps:     true,
			wantContent: "stdlib_command_creates",
		},
		{
			name:    "creates path exists is converged",
			creates: true,
			spec: map[string]any{
				"command": "touch /tmp/flag",
				"creates": "PLACEHOLDER",
			},
			wantOps: false,
		},
		{
			name:    "missing command returns error",
			creates: false,
			spec:    map[string]any{},
			wantErr: "spec.command is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			createsPath := filepath.Join(dir, "flag")

			if tt.creates {
				if err := os.WriteFile(createsPath, []byte("exists"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			spec := make(map[string]any)
			for k, v := range tt.spec {
				if v == "PLACEHOLDER" {
					spec[k] = createsPath
				} else {
					spec[k] = v
				}
			}

			provider := commandProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "command",
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
