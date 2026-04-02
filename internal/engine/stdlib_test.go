package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStdlibFileWrite(t *testing.T) {
	shell := EmbeddedShell{}
	dir := t.TempDir()

	t.Run("creates file with content and mode", func(t *testing.T) {
		path := filepath.Join(dir, "write-basic", "hello.txt")
		script := `stdlib_file_write '` + path + `' '0644' 'root:root' <<'ANNEAL_EOF'
hello world
ANNEAL_EOF`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "hello world" {
			t.Errorf("content = %q, want %q", string(got), "hello world")
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o644 {
			t.Errorf("mode = %o, want 0644", perm)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		path := filepath.Join(dir, "write-nested", "a", "b", "c.txt")
		script := `stdlib_file_write '` + path + `' '0600' 'root:root' <<'ANNEAL_EOF'
nested content
ANNEAL_EOF`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "nested content" {
			t.Errorf("content = %q, want %q", string(got), "nested content")
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %o, want 0600", perm)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		path := filepath.Join(dir, "write-overwrite.txt")
		os.WriteFile(path, []byte("old"), 0o644)

		script := `stdlib_file_write '` + path + `' '0644' 'root:root' <<'ANNEAL_EOF'
new content
ANNEAL_EOF`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "new content" {
			t.Errorf("content = %q, want %q", string(got), "new content")
		}
	})
}

func TestStdlibFileCopy(t *testing.T) {
	shell := EmbeddedShell{}
	dir := t.TempDir()

	t.Run("copies file to dest with mode", func(t *testing.T) {
		src := filepath.Join(dir, "copy-src.txt")
		dest := filepath.Join(dir, "copy-sub", "copy-dest.txt")
		os.WriteFile(src, []byte("copy me"), 0o644)

		script := `stdlib_file_copy '` + src + `' '` + dest + `' '0600' 'root:root'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != "copy me" {
			t.Errorf("content = %q, want %q", string(got), "copy me")
		}

		info, err := os.Stat(dest)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %o, want 0600", perm)
		}
	})

	t.Run("does not create dest when source missing", func(t *testing.T) {
		dest := filepath.Join(dir, "copy-missing-dest.txt")
		script := `stdlib_file_copy '` + filepath.Join(dir, "nonexistent") + `' '` + dest + `' '0644' 'root:root'`
		// The cp command may or may not error in the embedded shell,
		// but the destination file should not be created.
		shell.Execute(script)
		if _, err := os.Stat(dest); err == nil {
			t.Error("dest should not exist when source is missing")
		}
	})
}

func TestStdlibDirCreate(t *testing.T) {
	shell := EmbeddedShell{}
	dir := t.TempDir()

	t.Run("creates directory with mode", func(t *testing.T) {
		path := filepath.Join(dir, "newdir")
		script := `stdlib_dir_create '` + path + `' '0755' 'root:root'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("mode = %o, want 0755", perm)
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		path := filepath.Join(dir, "a", "b", "c")
		script := `stdlib_dir_create '` + path + `' '0700' 'root:root'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("mode = %o, want 0700", perm)
		}
	})
}

func TestStdlibSymlink(t *testing.T) {
	shell := EmbeddedShell{}
	dir := t.TempDir()

	t.Run("creates symlink", func(t *testing.T) {
		target := filepath.Join(dir, "link-target.txt")
		link := filepath.Join(dir, "link.txt")
		os.WriteFile(target, []byte("target"), 0o644)

		script := `stdlib_symlink '` + target + `' '` + link + `'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if got != target {
			t.Errorf("link target = %q, want %q", got, target)
		}
	})

	t.Run("replaces existing symlink", func(t *testing.T) {
		oldTarget := filepath.Join(dir, "old-target.txt")
		newTarget := filepath.Join(dir, "new-target.txt")
		link := filepath.Join(dir, "replace-link.txt")
		os.WriteFile(oldTarget, []byte("old"), 0o644)
		os.WriteFile(newTarget, []byte("new"), 0o644)
		os.Symlink(oldTarget, link)

		script := `stdlib_symlink '` + newTarget + `' '` + link + `'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if got != newTarget {
			t.Errorf("link target = %q, want %q", got, newTarget)
		}
	})

	t.Run("creates parent directories for link", func(t *testing.T) {
		target := filepath.Join(dir, "sym-target.txt")
		link := filepath.Join(dir, "sym-nested", "deep", "link.txt")
		os.WriteFile(target, []byte("x"), 0o644)

		script := `stdlib_symlink '` + target + `' '` + link + `'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		got, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if got != target {
			t.Errorf("link target = %q, want %q", got, target)
		}
	})
}

func TestStdlibFileRemove(t *testing.T) {
	shell := EmbeddedShell{}
	dir := t.TempDir()

	t.Run("removes existing file", func(t *testing.T) {
		path := filepath.Join(dir, "to-remove.txt")
		os.WriteFile(path, []byte("remove me"), 0o644)

		script := `stdlib_file_remove '` + path + `'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("file should have been removed")
		}
	})

	t.Run("succeeds on nonexistent file", func(t *testing.T) {
		path := filepath.Join(dir, "already-gone.txt")
		script := `stdlib_file_remove '` + path + `'`
		_, err := shell.Execute(script)
		if err != nil {
			t.Fatalf("Execute should not fail on missing file: %v", err)
		}
	})
}
