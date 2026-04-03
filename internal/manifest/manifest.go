package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

// Include declares an included manifest with optional variable overrides.
type Include struct {
	Path string         `yaml:"path"`
	Vars map[string]any `yaml:"vars"`
}

type Manifest struct {
	Vars      map[string]any `yaml:"vars"`
	Includes  []Include      `yaml:"includes"`
	Resources []Resource     `yaml:"resources"`
}

type Resource struct {
	Kind       string         `yaml:"kind" json:"kind"`
	Name       string         `yaml:"name" json:"name"`
	DependsOn  []string       `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Each       []any          `yaml:"each,omitempty" json:"each,omitempty"`
	Notify     []string       `yaml:"notify,omitempty" json:"notify,omitempty"`
	Trigger    bool           `yaml:"trigger,omitempty" json:"trigger,omitempty"`
	Spec       map[string]any `yaml:"spec" json:"spec"`
	SourceFile string         `yaml:"-" json:"source_file,omitempty"`
}

type ResolvedManifest struct {
	Vars      map[string]any
	Resources []ResolvedResource
}

type ResolvedResource struct {
	Kind             string
	Name             string
	DependsOn        []string
	Notify           []string
	Trigger          bool
	Spec             map[string]any
	Vars             map[string]any // Template context for providers that render external templates
	DeclarationOrder int
}

type ResolveOptions struct {
	Env          map[string]string
	Builtins     Builtins
	HostVarsFile string // Optional path to host-specific variable overrides
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

// Load loads a manifest and recursively resolves includes with cycle detection.
// Diamond includes (same file reached from multiple parents) are deduplicated:
// the first occurrence's resources and vars are used; subsequent occurrences are
// skipped. This matches SD-001's "include graph resolution" semantics.
func Load(path string) (*Manifest, error) {
	seen := map[string]bool{}
	m, err := loadWithIncludes(path, nil, seen)
	if err != nil {
		return nil, err
	}
	if err := m.ValidateMerged(); err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", path, err)
	}
	return m, nil
}

// loadSingle loads and validates a single manifest file without processing includes.
func loadSingle(path string) (*Manifest, error) {
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

	// Tag each resource with its source file for provenance tracking.
	for i := range manifest.Resources {
		manifest.Resources[i].SourceFile = path
	}

	return &manifest, nil
}

// loadWithIncludes recursively loads a manifest and its includes, detecting cycles.
// visited tracks the current include chain (per-branch) for cycle detection.
// seen tracks all resolved absolute paths (shared across branches) for diamond deduplication.
func loadWithIncludes(path string, visited []string, seen map[string]bool) (*Manifest, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}

	// Cycle detection: check the current branch for a back-edge.
	for _, v := range visited {
		if v == absPath {
			chain := append(append([]string{}, visited...), absPath)
			return nil, fmt.Errorf("circular include detected: %s", strings.Join(chain, " → "))
		}
	}

	// Diamond deduplication: if this path was already fully resolved via
	// another branch, return an empty manifest so its resources and vars
	// are not duplicated. The first branch to reach a file wins.
	if seen[absPath] {
		return &Manifest{}, nil
	}
	seen[absPath] = true

	raw, err := loadSingle(path)
	if err != nil {
		return nil, err
	}

	if len(raw.Includes) == 0 {
		return raw, nil
	}

	dir := filepath.Dir(absPath)
	newVisited := append(append([]string{}, visited...), absPath)

	mergedVars := map[string]any{}
	var mergedResources []Resource

	for _, inc := range raw.Includes {
		if inc.Path == "" {
			return nil, fmt.Errorf("load manifest %s: include path is required", path)
		}
		incPath := inc.Path
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, incPath)
		}

		child, err := loadWithIncludes(incPath, newVisited, seen)
		if err != nil {
			return nil, fmt.Errorf("include %s: %w", inc.Path, err)
		}

		// Module defaults: add child vars without overriding existing
		for k, v := range child.Vars {
			if _, exists := mergedVars[k]; !exists {
				mergedVars[k] = v
			}
		}

		// Include-level var overrides from the parent
		for k, v := range inc.Vars {
			mergedVars[k] = v
		}

		mergedResources = append(mergedResources, child.Resources...)
	}

	// Root vars override everything from modules
	for k, v := range raw.Vars {
		mergedVars[k] = v
	}

	// Include resources before root resources
	mergedResources = append(mergedResources, raw.Resources...)

	return &Manifest{
		Vars:      mergedVars,
		Resources: mergedResources,
	}, nil
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

// ValidateMerged checks constraints that only apply after include merging,
// such as duplicate resource names across included manifests.
func (m *Manifest) ValidateMerged() error {
	type resourceRef struct {
		index      int
		sourceFile string
	}
	seen := map[string]resourceRef{}
	for idx, resource := range m.Resources {
		if prev, exists := seen[resource.Name]; exists {
			prevFile := prev.sourceFile
			curFile := resource.SourceFile
			if prevFile != "" && curFile != "" {
				return fmt.Errorf("duplicate resource name %q (resource %d from %s and %d from %s)", resource.Name, prev.index, prevFile, idx, curFile)
			}
			return fmt.Errorf("duplicate resource name %q (resource %d and %d)", resource.Name, prev.index, idx)
		}
		seen[resource.Name] = resourceRef{index: idx, sourceFile: resource.SourceFile}
	}
	return nil
}

func (m *Manifest) Resolve(opts ResolveOptions) (*ResolvedManifest, error) {
	builtins := opts.Builtins.withDefaults()

	// Variable precedence: module defaults → root vars → host file → env vars.
	// By this point m.Vars already has module defaults merged under root vars
	// (handled by loadWithIncludes), so we start with those.
	resolvedVars := map[string]any{}
	for key, value := range m.Vars {
		resolvedVars[key] = value
	}

	// Host file overrides (between manifest vars and env vars)
	if opts.HostVarsFile != "" {
		hostVars, err := loadHostVars(opts.HostVarsFile)
		if err != nil {
			return nil, fmt.Errorf("host vars: %w", err)
		}
		for k, v := range hostVars {
			resolvedVars[k] = v
		}
	}

	// Environment overrides (highest precedence for variables)
	for key := range resolvedVars {
		if value, ok := lookupEnvOverride(opts.Env, key); ok {
			resolvedVars[key] = value
		}
	}

	ctx := makeTemplateContext(resolvedVars, builtins)

	// Pass 1: Expand iterators.
	// Resources with `each` are expanded into N concrete resources, one per item.
	// Each item value is rendered through the base context so template expressions
	// in the each list resolve before pass 2.
	type expandedRes struct {
		resource Resource
		iterVars map[string]any // Item and Index for this expansion (nil if not from each)
	}
	var expanded []expandedRes
	for _, resource := range m.Resources {
		if resource.Each == nil {
			// No each field — pass through unchanged
			expanded = append(expanded, expandedRes{resource: resource})
			continue
		}
		// Expand: each item becomes a separate resource
		for idx, rawItem := range resource.Each {
			item, err := resolveValue(rawItem, ctx)
			if err != nil {
				return nil, fmt.Errorf("resource %s each[%d]: %w", resource.Name, idx, err)
			}
			expanded = append(expanded, expandedRes{
				resource: resource,
				iterVars: map[string]any{
					"Item":  item,
					"Index": idx,
				},
			})
		}
		// Empty each list: zero resources produced (no error)
	}

	// Pass 2: Render all template expressions with full context.
	resolved := &ResolvedManifest{
		Vars:      mapsClone(resolvedVars),
		Resources: make([]ResolvedResource, 0, len(expanded)),
	}
	for idx, er := range expanded {
		resourceCtx := ctx
		if er.iterVars != nil {
			resourceCtx = mapsClone(ctx)
			for k, v := range er.iterVars {
				resourceCtx[k] = v
			}
		}

		name, err := RenderString(er.resource.Name, resourceCtx)
		if err != nil {
			return nil, fmt.Errorf("resource %d name: %w", idx, err)
		}
		dependsOn := make([]string, 0, len(er.resource.DependsOn))
		for depIdx, dep := range er.resource.DependsOn {
			rendered, err := RenderString(dep, resourceCtx)
			if err != nil {
				return nil, fmt.Errorf("resource %d depends_on[%d]: %w", idx, depIdx, err)
			}
			dependsOn = append(dependsOn, rendered)
		}
		spec, err := resolveValue(er.resource.Spec, resourceCtx)
		if err != nil {
			return nil, fmt.Errorf("resource %d (%s) spec: %w", idx, name, err)
		}
		resolvedSpec, ok := spec.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("resource %d (%s): spec must resolve to an object", idx, name)
		}
		resolved.Resources = append(resolved.Resources, ResolvedResource{
			Kind:             er.resource.Kind,
			Name:             name,
			DependsOn:        dependsOn,
			Notify:           er.resource.Notify,
			Trigger:          er.resource.Trigger,
			Spec:             resolvedSpec,
			Vars:             mapsClone(resourceCtx),
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

// loadHostVars loads a YAML file containing host-specific variable overrides.
func loadHostVars(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load host vars %s: %w", path, err)
	}
	var vars map[string]any
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return nil, fmt.Errorf("load host vars %s: %w", path, err)
	}
	return vars, nil
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
