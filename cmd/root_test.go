package cmd_test

import (
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

// TestUsageErrorsExitValidation pins the reconciled contract from the QA
// review: an unknown command, an unknown nested command, an invalid enum
// argument, and a bad flag are all user/validation errors and must exit 3,
// matching the documented exit-code table instead of leaking Cobra's exit 1.
func TestUsageErrorsExitValidation(t *testing.T) {
	cases := map[string][]string{
		"unknown top-level command": {"frobnicate"},
		"unknown nested command":    {"file", "frobnicate"},
		"invalid completion shell":  {"completion", "tcsh"},
		"unknown flag":              {"me", "--nope"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			f, _, _ := newTestFactory(t, "")
			root := cmd.NewRootCmd(f)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("expected an error for %v", args)
			}
			if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
				t.Errorf("got exit code %d, want %d for %v", got, cmdutil.ExitValidation, args)
			}
		})
	}
}

// TestGroupCommandWithoutArgsShowsHelp confirms the runnable-group shim added
// for the unknown-command check still lets a bare group command print help and
// succeed.
func TestGroupCommandWithoutArgsShowsHelp(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetOut(new(strings.Builder))
	root.SetArgs([]string{"file"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bare group command should print help and succeed, got: %v", err)
	}
}
