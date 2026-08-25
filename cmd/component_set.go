package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/output"
)

// newComponentSetCmd groups the component-set list/get subcommands.
func newComponentSetCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component-set",
		Short: "Work with published component sets",
	}
	cmd.AddCommand(newComponentSetListCmd(f), newComponentSetGetCmd(f))
	return cmd
}

func newComponentSetListCmd(f *cmdutil.Factory) *cobra.Command {
	var file, team string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List component sets in a file or team",
		Args:    cobra.NoArgs,
		Example: `  figma component-set list --file abc123
  figma component-set list --team 1234567890`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runComponentSetList(cmd.Context(), f, file, team)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file key or URL to list component sets from")
	cmd.Flags().StringVar(&team, "team", "", "team id to list published component sets from")
	return cmd
}

func runComponentSetList(ctx context.Context, f *cmdutil.Factory, file, team string) error {
	op, fields, err := libraryScope(f, file, team, "getFileComponentSets", "getTeamComponentSets")
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
	sets, err := figma.DecodeMetaArray[figma.Component](resp.Raw, "component_sets")
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(sets))
	for _, s := range sets {
		rows = append(rows, []string{s.Key, s.Name, truncate(s.Description, 50)})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"KEY", "NAME", "DESCRIPTION"}, rows)
}

func newComponentSetGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Get a published component set by key",
		Args:    cobra.ExactArgs(1),
		Example: `  figma component-set get 8ba4e7a2c...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJSONCall(cmd.Context(), f, "getComponentSet", map[string]any{"key": args[0]})
		},
	}
}
