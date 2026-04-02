package engine

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// ResourcePlan holds the planned operations for a single resource.
type ResourcePlan struct {
	Name   string
	Kind   string
	Script string // Shell fragment for this resource (empty = already converged)
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
			"file": fileProvider{},
		},
	}
}

func (p *Planner) Validate(resources []manifest.ResolvedResource) error {
	if _, err := topoSort(resources); err != nil {
		return err
	}
	for _, resource := range resources {
		if _, ok := p.providers[resource.Kind]; !ok {
			return fmt.Errorf("resource %s: unknown kind %q", resource.Name, resource.Kind)
		}
	}
	return nil
}

// BuildPlan produces a structured plan with per-resource operations.
func (p *Planner) BuildPlan(resources []manifest.ResolvedResource) (*Plan, error) {
	ordered, err := topoSort(resources)
	if err != nil {
		return nil, err
	}

	plan := &Plan{}
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
		}
		plan.Resources = append(plan.Resources, ResourcePlan{
			Name:   resource.Name,
			Kind:   resource.Kind,
			Script: script,
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
	current, err := os.ReadFile(path)
	if err == nil && string(current) == content {
		return nil, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	mode := "0644"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}
	owner := "root:root"
	if rawOwner, ok := resource.Spec["owner"].(string); ok && rawOwner != "" {
		owner = rawOwner
	}
	delim := uniqueHeredocDelimiter(content)
	return []string{
		fmt.Sprintf("stdlib_file_write %s %s %s <<'%s'\n%s\n%s", shellQuote(path), shellQuote(mode), shellQuote(owner), delim, content, delim),
	}, nil
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
