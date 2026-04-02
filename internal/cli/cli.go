package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/erik/anneal/internal/engine"
	"github.com/erik/anneal/internal/manifest"
	"github.com/spf13/cobra"
)

const (
	ExitCodeSuccess       = 0
	ExitCodeRuntimeError  = 1
	ExitCodeUsageError    = 2
	ExitCodeUnimplemented = 3
)

type exitError struct {
	code int
	err  error
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
			fmt.Fprintln(stderr, coded.err)
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

	root.AddCommand(newValidateCmd(&opts))
	root.AddCommand(newPlanCmd(&opts))
	root.AddCommand(newApplyCmd(&opts))
	root.AddCommand(newVersionCmd(version))

	return root
}

func newValidateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the manifest without touching system state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := manifest.LoadResolved(opts.manifestPath, manifest.ResolveOptions{
				Env:      currentEnv(),
				Builtins: manifest.CurrentBuiltins(),
			})
			if err != nil {
				return err
			}
			if err := engine.NewPlanner().Validate(resolved.Resources); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "manifest %s is valid\n", opts.manifestPath)
			return nil
		},
	}
}

func newPlanCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Build an execution plan from the manifest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := manifest.LoadResolved(opts.manifestPath, manifest.ResolveOptions{
				Env:      currentEnv(),
				Builtins: manifest.CurrentBuiltins(),
			})
			if err != nil {
				return err
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
}

func newApplyCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply a previously generated plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := manifest.Load(opts.manifestPath); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "apply command is not implemented yet")
			return &exitError{
				code: ExitCodeUnimplemented,
				err:  errors.New("apply pipeline not implemented"),
			}
		},
	}
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
