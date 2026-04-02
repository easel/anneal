package engine

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/erik/anneal/internal/manifest"
)

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

func (p *Planner) Build(resources []manifest.ResolvedResource) (string, error) {
	ordered, err := topoSort(resources)
	if err != nil {
		return "", err
	}

	var ops []string
	for _, resource := range ordered {
		provider, ok := p.providers[resource.Kind]
		if !ok {
			return "", fmt.Errorf("resource %s: unknown kind %q", resource.Name, resource.Kind)
		}
		resourceOps, err := provider.Plan(resource)
		if err != nil {
			return "", fmt.Errorf("resource %s: %w", resource.Name, err)
		}
		ops = append(ops, resourceOps...)
	}
	if len(ops) == 0 {
		return "", nil
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
	return buf.String(), nil
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
	return []string{
		fmt.Sprintf("stdlib_file_write %s %s %s <<'ANNEAL_EOF'\n%s\nANNEAL_EOF", path, mode, owner, content),
	}, nil
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
