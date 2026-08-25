package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaurl"
	"github.com/branow/figma-cli/internal/output"
)

// newImageCmd groups the image render/fills subcommands.
func newImageCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Render node images and list image fills",
	}
	cmd.AddCommand(newImageRenderCmd(f), newImageFillsCmd(f))
	return cmd
}

func newImageRenderCmd(f *cmdutil.Factory) *cobra.Command {
	var ids, format, scale, version, out string
	var concurrency int
	cmd := &cobra.Command{
		Use:   "render <file-key-or-url>",
		Short: "Render node images, optionally downloading them to disk",
		Long: `Render one or more nodes of a file to images. Figma returns short-lived
URLs to the rendered assets; with --out those assets are downloaded to disk,
which is the reason to reach for this over the raw API. Without --out the
resolved URLs are printed.

--out may be a directory (assets are named by node id) or, when exactly one
node is rendered, a file path such as ./logo.png. Many assets download in
parallel; --concurrency caps how many at once.`,
		Args: cobra.ExactArgs(1),
		Example: `  # Print the rendered-asset URLs
  figma image render abc123 --ids 1-2,3-4

  # Download PNGs to ./assets
  figma image render "https://www.figma.com/design/abc123/N?node-id=1-2" --out ./assets

  # Download a single node to a chosen file name
  figma image render abc123 --ids 1-2 --format svg --out ./logo.svg`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageRender(cmd.Context(), f, args[0], ids, format, scale, version, out, concurrency)
		},
	}
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated node ids to render")
	cmd.Flags().StringVar(&format, "format", "", "image format: png|jpg|svg|pdf (default png)")
	cmd.Flags().StringVar(&scale, "scale", "", "scale factor between 0.01 and 4")
	cmd.Flags().StringVar(&version, "version", "", "a specific version id")
	cmd.Flags().StringVar(&out, "out", "", "download assets to this directory (or file, for a single node) instead of printing URLs")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "maximum simultaneous asset downloads")
	return cmd
}

func runImageRender(ctx context.Context, f *cmdutil.Factory, arg, ids, format, scale, version, out string, concurrency int) error {
	if err := validateRenderFormat(format); err != nil {
		return err
	}
	if err := validateRenderScale(scale); err != nil {
		return err
	}
	ref, ok := figmaurl.Parse(arg)
	if !ok {
		return &cmdutil.ValidationError{Message: "could not parse a file key from " + arg}
	}
	nodes := figmaurl.NormalizeIDList(ids)
	if nodes == "" {
		nodes = ref.NodeID
	}
	if nodes == "" {
		return &cmdutil.ValidationError{Message: "at least one node id is required (use --ids or a URL with node-id)"}
	}

	fields := map[string]any{"file_key": ref.FileKey, "ids": nodes}
	addOptional(fields, "format", format)
	addOptional(fields, "scale", scale)
	addOptional(fields, "version", version)

	resp, err := callAPI(ctx, f, "getImages", fields)
	if err != nil {
		return err
	}
	result, err := figma.ParseRenderResult(resp.Raw)
	if err != nil {
		return err
	}
	if result.Err != "" {
		return &figmaError{message: result.Err}
	}

	if out == "" {
		if wantJSON(f) {
			return writeResultJSON(f, resp.Raw)
		}
		rows := make([][]string, 0, len(result.Images))
		for node, url := range result.Images {
			value := "(render failed)"
			if url != nil {
				value = *url
			}
			rows = append(rows, []string{node, value})
		}
		return output.WriteTable(f.IOStreams.Out, []string{"NODE", "URL"}, rows)
	}

	if looksLikeFile(out) {
		return renderToFile(ctx, f, result, out)
	}
	return renderToDir(ctx, f, result, out, format, concurrency)
}

// renderToDir downloads every rendered node into dir, then prints one line per
// asset and a final summary.
func renderToDir(ctx context.Context, f *cmdutil.Factory, result figma.RenderResult, dir, format string, concurrency int) error {
	saved, failed, err := figma.Download(ctx, httpGet, result, dir, format, concurrency)
	if err != nil {
		return err
	}
	if !f.Quiet {
		for _, s := range saved {
			fmt.Fprintf(f.IOStreams.Out, "%s -> %s\n", s.NodeID, s.Path)
		}
		for _, node := range failed {
			fmt.Fprintf(f.IOStreams.ErrOut, "warning: node %s did not render\n", node)
		}
		fmt.Fprintf(f.IOStreams.Out, "Downloaded %d/%d assets to %s\n", len(saved), len(saved)+len(failed), dir)
	}
	// Every requested node failed to render: report the failure rather than
	// exit 0 on an empty download, matching the single-file path's guard and
	// keeping a total failure detectable from a script's exit code.
	if len(saved) == 0 && len(failed) > 0 {
		return &figmaError{message: "no node rendered successfully"}
	}
	return nil
}

// renderToFile writes the single rendered node to the exact path in out. It
// errors when the render produced no asset or more than one, since a lone file
// name cannot hold several.
func renderToFile(ctx context.Context, f *cmdutil.Factory, result figma.RenderResult, out string) error {
	var url, node string
	rendered := 0
	for n, u := range result.Images {
		if u != nil && *u != "" {
			url, node = *u, n
			rendered++
		}
	}
	if rendered == 0 {
		return &figmaError{message: "no node rendered successfully"}
	}
	if rendered > 1 {
		return &cmdutil.ValidationError{Message: fmt.Sprintf("--out %q names a file but %d nodes rendered; pass a directory to download more than one", out, rendered)}
	}
	if err := figma.SaveAsset(ctx, httpGet, url, out); err != nil {
		return err
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "%s -> %s\n", node, out)
		fmt.Fprintf(f.IOStreams.Out, "Downloaded 1/1 assets to %s\n", out)
	}
	return nil
}

// looksLikeFile reports whether out should be treated as a single-file target
// rather than a directory: it is not an existing directory, does not end in a
// path separator, and carries a file extension.
func looksLikeFile(out string) bool {
	if fi, err := os.Stat(out); err == nil {
		return !fi.IsDir()
	}
	if strings.HasSuffix(out, string(os.PathSeparator)) {
		return false
	}
	return filepath.Ext(out) != ""
}

func newImageFillsCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "fills <file-key-or-url>",
		Short: "List image-fill download URLs for a file",
		Args:  cobra.ExactArgs(1),
		Example: `  figma image fills abc123
  figma image fills abc123 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			resp, err := callAPI(cmd.Context(), f, "getImageFills", map[string]any{"file_key": key})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return writeResultJSON(f, resp.Raw)
			}
			var envelope struct {
				Meta struct {
					Images map[string]string `json:"images"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(resp.Raw, &envelope); err != nil {
				return err
			}
			rows := make([][]string, 0, len(envelope.Meta.Images))
			for ref, url := range envelope.Meta.Images {
				rows = append(rows, []string{ref, url})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"REF", "URL"}, rows)
		},
	}
}

// httpGet fetches an asset URL with a bounded timeout. It is separate from
// the figmaapi client because these are pre-signed S3 URLs, not Figma API
// endpoints, and carry no auth header.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("asset download returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// figmaError adapts a Figma-reported render error into a plain command error.
type figmaError struct{ message string }

func (e *figmaError) Error() string { return e.message }
