package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/easel/anneal/internal/manifest"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// requiredShellFunctions are the function names a custom provider script must define.
var requiredShellFunctions = []string{"read", "diff", "emit"}

// shellFuncPattern matches POSIX shell function definitions like "read() {" or "read () {".
var shellFuncPattern = regexp.MustCompile(`(?m)^(\w+)\s*\(\)\s*\{`)

// ShellProvider wraps a shell script implementing read/diff/emit as a Provider.
type ShellProvider struct {
	Kind       string // Provider kind derived from filename
	ScriptPath string // Absolute path to the shell script
	Script     string // Script contents (cached at discovery time)
}

// Plan executes the shell provider's read/diff/emit pipeline and returns
// the emitted operations. The resource spec is passed as ANNEAL_SPEC (JSON)
// and individual spec fields are set as ANNEAL_SPEC_<KEY> environment variables.
func (sp *ShellProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	specJSON, err := json.Marshal(resource.Spec)
	if err != nil {
		return nil, fmt.Errorf("shell provider %s: marshal spec: %w", sp.Kind, err)
	}

	// Build a wrapper script that:
	// 1. Sources the provider script (defines read/diff/emit functions)
	// 2. Calls read to get current state
	// 3. Calls diff to compare desired vs current
	// 4. Calls emit to produce operations
	wrapper := sp.buildPlanScript(string(specJSON), resource.Spec)

	// Execute in the embedded interpreter with stdlib available.
	full := stdlibPreamble + "\n" + wrapper
	prog, err := syntax.NewParser().Parse(strings.NewReader(full), sp.Kind+".sh")
	if err != nil {
		return nil, fmt.Errorf("shell provider %s: parse: %w", sp.Kind, err)
	}

	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
	)
	if err != nil {
		return nil, fmt.Errorf("shell provider %s: interpreter: %w", sp.Kind, err)
	}

	if err := runner.Run(context.Background(), prog); err != nil {
		return nil, fmt.Errorf("shell provider %s: execution failed: %w\nstderr: %s",
			sp.Kind, err, stderr.String())
	}

	// Parse output: each non-empty line is an operation.
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil // Converged — no operations needed.
	}

	return []string{output}, nil
}

// buildPlanScript constructs the shell script that orchestrates read/diff/emit.
func (sp *ShellProvider) buildPlanScript(specJSON string, spec map[string]any) string {
	var buf bytes.Buffer

	// Export the full spec as JSON.
	buf.WriteString(fmt.Sprintf("ANNEAL_SPEC=%s\n", shellQuote(specJSON)))
	buf.WriteString("export ANNEAL_SPEC\n")

	// Export individual spec fields as ANNEAL_SPEC_<KEY> for convenience.
	for key, val := range spec {
		var strVal string
		switch v := val.(type) {
		case string:
			strVal = v
		default:
			b, _ := json.Marshal(v)
			strVal = string(b)
		}
		envKey := "ANNEAL_SPEC_" + strings.ToUpper(key)
		buf.WriteString(fmt.Sprintf("%s=%s\n", envKey, shellQuote(strVal)))
		buf.WriteString(fmt.Sprintf("export %s\n", envKey))
	}

	// Source the provider script to define read/diff/emit functions.
	buf.WriteString("\n# Provider functions\n")
	buf.WriteString(sp.Script)
	buf.WriteString("\n\n")

	// Call the pipeline: read → diff → emit.
	// read outputs current state to a temp variable.
	// diff compares spec (via env) against current state (via stdin).
	// emit produces stdlib operations.
	buf.WriteString("_anneal_current=\"$(read)\"\n")
	buf.WriteString("_anneal_changes=\"$(printf '%s' \"$_anneal_current\" | diff)\"\n")
	buf.WriteString("if [ -n \"$_anneal_changes\" ]; then\n")
	buf.WriteString("  printf '%s' \"$_anneal_changes\" | emit\n")
	buf.WriteString("fi\n")

	return buf.String()
}

// DiscoverShellProviders scans the providers/ directory relative to the manifest
// for shell scripts and returns validated ShellProvider instances.
func DiscoverShellProviders(manifestPath string) ([]*ShellProvider, error) {
	manifestDir := filepath.Dir(manifestPath)
	if !filepath.IsAbs(manifestDir) {
		abs, err := filepath.Abs(manifestDir)
		if err != nil {
			return nil, fmt.Errorf("resolve manifest directory: %w", err)
		}
		manifestDir = abs
	}

	providersDir := filepath.Join(manifestDir, "providers")
	entries, err := os.ReadDir(providersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No providers directory — not an error.
		}
		return nil, fmt.Errorf("read providers directory: %w", err)
	}

	var providers []*ShellProvider
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sh") {
			continue
		}

		kind := strings.TrimSuffix(name, ".sh")
		scriptPath := filepath.Join(providersDir, name)
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			return nil, fmt.Errorf("read provider script %s: %w", name, err)
		}

		providers = append(providers, &ShellProvider{
			Kind:       kind,
			ScriptPath: scriptPath,
			Script:     string(content),
		})
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Kind < providers[j].Kind
	})

	return providers, nil
}

// ValidateShellProvider checks that a shell script defines all required functions
// (read, diff, emit). Returns an error describing which functions are missing.
func ValidateShellProvider(sp *ShellProvider) error {
	defined := make(map[string]bool)
	matches := shellFuncPattern.FindAllStringSubmatch(sp.Script, -1)
	for _, m := range matches {
		defined[m[1]] = true
	}

	var missing []string
	for _, fn := range requiredShellFunctions {
		if !defined[fn] {
			missing = append(missing, fn+"()")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("provider %s (%s): missing required functions: %s",
			sp.Kind, filepath.Base(sp.ScriptPath), strings.Join(missing, ", "))
	}
	return nil
}
