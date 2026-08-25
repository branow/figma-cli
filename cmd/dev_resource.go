package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newDevResourceCmd groups the dev-resource list/create/update/delete
// subcommands.
func newDevResourceCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev-resource",
		Aliases: []string{"dev-resources"},
		Short:   "Manage dev resources attached to file nodes",
	}
	cmd.AddCommand(
		newDevResourceListCmd(f),
		newDevResourceCreateCmd(f),
		newDevResourceUpdateCmd(f),
		newDevResourceDeleteCmd(f),
	)
	return cmd
}

func newDevResourceListCmd(f *cmdutil.Factory) *cobra.Command {
	var nodeIDs string
	cmd := &cobra.Command{
		Use:     "list <file-key-or-url>",
		Aliases: []string{"ls"},
		Short:   "List dev resources in a file",
		Args:    cobra.ExactArgs(1),
		Example: `  figma dev-resource list abc123
  figma dev-resource list abc123 --nodes 1-2,3-4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			fields := map[string]any{"file_key": key}
			if nodeIDs != "" {
				fields["node_ids"] = figmaurl.NormalizeIDList(nodeIDs)
			}
			resp, err := callAPI(cmd.Context(), f, "getDevResources", fields)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			resources, err := figma.DecodeArray[figma.DevResource](resp.Raw, "dev_resources")
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(resources))
			for _, r := range resources {
				rows = append(rows, []string{r.ID, r.Name, r.NodeID, r.URL})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"ID", "NAME", "NODE", "URL"}, rows)
		},
	}
	cmd.Flags().StringVar(&nodeIDs, "nodes", "", "comma-separated node ids to filter by")
	return cmd
}

func newDevResourceCreateCmd(f *cmdutil.Factory) *cobra.Command {
	var file, node, name, url string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a dev resource on a file node",
		Args:  cobra.NoArgs,
		Example: `  figma dev-resource create --file abc123 --node 1-2 \
    --name "Spec" --url https://example.com/spec`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFlags(
				[2]string{"--file", file}, [2]string{"--node", node},
				[2]string{"--name", name}, [2]string{"--url", url},
			); err != nil {
				return err
			}
			key, err := fileKeyArg(file)
			if err != nil {
				return err
			}
			nodeID := figmaurl.NormalizeNodeID(node)
			resource := map[string]any{
				"file_key": key,
				"node_id":  nodeID,
				"name":     name,
				"url":      url,
			}
			return mutate(cmd.Context(), f, "postDevResources",
				map[string]any{"dev_resources": []any{resource}},
				func(raw []byte) string {
					return fmt.Sprintf("Created dev resource %q on node %s", name, nodeID)
				})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file key or URL")
	cmd.Flags().StringVar(&node, "node", "", "node id to attach the resource to")
	cmd.Flags().StringVar(&name, "name", "", "resource name")
	cmd.Flags().StringVar(&url, "url", "", "resource URL")
	return cmd
}

func newDevResourceUpdateCmd(f *cmdutil.Factory) *cobra.Command {
	var id, name, url string
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update a dev resource by id",
		Args:    cobra.NoArgs,
		Example: `  figma dev-resource update --id 4242 --name "Updated spec"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" {
				return &cmdutil.ValidationError{Message: "--id is required"}
			}
			resource := map[string]any{"id": id}
			if name != "" {
				resource["name"] = name
			}
			if url != "" {
				resource["url"] = url
			}
			return mutate(cmd.Context(), f, "putDevResources",
				map[string]any{"dev_resources": []any{resource}},
				func(raw []byte) string {
					return "Updated dev resource " + id
				})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "dev resource id")
	cmd.Flags().StringVar(&name, "name", "", "new resource name")
	cmd.Flags().StringVar(&url, "url", "", "new resource URL")
	return cmd
}

func newDevResourceDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <file-key-or-url> <dev-resource-id>",
		Short:   "Delete a dev resource",
		Args:    cobra.ExactArgs(2),
		Example: `  figma dev-resource delete abc123 4242`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			_, err = callAPI(cmd.Context(), f, "deleteDevResource",
				map[string]any{"file_key": key, "dev_resource_id": args[1]})
			if err != nil {
				return err
			}
			if !f.Quiet {
				fmt.Fprintf(f.IOStreams.Out, "Deleted dev resource %s\n", args[1])
			}
			return nil
		},
	}
}
