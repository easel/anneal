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

func TestTemplateFileProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		tmplContent string
		vars        map[string]any
		existing    *fileState // nil = file does not exist
		wantOps     bool
		wantContent string
	}{
		{
			name:        "absent file gets created from template",
			tmplContent: "Hello {{ .name }}!",
			vars:        map[string]any{"name": "world"},
			existing:    nil,
			wantOps:     true,
			wantContent: "Hello world!",
		},
		{
			name:        "matching rendered content is converged",
			tmplContent: "Hello {{ .name }}!",
			vars:        map[string]any{"name": "world"},
			existing:    &fileState{content: "Hello world!"},
			wantOps:     false,
		},
		{
			name:        "content drift triggers write",
			tmplContent: "Hello {{ .name }}!",
			vars:        map[string]any{"name": "world"},
			existing:    &fileState{content: "Hello old!"},
			wantOps:     true,
			wantContent: "Hello world!",
		},
		{
			name:        "plain text without templates",
			tmplContent: "no templates here",
			vars:        map[string]any{},
			existing:    nil,
			wantOps:     true,
			wantContent: "no templates here",
		},
		{
			name:        "sprig functions work",
			tmplContent: "{{ .name | upper }}",
			vars:        map[string]any{"name": "hello"},
			existing:    nil,
			wantOps:     true,
			wantContent: "HELLO",
		},
		{
			name:        "multiline template",
			tmplContent: "server {{ .Hostname }}\nport {{ .port }}\n",
			vars:        map[string]any{"Hostname": "web01", "port": "8080"},
			existing:    nil,
			wantOps:     true,
			wantContent: "server web01\nport 8080\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "template.tmpl")
			destPath := filepath.Join(dir, "target")

			os.WriteFile(srcPath, []byte(tt.tmplContent), 0o644)
			if tt.existing != nil {
				os.WriteFile(destPath, []byte(tt.existing.content), 0o644)
			}

			provider := templateFileProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "template_file",
				Name: "test",
				Spec: map[string]any{
					"source": srcPath,
					"path":   destPath,
				},
				Vars: tt.vars,
			})
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

func TestTemplateFileProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		setup   func(dir string) // optional setup for source file
		wantErr string
	}{
		{
			name:    "missing source",
			spec:    map[string]any{"path": "/tmp/f"},
			wantErr: "source is required",
		},
		{
			name:    "missing path",
			spec:    map[string]any{"source": "/tmp/s"},
			wantErr: "path is required",
		},
		{
			name: "source file not found",
			spec: map[string]any{"source": "/nonexistent/file.tmpl", "path": "/tmp/f"},
			wantErr: "reading source",
		},
		{
			name: "invalid template syntax",
			spec: map[string]any{"source": "PLACEHOLDER", "path": "/tmp/f"},
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "bad.tmpl"), []byte("{{ .unclosed"), 0o644)
			},
			wantErr: "rendering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(dir)
			}
			spec := make(map[string]any)
			for k, v := range tt.spec {
				if v == "PLACEHOLDER" {
					spec[k] = filepath.Join(dir, "bad.tmpl")
				} else {
					spec[k] = v
				}
			}

			provider := templateFileProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "template_file",
				Name: "test",
				Spec: spec,
				Vars: map[string]any{},
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

func TestStaticFileProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		srcContent  string
		existing    *fileState
		wantOps     bool
		wantContent string
	}{
		{
			name:        "absent file gets created",
			srcContent:  "static content",
			existing:    nil,
			wantOps:     true,
			wantContent: "static content",
		},
		{
			name:       "matching content is converged",
			srcContent: "static content",
			existing:   &fileState{content: "static content"},
			wantOps:    false,
		},
		{
			name:        "content drift triggers write",
			srcContent:  "new content",
			existing:    &fileState{content: "old content"},
			wantOps:     true,
			wantContent: "new content",
		},
		{
			name:        "shell variables preserved verbatim",
			srcContent:  "export PATH=${DESTDIR}/bin:$PATH",
			existing:    nil,
			wantOps:     true,
			wantContent: "${DESTDIR}",
		},
		{
			name:        "template syntax preserved verbatim",
			srcContent:  "{{ .Hostname }} is literal text",
			existing:    nil,
			wantOps:     true,
			wantContent: "{{ .Hostname }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "source")
			destPath := filepath.Join(dir, "target")

			os.WriteFile(srcPath, []byte(tt.srcContent), 0o644)
			if tt.existing != nil {
				os.WriteFile(destPath, []byte(tt.existing.content), 0o644)
			}

			provider := staticFileProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "static_file",
				Name: "test",
				Spec: map[string]any{
					"source": srcPath,
					"path":   destPath,
				},
			})
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

func TestStaticFileProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing source",
			spec:    map[string]any{"path": "/tmp/f"},
			wantErr: "source is required",
		},
		{
			name:    "missing path",
			spec:    map[string]any{"source": "/tmp/s"},
			wantErr: "path is required",
		},
		{
			name:    "source file not found",
			spec:    map[string]any{"source": "/nonexistent/file", "path": "/tmp/f"},
			wantErr: "reading source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := staticFileProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "static_file",
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

func TestFileCopyProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		srcContent  string
		existing    *fileState
		mode        string
		wantOps     bool
		wantContent string
	}{
		{
			name:        "absent destination gets created",
			srcContent:  "source data",
			existing:    nil,
			wantOps:     true,
			wantContent: "stdlib_file_copy",
		},
		{
			name:       "matching content is converged",
			srcContent: "source data",
			existing:   &fileState{content: "source data"},
			wantOps:    false,
		},
		{
			name:        "content drift triggers copy",
			srcContent:  "new data",
			existing:    &fileState{content: "old data"},
			wantOps:     true,
			wantContent: "stdlib_file_copy",
		},
		{
			name:        "custom mode in output",
			srcContent:  "secure",
			existing:    nil,
			mode:        "0600",
			wantOps:     true,
			wantContent: "'0600'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "source")
			destPath := filepath.Join(dir, "target")

			os.WriteFile(srcPath, []byte(tt.srcContent), 0o644)
			if tt.existing != nil {
				os.WriteFile(destPath, []byte(tt.existing.content), 0o644)
			}

			spec := map[string]any{
				"source": srcPath,
				"path":   destPath,
			}
			if tt.mode != "" {
				spec["mode"] = tt.mode
			}

			provider := fileCopyProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file_copy",
				Name: "test",
				Spec: spec,
			})
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

func TestFileCopyProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing source",
			spec:    map[string]any{"path": "/tmp/f"},
			wantErr: "source is required",
		},
		{
			name:    "missing path",
			spec:    map[string]any{"source": "/tmp/s"},
			wantErr: "path is required",
		},
		{
			name:    "source file not found",
			spec:    map[string]any{"source": "/nonexistent/file", "path": "/tmp/f"},
			wantErr: "reading source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fileCopyProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file_copy",
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

// TestFileCopyEmitsSourcePath verifies that file_copy operations reference
// the source path (for runtime copy) rather than embedding content.
func TestFileCopyEmitsSourcePath(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source")
	destPath := filepath.Join(dir, "target")
	os.WriteFile(srcPath, []byte("data"), 0o644)

	provider := fileCopyProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "file_copy",
		Name: "test",
		Spec: map[string]any{"source": srcPath, "path": destPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, srcPath) {
		t.Fatalf("expected source path %q in ops:\n%s", srcPath, joined)
	}
	if !strings.Contains(joined, destPath) {
		t.Fatalf("expected dest path %q in ops:\n%s", destPath, joined)
	}
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
