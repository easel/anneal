package engine

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/easel/anneal/internal/manifest"
)

// ResourcePlan holds the planned operations for a single resource.
type ResourcePlan struct {
	Name    string
	Kind    string
	Script  string // Shell fragment for this resource (empty = already converged)
	Trigger bool   // True if this is a trigger resource
}

// Plan holds the full execution plan with per-resource breakdown.
type Plan struct {
	Resources []ResourcePlan
}

// Script returns the full plan as an executable shell script.
// Returns empty string if no resources need changes.
func (p *Plan) Script() string {
	var ops []string
	for _, rp := range p.Resources {
		if rp.Script != "" {
			ops = append(ops, rp.Script)
		}
	}
	if len(ops) == 0 {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n")
	buf.WriteString("set -e\n\n")
	for idx, op := range ops {
		if idx > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(op)
		if !strings.HasSuffix(op, "\n") {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// ResourceStatus describes what happened to a resource during apply.
type ResourceStatus int

const (
	StatusApplied ResourceStatus = iota
	StatusFailed
	StatusSkipped
	StatusConverged // Already in desired state, no action taken
)

func (s ResourceStatus) String() string {
	switch s {
	case StatusApplied:
		return "applied"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	case StatusConverged:
		return "converged"
	default:
		return "unknown"
	}
}

// ResourceResult records the outcome of applying one resource.
type ResourceResult struct {
	Name   string
	Kind   string
	Status ResourceStatus
	Error  error
}

// ApplyResult holds the outcome of a full apply run.
type ApplyResult struct {
	Results []ResourceResult
}

// Summary returns a human-readable fail-stop summary.
func (ar *ApplyResult) Summary() string {
	var buf bytes.Buffer
	for _, r := range ar.Results {
		switch r.Status {
		case StatusApplied:
			fmt.Fprintf(&buf, "  applied: %s (%s)\n", r.Name, r.Kind)
		case StatusConverged:
			fmt.Fprintf(&buf, "  converged: %s (%s)\n", r.Name, r.Kind)
		case StatusFailed:
			fmt.Fprintf(&buf, "  FAILED: %s (%s): %v\n", r.Name, r.Kind, r.Error)
		case StatusSkipped:
			fmt.Fprintf(&buf, "  skipped: %s (%s)\n", r.Name, r.Kind)
		}
	}
	return buf.String()
}

// Failed returns true if any resource failed.
func (ar *ApplyResult) Failed() bool {
	for _, r := range ar.Results {
		if r.Status == StatusFailed {
			return true
		}
	}
	return false
}

type Planner struct {
	providers map[string]Provider
}

type Provider interface {
	Plan(resource manifest.ResolvedResource) ([]string, error)
}

func NewPlanner() *Planner {
	return &Planner{
		providers: map[string]Provider{
			"file":          fileProvider{},
			"template_file": templateFileProvider{},
			"static_file":   staticFileProvider{},
			"file_copy":     fileCopyProvider{},
			"directory":     directoryProvider{},
			"symlink":       symlinkProvider{},
			"file_absent":   fileAbsentProvider{},
			"apt_packages":  aptPackagesProvider{},
			"apt_purge":     aptPurgeProvider{},
		},
	}
}

func (p *Planner) Validate(resources []manifest.ResolvedResource) error {
	// Separate normal and trigger resources.
	var normal []manifest.ResolvedResource
	triggerSet := map[string]bool{}
	for _, r := range resources {
		if r.Trigger {
			triggerSet[r.Name] = true
		} else {
			normal = append(normal, r)
		}
	}

	// Validate dependency graph for normal resources only (triggers are ordered after).
	if _, err := topoSort(normal); err != nil {
		return err
	}

	for _, r := range resources {
		if _, ok := p.providers[r.Kind]; !ok {
			return fmt.Errorf("resource %s: unknown kind %q", r.Name, r.Kind)
		}
		// Validate that notify targets reference trigger resources.
		for _, target := range r.Notify {
			if !triggerSet[target] {
				return fmt.Errorf("resource %s: notify target %q is not a trigger resource", r.Name, target)
			}
		}
	}
	return nil
}

// BuildPlan produces a structured plan with per-resource operations.
// Trigger resources are ordered after all normal resources and only
// receive operations when at least one notifying resource has changes.
func (p *Planner) BuildPlan(resources []manifest.ResolvedResource) (*Plan, error) {
	// Separate normal and trigger resources.
	var normal, triggers []manifest.ResolvedResource
	for _, r := range resources {
		if r.Trigger {
			triggers = append(triggers, r)
		} else {
			normal = append(normal, r)
		}
	}

	ordered, err := topoSort(normal)
	if err != nil {
		return nil, err
	}

	// Plan normal resources and track which triggers are notified.
	plan := &Plan{}
	notifiedTriggers := map[string]bool{}

	for _, resource := range ordered {
		provider, ok := p.providers[resource.Kind]
		if !ok {
			return nil, fmt.Errorf("resource %s: unknown kind %q", resource.Name, resource.Kind)
		}
		ops, err := provider.Plan(resource)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", resource.Name, err)
		}
		var script string
		if len(ops) > 0 {
			script = strings.Join(ops, "\n")
			// Resource has changes — mark its notify targets as pending.
			for _, target := range resource.Notify {
				notifiedTriggers[target] = true
			}
		}
		plan.Resources = append(plan.Resources, ResourcePlan{
			Name:   resource.Name,
			Kind:   resource.Kind,
			Script: script,
		})
	}

	// Plan trigger resources — only if notified by a changed resource.
	sortReady(triggers)
	for _, resource := range triggers {
		if !notifiedTriggers[resource.Name] {
			// Not notified — converged (no operations).
			plan.Resources = append(plan.Resources, ResourcePlan{
				Name:    resource.Name,
				Kind:    resource.Kind,
				Trigger: true,
			})
			continue
		}
		provider, ok := p.providers[resource.Kind]
		if !ok {
			return nil, fmt.Errorf("trigger %s: unknown kind %q", resource.Name, resource.Kind)
		}
		ops, err := provider.Plan(resource)
		if err != nil {
			return nil, fmt.Errorf("trigger %s: %w", resource.Name, err)
		}
		var script string
		if len(ops) > 0 {
			script = "# triggered\n" + strings.Join(ops, "\n")
		}
		plan.Resources = append(plan.Resources, ResourcePlan{
			Name:    resource.Name,
			Kind:    resource.Kind,
			Script:  script,
			Trigger: true,
		})
	}

	return plan, nil
}

// Build returns the plan as a monolithic shell script (backward compatible).
func (p *Planner) Build(resources []manifest.ResolvedResource) (string, error) {
	plan, err := p.BuildPlan(resources)
	if err != nil {
		return "", err
	}
	return plan.Script(), nil
}

// Apply executes a plan resource-by-resource with fail-stop semantics.
// If savedScript is non-empty, it re-plans and compares the script output
// against the saved version to detect drift before executing.
func (p *Planner) Apply(sys System, resources []manifest.ResolvedResource, savedScript string) (*ApplyResult, error) {
	currentPlan, err := p.BuildPlan(resources)
	if err != nil {
		return nil, fmt.Errorf("re-plan failed: %w", err)
	}

	if savedScript != "" {
		currentScript := currentPlan.Script()
		if currentScript != savedScript {
			return nil, fmt.Errorf("plan drift detected: system state changed since plan was saved")
		}
	}

	result := &ApplyResult{}
	failed := false
	for _, rp := range currentPlan.Resources {
		if failed {
			result.Results = append(result.Results, ResourceResult{
				Name:   rp.Name,
				Kind:   rp.Kind,
				Status: StatusSkipped,
			})
			continue
		}
		if rp.Script == "" {
			result.Results = append(result.Results, ResourceResult{
				Name:   rp.Name,
				Kind:   rp.Kind,
				Status: StatusConverged,
			})
			continue
		}
		_, execErr := sys.Execute(rp.Script)
		if execErr != nil {
			failed = true
			result.Results = append(result.Results, ResourceResult{
				Name:   rp.Name,
				Kind:   rp.Kind,
				Status: StatusFailed,
				Error:  execErr,
			})
		} else {
			result.Results = append(result.Results, ResourceResult{
				Name:   rp.Name,
				Kind:   rp.Kind,
				Status: StatusApplied,
			})
		}
	}
	return result, nil
}

type fileProvider struct{}

func (fileProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("file spec.path is required")
	}
	content, ok := resource.Spec["content"].(string)
	if !ok {
		return nil, fmt.Errorf("file spec.content is required")
	}

	mode := "0644"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}

	current, err := os.ReadFile(path)
	if err == nil && string(current) == content {
		// Content matches — check metadata drift only.
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		return metadataOps(path, info, mode, owner), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var ops []string
	if err != nil {
		ops = append(ops, "# new file")
	} else {
		ops = append(ops, "# content changed")
	}
	delim := uniqueHeredocDelimiter(content)
	ops = append(ops, fmt.Sprintf("stdlib_file_write %s %s %s <<'%s'\n%s\n%s", shellQuote(path), shellQuote(mode), shellQuote(owner), delim, content, delim))
	return ops, nil
}

// templateFileProvider renders a Go template source file with manifest variables
// and writes the result to the destination path.
type templateFileProvider struct{}

func (templateFileProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	source, ok := resource.Spec["source"].(string)
	if !ok || source == "" {
		return nil, fmt.Errorf("template_file spec.source is required")
	}
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("template_file spec.path is required")
	}

	tmplContent, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("template_file: reading source %s: %w", source, err)
	}

	rendered, err := manifest.RenderString(string(tmplContent), resource.Vars)
	if err != nil {
		return nil, fmt.Errorf("template_file: rendering %s: %w", source, err)
	}

	mode := "0644"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}

	current, err := os.ReadFile(path)
	if err == nil && string(current) == rendered {
		// Content matches — check metadata drift only.
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		return metadataOps(path, info, mode, owner), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var ops []string
	if err != nil {
		ops = append(ops, "# new file")
	} else {
		ops = append(ops, "# content changed")
	}
	delim := uniqueHeredocDelimiter(rendered)
	ops = append(ops, fmt.Sprintf("stdlib_file_write %s %s %s <<'%s'\n%s\n%s", shellQuote(path), shellQuote(mode), shellQuote(owner), delim, rendered, delim))
	return ops, nil
}

