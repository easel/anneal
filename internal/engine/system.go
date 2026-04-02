package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// System abstracts shell execution so providers and apply logic
// can be tested against a mock without touching the real OS.
type System interface {
	// Execute runs a shell script and returns combined output.
	// It returns an error if the script exits non-zero.
	Execute(script string) (string, error)
}

// EmbeddedShell executes scripts using the mvdan/sh pure-Go interpreter.
type EmbeddedShell struct{}

func (EmbeddedShell) Execute(script string) (string, error) {
	full := stdlibPreamble + "\n" + script
	prog, err := syntax.NewParser().Parse(strings.NewReader(full), "plan")
	if err != nil {
		return "", fmt.Errorf("parse script: %w", err)
	}
	var out bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &out, &out),
	)
	if err != nil {
		return "", fmt.Errorf("create interpreter: %w", err)
	}
	err = runner.Run(context.Background(), prog)
	if err != nil {
		return out.String(), fmt.Errorf("script failed: %w\n%s", err, out.String())
	}
	return out.String(), nil
}

// MockSystem records executed scripts for testing.
type MockSystem struct {
	Executed []string
	// FailOn maps a script substring to an error; if the script contains
	// the substring, Execute returns the error.
	FailOn map[string]error
}

func (m *MockSystem) Execute(script string) (string, error) {
	m.Executed = append(m.Executed, script)
	for substr, err := range m.FailOn {
		if strings.Contains(script, substr) {
			return "", err
		}
	}
	return "", nil
}
