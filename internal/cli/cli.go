package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/easel/anneal/internal/engine"
	"github.com/easel/anneal/internal/manifest"
	"github.com/spf13/cobra"
)

const (
	ExitCodeSuccess       = 0
	ExitCodeRuntimeError  = 1
	ExitCodeUsageError    = 2
	ExitCodeUnimplemented = 3
)

type exitError struct {
	code   int
	err    error
	silent bool // When true, do not print error to stderr (used for JSON output)
}

func (e *exitError) Error() string {
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

func Execute(args []string, stdout, stderr io.Writer, version string) int {
	cmd := newRootCmd(stdout, stderr, version)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	if err := cmd.Execute(); err != nil {
		var coded *exitError
		if errors.As(err, &coded) {
			if !coded.silent {
				fmt.Fprintln(stderr, coded.err)
			}
			return coded.code
		}
		fmt.Fprintln(stderr, err)
		if errors.Is(err, flagParseError{}) {
			return ExitCodeUsageError
		}
		return ExitCodeRuntimeError
	}

	return ExitCodeSuccess
}

type flagParseError struct{}

func (flagParseError) Error() string { return "usage error" }

type options struct {
	manifestPath string
	hostVarsFile string
}

func newRootCmd(stdout, stderr io.Writer, version string) *cobra.Command {
	opts := options{}

	root := &cobra.Command{
		Use:           "anneal",
		Short:         "Declarative host configuration engine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return errors.Join(flagParseError{}, err)
	})
	root.PersistentFlags().StringVarP(
		&opts.manifestPath,
		"manifest",
		"f",
		"anneal.yaml",
		"Path to the manifest file",
	)
	root.PersistentFlags().StringVar(
		&opts.hostVarsFile,
		"host-vars",
		"",
		"Path to host-specific variable overrides file",
	)

	root.AddCommand(newValidateCmd(&opts))
	root.AddCommand(newPlanCmd(&opts))
	root.AddCommand(newApplyCmd(&opts))
	root.AddCommand(newVersionCmd(version))

	return root
}

func newValidateCmd(opts *options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the manifest without touching system state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := manifest.LoadResolved(opts.manifestPath, manifest.ResolveOptions{
				Env:          currentEnv(),
				Builtins:     manifest.CurrentBuiltins(),
				HostVarsFile: opts.hostVarsFile,
			})
			if err != nil {
				if jsonOutput {
					if wErr := writeJSON(cmd.OutOrStdout(), validateOutput{
						Valid:  false,
						Issues: []validateIssue{{Level: "error", Message: err.Error()}},
					}); wErr != nil {
						return wErr
					}
					return &exitError{code: ExitCodeRuntimeError, err: err, silent: true}
				}
				return err
			}
			if err := engine.NewPlanner().Validate(resolved.Resources); err != nil {
				if jsonOutput {
					if wErr := writeJSON(cmd.OutOrStdout(), validateOutput{
						Valid:  false,
						Issues: []validateIssue{{Level: "error", Message: err.Error()}},
					}); wErr != nil {
						return wErr
					}
					return &exitError{code: ExitCodeRuntimeError, err: err, silent: true}
				}
				return err
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), validateOutput{
					Valid:  true,
					Issues: []validateIssue{},
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "manifest %s is valid\n", opts.manifestPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output structured JSON")
	return cmd
}

func newPlanCmd(opts *options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Build an execution plan from the manifest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := manifest.LoadResolved(opts.manifestPath, manifest.ResolveOptions{
				Env:          currentEnv(),
				Builtins:     manifest.CurrentBuiltins(),
				HostVarsFile: opts.hostVarsFile,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				plan, err := engine.NewPlanner().BuildPlan(resolved.Resources)
				if err != nil {
					return err
				}
				var resources []planResourceOutput
				for _, rp := range plan.Resources {
					status := "converged"
					if rp.Script != "" {
						status = "changed"
					}
					resources = append(resources, planResourceOutput{
						Name:       rp.Name,
						Kind:       rp.Kind,
						Status:     status,
						Operations: rp.Script,
						Trigger:    rp.Trigger,
					})
				}
				return writeJSON(cmd.OutOrStdout(), planOutput{Resources: resources})
			}
			plan, err := engine.NewPlanner().Build(resolved.Resources)
			if err != nil {
				return err
			}
			if plan == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "# plan is empty")
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), plan)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output structured JSON")
	return cmd
}

func newApplyCmd(opts *options) *cobra.Command {
	var planFile string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply the manifest, converging the system to desired state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := manifest.LoadResolved(opts.manifestPath, manifest.ResolveOptions{
				Env:          currentEnv(),
				Builtins:     manifest.CurrentBuiltins(),
				HostVarsFile: opts.hostVarsFile,
			})
			if err != nil {
				return err
			}
			planner := engine.NewPlanner()

			var savedScript string
			if planFile != "" {
				savedScript, err = loadSavedPlan(planFile)
				if err != nil {
					return err
				}
			}

			result, err := planner.Apply(engine.EmbeddedShell{}, resolved.Resources, savedScript)
			if err != nil {
				return err
			}

			if jsonOutput {
				var resources []applyResourceOutput
				for _, r := range result.Results {
					entry := applyResourceOutput{
						Name:   r.Name,
						Kind:   r.Kind,
						Status: r.Status.String(),
					}
					if r.Error != nil {
						errMsg := r.Error.Error()
						entry.Error = &errMsg
					}
					resources = append(resources, entry)
				}
				if writeErr := writeJSON(cmd.OutOrStdout(), applyOutput{
					Success:   !result.Failed(),
					Resources: resources,
				}); writeErr != nil {
					return writeErr
				}
				if result.Failed() {
					return &exitError{
						code:   ExitCodeRuntimeError,
						err:    errors.New("apply failed"),
						silent: true,
					}
				}
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), result.Summary())
			if result.Failed() {
				return &exitError{
					code: ExitCodeRuntimeError,
					err:  errors.New("apply failed"),
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&planFile, "plan", "", "Path to a saved plan file for drift detection")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output structured JSON")
	return cmd
}

// loadSavedPlan reads a saved plan script file. Apply will re-plan and
// compare the script output against this to detect drift.
func loadSavedPlan(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read saved plan: %w", err)
	}
	return string(data), nil
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the anneal version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	}
}

// JSON output types for --json flag.

type validateOutput struct {
	Valid  bool            `json:"valid"`
	Issues []validateIssue `json:"issues"`
}

type validateIssue struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type planOutput struct {
	Resources []planResourceOutput `json:"resources"`
}

type planResourceOutput struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Status     string `json:"status"` // "changed" or "converged"
	Operations string `json:"operations,omitempty"`
	Trigger    bool   `json:"trigger,omitempty"`
}

type applyOutput struct {
	Success   bool                  `json:"success"`
	Resources []applyResourceOutput `json:"resources"`
}

type applyResourceOutput struct {
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	Status string  `json:"status"` // "applied", "failed", "skipped", "converged"
	Error  *string `json:"error,omitempty"`
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func currentEnv() map[string]string {
	env := make(map[string]string, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		env[key] = value
	}
	return env
}