// staticFileProvider copies a source file verbatim — no template processing.
type staticFileProvider struct{}

func (staticFileProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	source, ok := resource.Spec["source"].(string)
	if !ok || source == "" {
		return nil, fmt.Errorf("static_file spec.source is required")
	}
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("static_file spec.path is required")
	}

	srcContent, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("static_file: reading source %s: %w", source, err)
	}

	mode := "0644"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}

	current, err := os.ReadFile(path)
	if err == nil && string(current) == string(srcContent) {
		// Content matches — check metadata drift only.
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		return metadataOps(path, info, mode, owner), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var ops []string
	if err != nil {
		ops = append(ops, "# new file")
	} else {
		ops = append(ops, "# content changed")
	}
	content := string(srcContent)
	delim := uniqueHeredocDelimiter(content)
	ops = append(ops, fmt.Sprintf("stdlib_file_write %s %s %s <<'%s'\n%s\n%s", shellQuote(path), shellQuote(mode), shellQuote(owner), delim, content, delim))
	return ops, nil
}

// fileCopyProvider copies a file from source to destination using stdlib_file_copy.
type fileCopyProvider struct{}

func (fileCopyProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	source, ok := resource.Spec["source"].(string)
	if !ok || source == "" {
		return nil, fmt.Errorf("file_copy spec.source is required")
	}
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("file_copy spec.path is required")
	}

	srcContent, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("file_copy: reading source %s: %w", source, err)
	}

	mode := "0644"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}

	current, err := os.ReadFile(path)
	if err == nil && string(current) == string(srcContent) {
		// Content matches — check metadata drift only.
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		return metadataOps(path, info, mode, owner), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var ops []string
	if err != nil {
		ops = append(ops, "# new file")
	} else {
		ops = append(ops, "# content changed")
	}
	ops = append(ops, fmt.Sprintf("stdlib_file_copy %s %s %s %s", shellQuote(source), shellQuote(path), shellQuote(mode), shellQuote(owner)))
	return ops, nil
}

