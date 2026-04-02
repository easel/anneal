package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func TestFileProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		existing    *fileState // nil = file does not exist
		desired     fileDesired
		wantOps     bool   // true if provider should emit operations
		wantContent string // expected substring in emitted ops
	}{
		{
			name:     "absent file gets created",
			existing: nil,
			desired: fileDesired{
				content: "new content",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "stdlib_file_write",
		},
		{
			name:     "matching file is converged (no ops)",
			existing: &fileState{content: "hello"},
			desired: fileDesired{
				content: "hello",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps: false,
		},
		{
			name:     "content drift triggers write",
			existing: &fileState{content: "old content"},
			desired: fileDesired{
				content: "new content",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "new content",
		},
		{
			name:     "empty content is valid",
			existing: nil,
			desired: fileDesired{
				content: "",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "stdlib_file_write",
		},
		{
			name:     "custom mode in output",
			existing: nil,
			desired: fileDesired{
				content: "secure",
				mode:    "0600",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "'0600'",
		},
		{
			name:     "custom owner in output",
			existing: nil,
			desired: fileDesired{
				content: "data",
				mode:    "0644",
				owner:   "app:app",
			},
			wantOps:     true,
			wantContent: "'app:app'",
		},
		{
			name:     "multiline content preserved",
			existing: nil,
			desired: fileDesired{
				content: "line1\nline2\nline3\n",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "line1\nline2\nline3\n",
		},
		{
			name:     "content with shell metacharacters",
			existing: nil,
			desired: fileDesired{
				content: "$(whoami) `date` $HOME && rm -rf /",
				mode:    "0644",
				owner:   "root:root",
			},
			wantOps:     true,
			wantContent: "$(whoami)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "target")

			if tt.existing != nil {
				os.WriteFile(path, []byte(tt.existing.content), 0o644)
			}

			spec := map[string]any{
				"path":    path,
				"content": tt.desired.content,
			}
			if tt.desired.mode != "0644" {
				spec["mode"] = tt.desired.mode
			}
			if tt.desired.owner != "root:root" {
				spec["owner"] = tt.desired.owner
			}

			provider := fileProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file",
				Name: "test",
				Spec: spec,
			})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if tt.wantOps && len(ops) == 0 {
				t.Fatal("expected operations but got none (file should not be converged)")
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

type fileState struct {
	content string
}

type fileDesired struct {
	content string
	mode    string
	owner   string
}

func TestFileProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing path",
			spec:    map[string]any{"content": "hello"},
			wantErr: "path is required",
		},
		{
			name:    "empty path",
			spec:    map[string]any{"path": "", "content": "hello"},
			wantErr: "path is required",
		},
		{
			name:    "missing content",
			spec:    map[string]any{"path": "/tmp/f"},
			wantErr: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fileProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file",
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
