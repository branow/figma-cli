package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newComponentCmd groups the component list/get subcommands.
func newComponentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component",
		Short: "Work with published components",
	}
	cmd.AddCommand(newComponentListCmd(f), newComponentGetCmd(f))
	return cmd
}

func newComponentListCmd(f *cmdutil.Factory) *cobra.Command {
	var file, team string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List components in a file or team",
		Args:    cobra.NoArgs,
		Example: `  figma component list --file abc123
  figma component list --team 1234567890`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runComponentList(cmd.Context(), f, file, team)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file key or URL to list components from")
	cmd.Flags().StringVar(&team, "team", "", "team id to list published components from")
	return cmd
}

func runComponentList(ctx context.Context, f *cmdutil.Factory, file, team string) error {
	op, fields, err := libraryScope(f, file, team, "getFileComponents", "getTeamComponents")
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
	components, err := figma.DecodeMetaArray[figma.Component](resp.Raw, "components")
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(components))
	for _, c := range components {
		rows = append(rows, []string{c.Key, c.Name, truncate(c.Description, 50)})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"KEY", "NAME", "DESCRIPTION"}, rows)
}

func newComponentGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Get a published component by key",
		Args:    cobra.ExactArgs(1),
		Example: `  figma component get 8ba4e7a2c...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJSONCall(cmd.Context(), f, "getComponent", map[string]any{"key": args[0]})
		},
	}
}

// libraryScope resolves the shared file-or-team choice the component,
// component-set, and style list commands make: exactly one of --file/--team,
// mapping to the file-scoped or team-scoped operation and its path field.
func libraryScope(f *cmdutil.Factory, file, team, fileOp, teamOp string) (string, map[string]any, error) {
	switch {
	case file != "" && team != "":
		return "", nil, &cmdutil.ValidationError{Message: "--file and --team cannot be combined"}
	case file != "":
		key, err := fileKeyArg(file)
		if err != nil {
			return "", nil, err
		}
		return fileOp, map[string]any{"file_key": key}, nil
	case team != "":
		id, ok := figmaurl.TeamID(team)
		if !ok {
			return "", nil, &cmdutil.ValidationError{Message: "could not parse a team id from " + team}
		}
		return teamOp, map[string]any{"team_id": id}, nil
	default:
		if configured := f.Config.TeamID("", f.ActiveProfile()); configured != "" {
			return teamOp, map[string]any{"team_id": configured}, nil
		}
		return "", nil, &cmdutil.ValidationError{Message: "one of --file or --team is required (or configure a default team_id)"}
	}
}