// directoryProvider ensures a directory exists with the correct mode and owner.
type directoryProvider struct{}

func (directoryProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("directory spec.path is required")
	}

	mode := "0755"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}

	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("directory: path %s exists but is not a directory", path)
		}
		// Directory exists — check metadata drift (mode and owner).
		return metadataOps(path, info, mode, owner), nil
	}

	// Directory does not exist — create it
	return []string{
		"# new directory",
		fmt.Sprintf("stdlib_dir_create %s %s %s", shellQuote(path), shellQuote(mode), shellQuote(owner)),
	}, nil
}

// symlinkProvider ensures a symlink exists pointing to the correct target.
type symlinkProvider struct{}

func (symlinkProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("symlink spec.path is required")
	}
	target, ok := resource.Spec["target"].(string)
	if !ok || target == "" {
		return nil, fmt.Errorf("symlink spec.target is required")
	}

	// Check current link state
	currentTarget, err := os.Readlink(path)
	if err == nil && currentTarget == target {
		// Already points to the right target
		return nil, nil
	}

	// Missing, not a symlink, or wrong target — create/update
	return []string{
		fmt.Sprintf("stdlib_symlink %s %s", shellQuote(target), shellQuote(path)),
	}, nil
}

// fileAbsentProvider ensures files are removed.
type fileAbsentProvider struct{}

