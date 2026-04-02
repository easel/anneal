package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Vars      map[string]any `yaml:"vars"`
	Resources []Resource     `yaml:"resources"`
}

type Resource struct {
	Kind      string         `yaml:"kind"`
	Name      string         `yaml:"name"`
	DependsOn []string       `yaml:"depends_on"`
	Spec      map[string]any `yaml:"spec"`
}

type ResolvedManifest struct {
	Vars      map[string]any
	Resources []ResolvedResource
}

type ResolvedResource struct {
	Kind             string
	Name             string
	DependsOn        []string
	Spec             map[string]any
	Vars             map[string]any // Template context for providers that render external templates
	DeclarationOrder int
}

type ResolveOptions struct {
	Env      map[string]string
	Builtins Builtins
}

type Builtins struct {
	Hostname   string
	FQDN       string
	Arch       string
	DebArch    string
	KernelArch string
	OSVersion  string
}

var nonEnvCharPattern = regexp.MustCompile(`[^A-Za-z0-9]+`)

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}
	if err := ensureSingleDocument(decoder, path); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}

	return &manifest, nil
}

func LoadResolved(path string, opts ResolveOptions) (*ResolvedManifest, error) {
	raw, err := Load(path)
	if err != nil {
		return nil, err
	}
	resolved, err := raw.Resolve(opts)
	if err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}
	return resolved, nil
}

func ensureSingleDocument(decoder *yaml.Decoder, path string) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("load manifest %s: %w", path, err)
	}
	return fmt.Errorf("load manifest %s: multiple YAML documents are not supported", path)
}

func (m *Manifest) Validate() error {
	for idx, resource := range m.Resources {
		if resource.Kind == "" {
			return fmt.Errorf("resource %d: kind is required", idx)
		}
		if resource.Name == "" {
			return fmt.Errorf("resource %d: name is required", idx)
		}
		if resource.Spec == nil {
			return fmt.Errorf("resource %d (%s): spec is required", idx, resource.Name)
		}
	}

	return nil
}

func (m *Manifest) Resolve(opts ResolveOptions) (*ResolvedManifest, error) {
	builtins := opts.Builtins.withDefaults()
	resolvedVars := map[string]any{}
	for key, value := range m.Vars {
		resolvedVars[key] = value
	}
	for key := range resolvedVars {
		if value, ok := lookupEnvOverride(opts.Env, key); ok {
			resolvedVars[key] = value
		}
	}

	ctx := makeTemplateContext(resolvedVars, builtins)
	resolved := &ResolvedManifest{
		Vars:      mapsClone(resolvedVars),
		Resources: make([]ResolvedResource, 0, len(m.Resources)),
	}
	for idx, resource := range m.Resources {
		name, err := RenderString(resource.Name, ctx)
		if err != nil {
			return nil, fmt.Errorf("resource %d name: %w", idx, err)
		}
		dependsOn := make([]string, 0, len(resource.DependsOn))
		for depIdx, dep := range resource.DependsOn {
			rendered, err := RenderString(dep, ctx)
			if err != nil {
				return nil, fmt.Errorf("resource %d depends_on[%d]: %w", idx, depIdx, err)
			}
			dependsOn = append(dependsOn, rendered)
		}
		spec, err := resolveValue(resource.Spec, ctx)
		if err != nil {
			return nil, fmt.Errorf("resource %d (%s) spec: %w", idx, name, err)
		}
		resolvedSpec, ok := spec.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("resource %d (%s): spec must resolve to an object", idx, name)
		}
		resolved.Resources = append(resolved.Resources, ResolvedResource{
			Kind:             resource.Kind,
			Name:             name,
			DependsOn:        dependsOn,
			Spec:             resolvedSpec,
			Vars:             mapsClone(ctx),
			DeclarationOrder: idx,
		})
	}
	return resolved, nil
}

func CurrentBuiltins() Builtins {
	hostname, _ := os.Hostname()
	return Builtins{
		Hostname:   hostname,
		FQDN:       hostname,
		Arch:       runtime.GOARCH,
		DebArch:    mapDebArch(runtime.GOARCH),
		KernelArch: mapKernelArch(runtime.GOARCH),
		OSVersion:  readOSVersion(),
	}
}

func (b Builtins) withDefaults() Builtins {
	current := CurrentBuiltins()
	if b.Hostname == "" {
		b.Hostname = current.Hostname
	}
	if b.FQDN == "" {
		if b.Hostname != current.Hostname {
			// Caller provided a custom hostname; fall back to it for FQDN.
			b.FQDN = b.Hostname
		} else {
			b.FQDN = current.FQDN
		}
	}
	if b.Arch == "" {
		b.Arch = current.Arch
	}
	if b.DebArch == "" {
		b.DebArch = mapDebArch(b.Arch)
	}
	if b.KernelArch == "" {
		b.KernelArch = mapKernelArch(b.Arch)
	}
	if b.OSVersion == "" {
		b.OSVersion = current.OSVersion
	}
	return b
}

func lookupEnvOverride(env map[string]string, key string) (string, bool) {
	if env == nil {
		return "", false
	}
	normalized := nonEnvCharPattern.ReplaceAllString(strings.ToUpper(key), "_")
	// Prefer ANNEAL_-prefixed forms to avoid collisions with system env vars.
	prefixed := []string{
		"ANNEAL_" + normalized,
		"ANNEAL_" + strings.ToUpper(key),
		"ANNEAL_" + key,
	}
	for _, candidate := range slices.Compact(prefixed) {
		if value, ok := env[candidate]; ok {
			return value, true
		}
	}
	return "", false
}

func makeTemplateContext(vars map[string]any, builtins Builtins) map[string]any {
	ctx := mapsClone(vars)
	ctx["Hostname"] = builtins.Hostname
	ctx["FQDN"] = builtins.FQDN
	ctx["Arch"] = builtins.Arch
	ctx["DebArch"] = builtins.DebArch
	ctx["KernelArch"] = builtins.KernelArch
	ctx["OSVersion"] = builtins.OSVersion
	return ctx
}

func resolveValue(value any, ctx map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return RenderString(typed, ctx)
	case []any:
		resolved := make([]any, 0, len(typed))
		for _, item := range typed {
			next, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, next)
		}
		return resolved, nil
	case map[string]any:
		resolved := make(map[string]any, len(typed))
		for key, item := range typed {
			resolvedKey, err := RenderString(key, ctx)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			next, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			resolved[resolvedKey] = next
		}
		return resolved, nil
	default:
		return value, nil
	}
}

// RenderString renders a Go template string with the given context and Sprig functions.
func RenderString(value string, ctx map[string]any) (string, error) {
	if !strings.Contains(value, "{{") {
		return value, nil
	}
	tmpl, err := template.New("value").Funcs(sprig.TxtFuncMap()).Option("missingkey=error").Parse(value)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func readOSVersion() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION_ID=") {
			return strings.Trim(line[len("VERSION_ID="):], `"`)
		}
	}
	return ""
}

func mapDebArch(arch string) string {
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return arch
	}
}

func mapKernelArch(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

func mapsClone[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
