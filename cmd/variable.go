package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
)

// newVariableCmd groups the variable subcommands. Variable endpoints are
// Enterprise-plan gated; the CLI surfaces them and lets the API enforce
// access.
func newVariableCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable",
		Short: "Work with file variables (Enterprise)",
	}
	cmd.AddCommand(newVariableListCmd(f))
	return cmd
}

func newVariableListCmd(f *cmdutil.Factory) *cobra.Command {
	var published bool
	cmd := &cobra.Command{
		Use:   "list <file-key-or-url>",
		Short: "List a file's local (or published) variables",
		Args:  cobra.ExactArgs(1),
		Example: `  figma variable list abc123
  figma variable list abc123 --published`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			op := "getLocalVariables"
			if published {
				op = "getPublishedVariables"
			}
			return runJSONCall(cmd.Context(), f, op, map[string]any{"file_key": key})
		},
	}
	cmd.Flags().BoolVar(&published, "published", false, "list published variables instead of local ones")
	return cmd
}
