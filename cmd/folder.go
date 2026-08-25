package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newFolderCmd groups the folder list/files subcommands (the v2 team-folder
// endpoints).
func newFolderCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Work with team folders and their files",
	}
	cmd.AddCommand(newFolderListCmd(f), newFolderFilesCmd(f))
	return cmd
}

func newFolderListCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "list [team-id-or-url]",
		Aliases: []string{"ls"},
		Short:   "List a team's top-level folders",
		Args:    cobra.MaximumNArgs(1),
		Example: `  figma folder list 1234567890
  figma folder list "https://www.figma.com/files/team/1234567890/My-Team"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			team, err := resolveTeam(f, firstArg(args))
			if err != nil {
				return err
			}
			resp, err := callAPI(cmd.Context(), f, "getTeamFolders", map[string]any{"team_id": team})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			folders, err := figma.DecodeArray[figma.Project](resp.Raw, "folders")
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(folders))
			for _, folder := range folders {
				rows = append(rows, []string{folder.ID, folder.Name})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"ID", "NAME"}, rows)
		},
	}
}

func newFolderFilesCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "files <folder-id-or-url>",
		Short: "List the files in a folder",
		Args:  cobra.ExactArgs(1),
		Example: `  figma folder files 987654321
  figma folder files "https://www.figma.com/files/folder/987654321"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			folder, ok := figmaurl.FolderID(args[0])
			if !ok {
				return &cmdutil.ValidationError{Message: "could not parse a folder id from " + args[0]}
			}
			resp, err := callAPI(cmd.Context(), f, "getFolderFiles", map[string]any{"folder_id": folder})
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