func (fileAbsentProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, _ := resource.Spec["path"].(string)
	pattern, _ := resource.Spec["pattern"].(string)

	if path == "" && pattern == "" {
		return nil, fmt.Errorf("file_absent requires spec.path or spec.pattern")
	}

	var paths []string
	if path != "" {
		paths = append(paths, path)
	}
	if pattern != "" {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("file_absent: invalid glob pattern %q: %w", pattern, err)
		}
		paths = append(paths, matches...)
	}

	var ops []string
	for _, p := range paths {
		_, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue // Already absent
		}
		if err != nil {
			return nil, err
		}
		ops = append(ops, fmt.Sprintf("stdlib_file_remove %s", shellQuote(p)))
	}
	return ops, nil
}

// metadataOps checks mode and owner of an existing file or directory and returns
// correction operations with descriptive comments when they differ from desired.
func metadataOps(path string, info os.FileInfo, desiredMode, desiredOwner string) []string {
	var ops []string

	currentMode := fmt.Sprintf("0%o", info.Mode().Perm())
	if currentMode != desiredMode {
		ops = append(ops, fmt.Sprintf("# mode: %s → %s", currentMode, desiredMode))
		ops = append(ops, fmt.Sprintf("chmod %s %s", shellQuote(desiredMode), shellQuote(path)))
	}

	// Check ownership only when running as root (only root can reliably chown).
	if os.Getuid() == 0 {
		currentOwner := fileOwner(info)
		if currentOwner != "" && currentOwner != desiredOwner {
			ops = append(ops, fmt.Sprintf("# owner: %s → %s", currentOwner, desiredOwner))
			ops = append(ops, fmt.Sprintf("chown %s %s", shellQuote(desiredOwner), shellQuote(path)))
		}
	}

	return ops
}

// fileOwner returns the "user:group" string for a file's current owner.
// Returns empty string if the owner cannot be determined.
func fileOwner(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	u, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
	if err != nil {
		return ""
	}
	g, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid))
	if err != nil {
		return ""
	}
	return u.Username + ":" + g.Name
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// uniqueHeredocDelimiter returns a delimiter guaranteed not to appear in content.
func uniqueHeredocDelimiter(content string) string {
	delim := "ANNEAL_EOF"
	for strings.Contains(content, delim) {
		delim += "_"
	}
	return delim
}

func topoSort(resources []manifest.ResolvedResource) ([]manifest.ResolvedResource, error) {
	byName := make(map[string]manifest.ResolvedResource, len(resources))
	inDegree := make(map[string]int, len(resources))
	children := make(map[string][]string, len(resources))

	for _, resource := range resources {
		if _, exists := byName[resource.Name]; exists {
			return nil, fmt.Errorf("duplicate resource name %q", resource.Name)
		}
		byName[resource.Name] = resource
		inDegree[resource.Name] = len(resource.DependsOn)
	}
	for _, resource := range resources {
		for _, dep := range resource.DependsOn {
			if _, ok := byName[dep]; !ok {
				return nil, fmt.Errorf("resource %s: unknown dependency %q", resource.Name, dep)
			}
			children[dep] = append(children[dep], resource.Name)
		}
	}

	ready := make([]manifest.ResolvedResource, 0, len(resources))
	for _, resource := range resources {
		if inDegree[resource.Name] == 0 {
			ready = append(ready, resource)
		}
	}
	sortReady(ready)

	ordered := make([]manifest.ResolvedResource, 0, len(resources))
	for len(ready) > 0 {
		resource := ready[0]
		ready = ready[1:]
		ordered = append(ordered, resource)

		for _, childName := range children[resource.Name] {
			inDegree[childName]--
			if inDegree[childName] == 0 {
				ready = append(ready, byName[childName])
			}
		}
		sortReady(ready)
	}

	if len(ordered) != len(resources) {
		return nil, fmt.Errorf("dependency cycle detected: %s", detectCycle(byName))
	}
	return ordered, nil
}

func detectCycle(byName map[string]manifest.ResolvedResource) string {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var stack []string
	var cycle []string

	var walk func(string) bool
	walk = func(name string) bool {
		visiting[name] = true
		visited[name] = true
		stack = append(stack, name)
		for _, dep := range byName[name].DependsOn {
			if !visited[dep] {
				if walk(dep) {
					return true
				}
				continue
			}
			if visiting[dep] {
				start := 0
				for idx, current := range stack {
					if current == dep {
						start = idx
						break
					}
				}
				cycle = append(append([]string{}, stack[start:]...), dep)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		visiting[name] = false
		return false
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if visited[name] {
			continue
		}
		if walk(name) {
			return strings.Join(cycle, " -> ")
		}
	}
	return "unknown"
}

func sortReady(resources []manifest.ResolvedResource) {
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].DeclarationOrder < resources[j].DeclarationOrder
	})
}
