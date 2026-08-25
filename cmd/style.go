package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/output"
)

// newStyleCmd groups the style list/get subcommands.
func newStyleCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "style",
		Short: "Work with published styles",
	}
	cmd.AddCommand(newStyleListCmd(f), newStyleGetCmd(f))
	return cmd
}

func newStyleListCmd(f *cmdutil.Factory) *cobra.Command {
	var file, team string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List styles in a file or team",
		Args:    cobra.NoArgs,
		Example: `  figma style list --file abc123
  figma style list --team 1234567890`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStyleList(cmd.Context(), f, file, team)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file key or URL to list styles from")
	cmd.Flags().StringVar(&team, "team", "", "team id to list published styles from")
	return cmd
}

func runStyleList(ctx context.Context, f *cmdutil.Factory, file, team string) error {
	op, fields, err := libraryScope(f, file, team, "getFileStyles", "getTeamStyles")
	if err != nil {
		return err
	}
	resp, err := callAPI(ctx, f, op, fields)
	if err != nil {
		return err
	}
	if wantJSON(f) {
		return writeResultJSON(f, resp.Raw)
	}
	styles, err := figma.DecodeMetaArray[figma.Style](resp.Raw, "styles")
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(styles))
	for _, s := range styles {
		rows = append(rows, []string{s.Key, s.Name, s.StyleType, truncate(s.Description, 40)})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"KEY", "NAME", "TYPE", "DESCRIPTION"}, rows)
}

func newStyleGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Get a published style by key",
		Args:    cobra.ExactArgs(1),
		Example: `  figma style get 8ba4e7a2c...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJSONCall(cmd.Context(), f, "getStyle", map[string]any{"key": args[0]})
		},
	}
}
