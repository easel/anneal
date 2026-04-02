package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoadManifest fuzzes the manifest YAML parser, variable resolution,
// template evaluation, and validation pipeline. The goal is to find panics,
// infinite loops, and unhandled error paths in the combined YAML+template
// processing stack.
func FuzzLoadManifest(f *testing.F) {
	// Seed corpus: valid manifests
	f.Add([]byte(`resources:
  - kind: file
    name: motd
    spec:
      path: /etc/motd
      content: hello
`))
	f.Add([]byte(`vars:
  env: prod
  port: "8080"
resources:
  - kind: file
    name: config
    spec:
      path: /etc/app.conf
      content: "env={{.env}} port={{.port}}"
`))
	f.Add([]byte(`resources:
  - kind: directory
    name: data
    spec:
      path: /var/data
      mode: "0755"
      owner: "root:root"
`))
	f.Add([]byte(`resources:
  - kind: apt_packages
    name: deps
    spec:
      packages:
        - curl
        - jq
`))
	f.Add([]byte(`resources:
  - kind: file
    name: a
    spec:
      path: /a
      content: a
  - kind: file
    name: b
    depends_on:
      - a
    spec:
      path: /b
      content: b
`))

	// Seed corpus: edge cases and known-bad inputs
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`resources: null`))
	f.Add([]byte(`resources: []`))
	f.Add([]byte(`resources:
  - kind: ""
    name: x
    spec:
      path: /x
`))
	f.Add([]byte(`resources:
  - kind: file
    name: ""
    spec:
      path: /x
`))
	f.Add([]byte(`resources:
  - kind: file
    name: x
`))
	// Deeply nested YAML
	f.Add([]byte(`vars:
  a:
    b:
      c:
        d: value
resources:
  - kind: file
    name: deep
    spec:
      path: /deep
      content: "{{.a}}"
`))
	// Template syntax errors
	f.Add([]byte(`resources:
  - kind: file
    name: bad-tmpl
    spec:
      path: /x
      content: "{{.unclosed"
`))
	// Unknown YAML fields
	f.Add([]byte(`resources:
  - kind: file
    name: x
    spec:
      path: /x
      content: ok
    unknown_field: true
`))
	// Very long string values
	f.Add([]byte(`resources:
  - kind: file
    name: big
    spec:
      path: /big
      content: "` + string(make([]byte, 1000)) + `"
`))
	// Iterator (each) expansion
	f.Add([]byte(`resources:
  - kind: file
    name: "item-{{.Item}}"
    each:
      - one
      - two
    spec:
      path: "/tmp/{{.Item}}"
      content: "{{.Item}}"
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "anneal.yaml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}

		// Load and parse — we don't care about errors, only panics.
		m, err := Load(path)
		if err != nil {
			return
		}

		// If loading succeeded, try resolving — exercises template evaluation.
		_, _ = m.Resolve(ResolveOptions{
			Env:      map[string]string{"HOME": "/root"},
			Builtins: CurrentBuiltins(),
		})
	})
}

// FuzzRenderString fuzzes the Go template + Sprig function evaluation pipeline.
// This is the core template rendering path used by manifest variable resolution
// and template_file providers.
func FuzzRenderString(f *testing.F) {
	// Seed corpus: valid templates
	f.Add("hello world", "key", "value")
	f.Add("{{.key}}", "key", "value")
	f.Add("prefix-{{.key}}-suffix", "key", "value")
	f.Add("{{.key | upper}}", "key", "hello")
	f.Add("{{.key | lower}}", "key", "HELLO")
	f.Add("{{.key | default \"fallback\"}}", "key", "")
	f.Add("{{if .key}}yes{{else}}no{{end}}", "key", "true")

	// Seed corpus: edge cases
	f.Add("", "key", "value")
	f.Add("no templates here", "key", "value")
	f.Add("{{", "key", "value")
	f.Add("}}", "key", "value")
	f.Add("{{.missing}}", "key", "value")
	f.Add("{{.key | repeat 3}}", "key", "x")
	f.Add("{{.key | sha256sum}}", "key", "data")
	f.Add("{{.key | b64enc}}", "key", "data")
	f.Add("{{.key | b64dec}}", "key", "aGVsbG8=")
	f.Add("{{.key | trim}}", "key", "  spaces  ")
	f.Add("{{.key | replace \"a\" \"b\"}}", "key", "aaa")

	// Seed corpus: potentially dangerous patterns
	f.Add("{{range $i, $v := .key}}{{$v}}{{end}}", "key", "list")
	f.Add("{{.key | quote}}", "key", "it's \"quoted\"")
	f.Add("{{.key | indent 4}}", "key", "line1\nline2")

	f.Fuzz(func(t *testing.T, tmpl, key, value string) {
		ctx := map[string]any{
			key: value,
		}
		// We don't care about errors, only panics.
		_, _ = RenderString(tmpl, ctx)
	})
}
