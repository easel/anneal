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

func TestDirectoryProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(dir string) string // returns path; creates state
		mode        string
		owner       string
		wantOps     bool
		wantContent string
	}{
		{
			name: "absent directory gets created",
			setup: func(dir string) string {
				return filepath.Join(dir, "newdir")
			},
			wantOps:     true,
			wantContent: "stdlib_dir_create",
		},
		{
			name: "existing directory with correct mode is converged",
			setup: func(dir string) string {
				p := filepath.Join(dir, "existing")
				os.Mkdir(p, 0o755)
				return p
			},
			mode:    "0755",
			wantOps: false,
		},
		{
			name: "existing directory with wrong mode triggers chmod",
			setup: func(dir string) string {
				p := filepath.Join(dir, "wrongmode")
				os.Mkdir(p, 0o755)
				return p
			},
			mode:        "0700",
			wantOps:     true,
			wantContent: "chmod",
		},
		{
			name: "nested absent directory gets created",
			setup: func(dir string) string {
				return filepath.Join(dir, "a", "b", "c")
			},
			wantOps:     true,
			wantContent: "stdlib_dir_create",
		},
		{
			name: "custom mode in create output",
			setup: func(dir string) string {
				return filepath.Join(dir, "custom")
			},
			mode:        "0700",
			wantOps:     true,
			wantContent: "'0700'",
		},
		{
			name: "custom owner in create output",
			setup: func(dir string) string {
				return filepath.Join(dir, "owndir")
			},
			owner:       "app:app",
			wantOps:     true,
			wantContent: "'app:app'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(dir)

			spec := map[string]any{"path": path}
			if tt.mode != "" {
				spec["mode"] = tt.mode
			}
			if tt.owner != "" {
				spec["owner"] = tt.owner
			}

			provider := directoryProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "directory",
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

func TestDirectoryProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		setup   func(dir string) string
		wantErr string
	}{
		{
			name:    "missing path",
			spec:    map[string]any{},
			wantErr: "path is required",
		},
		{
			name:    "empty path",
			spec:    map[string]any{"path": ""},
			wantErr: "path is required",
		},
		{
			name: "path exists but is a file",
			spec: map[string]any{"path": "PLACEHOLDER"},
			setup: func(dir string) string {
				p := filepath.Join(dir, "afile")
				os.WriteFile(p, []byte("x"), 0o644)
				return p
			},
			wantErr: "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := make(map[string]any)
			for k, v := range tt.spec {
				spec[k] = v
			}
			if tt.setup != nil {
				p := tt.setup(dir)
				if spec["path"] == "PLACEHOLDER" {
					spec["path"] = p
				}
			}

			provider := directoryProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "directory",
				Name: "test",
				Spec: spec,
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

func TestSymlinkProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(dir string) (path, target string)
		wantOps     bool
		wantContent string
	}{
		{
			name: "absent symlink gets created",
			setup: func(dir string) (string, string) {
				return filepath.Join(dir, "link"), "/some/target"
			},
			wantOps:     true,
			wantContent: "stdlib_symlink",
		},
		{
			name: "correct symlink is converged",
			setup: func(dir string) (string, string) {
				target := filepath.Join(dir, "target")
				os.WriteFile(target, []byte("x"), 0o644)
				link := filepath.Join(dir, "link")
				os.Symlink(target, link)
				return link, target
			},
			wantOps: false,
		},
		{
			name: "wrong target triggers update",
			setup: func(dir string) (string, string) {
				link := filepath.Join(dir, "link")
				os.Symlink("/old/target", link)
				return link, "/new/target"
			},
			wantOps:     true,
			wantContent: "stdlib_symlink",
		},
		{
			name: "broken symlink to correct target is converged",
			setup: func(dir string) (string, string) {
				target := "/nonexistent/target"
				link := filepath.Join(dir, "link")
				os.Symlink(target, link)
				return link, target
			},
			wantOps: false,
		},
		{
			name: "regular file at path triggers update",
			setup: func(dir string) (string, string) {
				link := filepath.Join(dir, "link")
				os.WriteFile(link, []byte("not a link"), 0o644)
				return link, "/some/target"
			},
			wantOps:     true,
			wantContent: "stdlib_symlink",
		},
		{
			name: "target path appears in output",
			setup: func(dir string) (string, string) {
				return filepath.Join(dir, "link"), "/specific/target/path"
			},
			wantOps:     true,
			wantContent: "/specific/target/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path, target := tt.setup(dir)

			provider := symlinkProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "symlink",
				Name: "test",
				Spec: map[string]any{
					"path":   path,
					"target": target,
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

func TestSymlinkProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing path",
			spec:    map[string]any{"target": "/foo"},
			wantErr: "path is required",
		},
		{
			name:    "missing target",
			spec:    map[string]any{"path": "/foo"},
			wantErr: "target is required",
		},
		{
			name:    "both missing",
			spec:    map[string]any{},
			wantErr: "path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := symlinkProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "symlink",
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

func TestFileAbsentProviderStateMatrix(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(dir string) map[string]any // returns spec
		wantOps     bool
		wantContent string
	}{
		{
			name: "present file produces remove op",
			setup: func(dir string) map[string]any {
				p := filepath.Join(dir, "target")
				os.WriteFile(p, []byte("x"), 0o644)
				return map[string]any{"path": p}
			},
			wantOps:     true,
			wantContent: "stdlib_file_remove",
		},
		{
			name: "absent file produces no ops",
			setup: func(dir string) map[string]any {
				return map[string]any{"path": filepath.Join(dir, "nonexistent")}
			},
			wantOps: false,
		},
		{
			name: "glob matches present files",
			setup: func(dir string) map[string]any {
				os.WriteFile(filepath.Join(dir, "a.tmp"), []byte("x"), 0o644)
				os.WriteFile(filepath.Join(dir, "b.tmp"), []byte("y"), 0o644)
				return map[string]any{"pattern": filepath.Join(dir, "*.tmp")}
			},
			wantOps:     true,
			wantContent: "stdlib_file_remove",
		},
		{
			name: "glob matches zero files produces no ops",
			setup: func(dir string) map[string]any {
				return map[string]any{"pattern": filepath.Join(dir, "*.nonexistent")}
			},
			wantOps: false,
		},
		{
			name: "symlink is removed",
			setup: func(dir string) map[string]any {
				link := filepath.Join(dir, "link")
				os.Symlink("/whatever", link)
				return map[string]any{"path": link}
			},
			wantOps:     true,
			wantContent: "stdlib_file_remove",
		},
		{
			name: "multiple glob matches produce multiple ops",
			setup: func(dir string) map[string]any {
				os.WriteFile(filepath.Join(dir, "x.log"), []byte("a"), 0o644)
				os.WriteFile(filepath.Join(dir, "y.log"), []byte("b"), 0o644)
				os.WriteFile(filepath.Join(dir, "z.log"), []byte("c"), 0o644)
				return map[string]any{"pattern": filepath.Join(dir, "*.log")}
			},
			wantOps:     true,
			wantContent: "stdlib_file_remove",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := tt.setup(dir)

			provider := fileAbsentProvider{}
			ops, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file_absent",
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

// TestFileProviderModeDrift verifies that matching content with wrong mode
// emits a chmod operation without rewriting the file.
func TestFileProviderModeDrift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	os.WriteFile(path, []byte("hello"), 0o644)

	provider := fileProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "file",
		Name: "test",
		Spec: map[string]any{
			"path":    path,
			"content": "hello",
			"mode":    "0600",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected chmod operation for mode drift, got none")
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod in ops, got:\n%s", joined)
	}
	if !strings.Contains(joined, "'0600'") {
		t.Fatalf("expected target mode 0600 in ops, got:\n%s", joined)
	}
	// Should NOT rewrite content
	if strings.Contains(joined, "stdlib_file_write") {
		t.Fatalf("mode-only drift should not trigger file rewrite, got:\n%s", joined)
	}
	// Should show mode change comment
	if !strings.Contains(joined, "# mode:") {
		t.Fatalf("expected mode change comment, got:\n%s", joined)
	}
}

// TestFileProviderContentDriftComment verifies plan diff comments for content changes.
func TestFileProviderContentDriftComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")

	// Test new file comment
	provider := fileProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "file",
		Name: "test",
		Spec: map[string]any{
			"path":    path,
			"content": "new content",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "# new file") {
		t.Fatalf("expected '# new file' comment for absent file, got:\n%s", joined)
	}

	// Test content changed comment
	os.WriteFile(path, []byte("old content"), 0o644)
	ops, err = provider.Plan(manifest.ResolvedResource{
		Kind: "file",
		Name: "test",
		Spec: map[string]any{
			"path":    path,
			"content": "new content",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	joined = strings.Join(ops, "\n")
	if !strings.Contains(joined, "# content changed") {
		t.Fatalf("expected '# content changed' comment for drift, got:\n%s", joined)
	}
}

// TestDirectoryProviderModeDriftComment verifies directory mode drift includes comment.
func TestDirectoryProviderModeDriftComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testdir")
	os.Mkdir(path, 0o755)

	provider := directoryProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "directory",
		Name: "test",
		Spec: map[string]any{
			"path": path,
			"mode": "0700",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "# mode:") {
		t.Fatalf("expected mode change comment, got:\n%s", joined)
	}
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod in ops, got:\n%s", joined)
	}
}

// TestDirectoryProviderNewDirComment verifies new directory has comment.
func TestDirectoryProviderNewDirComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newdir")

	provider := directoryProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "directory",
		Name: "test",
		Spec: map[string]any{
			"path": path,
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "# new directory") {
		t.Fatalf("expected '# new directory' comment, got:\n%s", joined)
	}
}

// TestStaticFileProviderModeDrift verifies static_file mode drift emits chmod.
func TestStaticFileProviderModeDrift(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source")
	destPath := filepath.Join(dir, "target")
	os.WriteFile(srcPath, []byte("static"), 0o644)
	os.WriteFile(destPath, []byte("static"), 0o644)

	provider := staticFileProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "static_file",
		Name: "test",
		Spec: map[string]any{
			"source": srcPath,
			"path":   destPath,
			"mode":   "0600",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected chmod operation for mode drift, got none")
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod in ops, got:\n%s", joined)
	}
	if strings.Contains(joined, "stdlib_file_write") {
		t.Fatalf("mode-only drift should not trigger file rewrite, got:\n%s", joined)
	}
}

// TestFileCopyProviderModeDrift verifies file_copy mode drift emits chmod.
func TestFileCopyProviderModeDrift(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source")
	destPath := filepath.Join(dir, "target")
	os.WriteFile(srcPath, []byte("data"), 0o644)
	os.WriteFile(destPath, []byte("data"), 0o644)

	provider := fileCopyProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "file_copy",
		Name: "test",
		Spec: map[string]any{
			"source": srcPath,
			"path":   destPath,
			"mode":   "0600",
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected chmod operation for mode drift, got none")
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod in ops, got:\n%s", joined)
	}
	if strings.Contains(joined, "stdlib_file_copy") {
		t.Fatalf("mode-only drift should not trigger file copy, got:\n%s", joined)
	}
}

// TestTemplateFileProviderModeDrift verifies template_file mode drift emits chmod.
func TestTemplateFileProviderModeDrift(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "template.tmpl")
	destPath := filepath.Join(dir, "target")
	os.WriteFile(srcPath, []byte("Hello {{ .name }}!"), 0o644)
	os.WriteFile(destPath, []byte("Hello world!"), 0o644)

	provider := templateFileProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "template_file",
		Name: "test",
		Spec: map[string]any{
			"source": srcPath,
			"path":   destPath,
			"mode":   "0600",
		},
		Vars: map[string]any{"name": "world"},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected chmod operation for mode drift, got none")
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod in ops, got:\n%s", joined)
	}
	if strings.Contains(joined, "stdlib_file_write") {
		t.Fatalf("mode-only drift should not trigger file rewrite, got:\n%s", joined)
	}
}

// TestMetadataOpsHelper verifies the metadataOps helper directly.
func TestMetadataOpsHelper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	os.WriteFile(path, []byte("x"), 0o644)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Mode matches — no ops
	ops := metadataOps(path, info, "0644", "root:root")
	if len(ops) != 0 {
		t.Fatalf("expected no ops for matching mode (non-root), got: %v", ops)
	}

	// Mode differs — chmod emitted
	ops = metadataOps(path, info, "0700", "root:root")
	if len(ops) == 0 {
		t.Fatal("expected chmod op for mode drift")
	}
	joined := strings.Join(ops, "\n")
	if !strings.Contains(joined, "chmod") {
		t.Fatalf("expected chmod, got:\n%s", joined)
	}
	if !strings.Contains(joined, "# mode: 0644") {
		t.Fatalf("expected mode comment, got:\n%s", joined)
	}
}

func TestFileAbsentProviderMultipleGlobOps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.tmp"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.tmp"), []byte("y"), 0o644)

	provider := fileAbsentProvider{}
	ops, err := provider.Plan(manifest.ResolvedResource{
		Kind: "file_absent",
		Name: "test",
		Spec: map[string]any{"pattern": filepath.Join(dir, "*.tmp")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d: %v", len(ops), ops)
	}
}

func TestFileAbsentProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{
			name:    "missing path and pattern",
			spec:    map[string]any{},
			wantErr: "requires spec.path or spec.pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fileAbsentProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "file_absent",
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
