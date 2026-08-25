package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newFileCmd groups the file get/nodes/meta/versions subcommands.
func newFileCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Work with Figma files",
	}
	cmd.AddCommand(
		newFileGetCmd(f),
		newFileNodesCmd(f),
		newFileMetaCmd(f),
		newFileVersionsCmd(f),
	)
	return cmd
}

// fileKeyArg resolves a file argument that may be a full Figma URL or a bare
// key into the key the API wants.
func fileKeyArg(arg string) (string, error) {
	key, ok := figmaurl.FileKey(arg)
	if !ok {
		return "", &cmdutil.ValidationError{Message: "could not parse a file key from " + arg}
	}
	return key, nil
}

func newFileGetCmd(f *cmdutil.Factory) *cobra.Command {
	var ids, version, depth, geometry string
	cmd := &cobra.Command{
		Use:   "get <file-key-or-url>",
		Short: "Get a file's document JSON",
		Args:  cobra.ExactArgs(1),
		Example: `  figma file get abc123
  figma file get https://www.figma.com/design/abc123/My-Design --depth 2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDepth(depth); err != nil {
				return err
			}
			if err := validateGeometry(geometry); err != nil {
				return err
			}
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			fields := map[string]any{"file_key": key}
			addOptional(fields, "version", version)
			addOptional(fields, "depth", depth)
			addOptional(fields, "geometry", geometry)
			if ids != "" {
				fields["ids"] = figmaurl.NormalizeIDList(ids)
			}
			return runJSONCall(cmd.Context(), f, "getFile", fields)
		},
	}
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated node ids to include")
	cmd.Flags().StringVar(&version, "version", "", "a specific version id")
	cmd.Flags().StringVar(&depth, "depth", "", "how deep to traverse the document tree")
	cmd.Flags().StringVar(&geometry, "geometry", "", `set to "paths" to export vector data`)
	return cmd
}

func newFileNodesCmd(f *cmdutil.Factory) *cobra.Command {
	var ids, version, depth, geometry string
	cmd := &cobra.Command{
		Use:   "nodes <file-key-or-url>",
		Short: "Get JSON for specific nodes of a file",
		Args:  cobra.ExactArgs(1),
		Example: `  figma file nodes abc123 --ids 1-2,3-4
  figma file nodes "https://www.figma.com/design/abc123/N?node-id=1-2"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDepth(depth); err != nil {
				return err
			}
			if err := validateGeometry(geometry); err != nil {
				return err
			}
			ref, ok := figmaurl.Parse(args[0])
			if !ok {
				return &cmdutil.ValidationError{Message: "could not parse a file key from " + args[0]}
			}
			nodes := figmaurl.NormalizeIDList(ids)
			if nodes == "" {
				nodes = ref.NodeID
			}
			if nodes == "" {
				return &cmdutil.ValidationError{Message: "at least one node id is required (use --ids or a URL with node-id)"}
			}
			fields := map[string]any{"file_key": ref.FileKey, "ids": nodes}
			addOptional(fields, "version", version)
			addOptional(fields, "depth", depth)
			addOptional(fields, "geometry", geometry)
			return runJSONCall(cmd.Context(), f, "getFileNodes", fields)
		},
	}
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated node ids to retrieve")
	cmd.Flags().StringVar(&version, "version", "", "a specific version id")
	cmd.Flags().StringVar(&depth, "depth", "", "how deep to traverse from each node")
	cmd.Flags().StringVar(&geometry, "geometry", "", `set to "paths" to export vector data`)
	return cmd
}

func newFileMetaCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "meta <file-key-or-url>",
		Short: "Get a file's metadata",
		Args:  cobra.ExactArgs(1),
		Example: `  figma file meta abc123
  figma file meta abc123 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			resp, err := callAPI(cmd.Context(), f, "getFileMeta", map[string]any{"file_key": key})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			var envelope struct {
				File map[string]any `json:"file"`
			}
			if err := json.Unmarshal(resp.Raw, &envelope); err != nil {
				return err
			}
			pairs := [][2]string{{"Key", key}}
			for _, k := range []string{"name", "last_touched_at", "editor_type", "role", "link_access"} {
				if v, ok := envelope.File[k]; ok {
					pairs = append(pairs, [2]string{k, scalarString(v)})
				}
			}
			return writeKeyValues(f, pairs)
		},
	}
}

func newFileVersionsCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "versions <file-key-or-url>",
		Short: "List a file's version history",
		Long: `List a file's version history. The history is paged; --limit caps the
number returned and --paginate follows every page until exhausted (or --limit
is reached), otherwise only the first page is shown.`,
		Args: cobra.ExactArgs(1),
		Example: `  figma file versions abc123
  figma file versions abc123 --paginate
  figma file versions abc123 --limit 50 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			// The default (no paging flags) keeps the original single-call
			// output byte-for-byte, including the pagination object under -o
			// json; paging only engages when the user asks for it.
			if !paginate && limit <= 0 {
				resp, err := callAPI(cmd.Context(), f, "getFileVersions", map[string]any{"file_key": key})
				if err != nil {
					return err
				}
				if wantJSON(f) {
					return writeResultJSON(f, resp.Raw)
				}
				versions, err := figma.DecodeArray[figma.Version](resp.Raw, "versions")
				if err != nil {
					return err
				}
				return writeVersionsTable(f, versions)
			}

			fetch := pageFetcher(cmd.Context(), f, "getFileVersions", map[string]any{"file_key": key})
			items, err := figma.Paginate("versions", fetch, figma.NextVersionsPage, paginate, limit)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				merged, err := figma.MergeItems("versions", items)
				if err != nil {
					return err
				}
				return writeResultJSON(f, merged)
			}
			versions, err := figma.DecodeItems[figma.Version](items)
			if err != nil {
				return err
			}
			return writeVersionsTable(f, versions)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of versions to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	return cmd
}

func writeVersionsTable(f *cmdutil.Factory, versions []figma.Version) error {
	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		rows = append(rows, []string{v.ID, v.Label, v.CreatedAt, v.User.Handle})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"ID", "LABEL", "CREATED", "AUTHOR"}, rows)
}
