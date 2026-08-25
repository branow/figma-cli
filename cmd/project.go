package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newProjectCmd groups the project list/files subcommands.
func newProjectCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Work with team projects and their files",
	}
	cmd.AddCommand(newProjectListCmd(f), newProjectFilesCmd(f))
	return cmd
}

// resolveTeam resolves the team id from an argument, then --team/config. The
// argument may be a bare id or a pasted Figma team URL. It errors when none is
// available so team-scoped commands fail with clear guidance instead of a
// confusing API error.
func resolveTeam(f *cmdutil.Factory, arg string) (string, error) {
	if arg != "" {
		id, ok := figmaurl.TeamID(arg)
		if !ok {
			return "", &cmdutil.ValidationError{Message: "could not parse a team id from " + arg}
		}
		return id, nil
	}
	if team := f.Config.TeamID("", f.ActiveProfile()); team != "" {
		return team, nil
	}
	return "", &cmdutil.ValidationError{Message: "a team id is required (pass it as an argument, use FIGMA_TEAM_ID, or run: figma config set team_id <id>)"}
}

func newProjectListCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "list [team-id-or-url]",
		Aliases: []string{"ls"},
		Short:   "List a team's projects",
		Args:    cobra.MaximumNArgs(1),
		Example: `  figma project list 1234567890
  figma project list "https://www.figma.com/files/team/1234567890/My-Team"
  figma project list          # uses the configured team_id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			team, err := resolveTeam(f, firstArg(args))
			if err != nil {
				return err
			}
			resp, err := callAPI(cmd.Context(), f, "getTeamProjects", map[string]any{"team_id": team})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			projects, err := figma.DecodeArray[figma.Project](resp.Raw, "projects")
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(projects))
			for _, p := range projects {
				rows = append(rows, []string{p.ID, p.Name})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"ID", "NAME"}, rows)
		},
	}
}

func newProjectFilesCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "files <project-id-or-url>",
		Short: "List the files in a project",
		Args:  cobra.ExactArgs(1),
		Example: `  figma project files 55391681
  figma project files "https://www.figma.com/files/project/55391681/My-Project"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, ok := figmaurl.ProjectID(args[0])
			if !ok {
				return &cmdutil.ValidationError{Message: "could not parse a project id from " + args[0]}
			}
			resp, err := callAPI(cmd.Context(), f, "getProjectFiles", map[string]any{"project_id": project})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			files, err := figma.DecodeArray[figma.File](resp.Raw, "files")
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(files))
			for _, file := range files {
				rows = append(rows, []string{file.Key, file.Name, file.LastModified})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"KEY", "NAME", "MODIFIED"}, rows)
		},
	}
}

// firstArg returns the first argument or "" when none was given.
func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
