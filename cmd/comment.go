package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/output"
)

// newCommentCmd groups the comment list/add/delete/react/unreact
// subcommands.
func newCommentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Work with file comments and reactions",
	}
	cmd.AddCommand(
		newCommentListCmd(f),
		newCommentAddCmd(f),
		newCommentDeleteCmd(f),
		newCommentReactCmd(f),
		newCommentUnreactCmd(f),
	)
	return cmd
}

func newCommentListCmd(f *cmdutil.Factory) *cobra.Command {
	var asMarkdown bool
	var limit int
	cmd := &cobra.Command{
		Use:     "list <file-key-or-url>",
		Aliases: []string{"ls"},
		Short:   "List comments on a file",
		Long: `List comments on a file. Figma returns the whole thread in a single
response, so --limit simply caps how many of the newest-listed comments are
shown.`,
		Args: cobra.ExactArgs(1),
		Example: `  figma comment list abc123
  figma comment list abc123 --limit 20
  figma comment list abc123 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			fields := map[string]any{"file_key": key}
			if asMarkdown {
				fields["as_md"] = true
			}
			resp, err := callAPI(cmd.Context(), f, "getComments", fields)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				if limit <= 0 {
					return writeResultJSON(f, resp.Raw)
				}
				items, err := figma.RawArray(resp.Raw, "comments")
				if err != nil {
					return err
				}
				merged, err := figma.MergeItems("comments", capItems(items, limit))
				if err != nil {
					return err
				}
				return writeResultJSON(f, merged)
			}
			comments, err := figma.DecodeArray[figma.Comment](resp.Raw, "comments")
			if err != nil {
				return err
			}
			if limit > 0 && len(comments) > limit {
				comments = comments[:limit]
			}
			rows := make([][]string, 0, len(comments))
			for _, c := range comments {
				rows = append(rows, []string{c.ID, c.User.Handle, truncate(c.Message, 60), c.CreatedAt})
			}
			return output.WriteTable(f.IOStreams.Out, []string{"ID", "AUTHOR", "MESSAGE", "CREATED"}, rows)
		},
	}
	cmd.Flags().BoolVar(&asMarkdown, "markdown", false, "return comment text as markdown where applicable")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of comments to show")
	return cmd
}

// capItems truncates items to at most limit (limit <= 0 means no cap).
func capItems[T any](items []T, limit int) []T {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func newCommentAddCmd(f *cmdutil.Factory) *cobra.Command {
	var message, replyTo string
	cmd := &cobra.Command{
		Use:   "add <file-key-or-url>",
		Short: "Add a comment to a file",
		Args:  cobra.ExactArgs(1),
		Example: `  figma comment add abc123 --message "Looks great"
  figma comment add abc123 --message "Agreed" --reply-to 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if message == "" {
				return &cmdutil.ValidationError{Message: "--message is required"}
			}
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			fields := map[string]any{"file_key": key, "message": message}
			addOptional(fields, "comment_id", replyTo)
			return mutate(cmd.Context(), f, "postComment", fields, func(raw []byte) string {
				if id := decodeID(raw); id != "" {
					return "Added comment " + id
				}
				return "Added comment"
			})
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "the comment text")
	cmd.Flags().StringVar(&replyTo, "reply-to", "", "id of the root comment to reply to")
	return cmd
}

func newCommentDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <file-key-or-url> <comment-id>",
		Short:   "Delete a comment",
		Args:    cobra.ExactArgs(2),
		Example: `  figma comment delete abc123 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			_, err = callAPI(cmd.Context(), f, "deleteComment", map[string]any{"file_key": key, "comment_id": args[1]})
			if err != nil {
				return err
			}
			if !f.Quiet {
				fmt.Fprintf(f.IOStreams.Out, "Deleted comment %s\n", args[1])
			}
			return nil
		},
	}
}

func newCommentReactCmd(f *cmdutil.Factory) *cobra.Command {
	var emoji string
	cmd := &cobra.Command{
		Use:     "react <file-key-or-url> <comment-id>",
		Short:   "Add a reaction to a comment",
		Args:    cobra.ExactArgs(2),
		Example: `  figma comment react abc123 12345 --emoji :eyes:`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if emoji == "" {
				return &cmdutil.ValidationError{Message: "--emoji is required"}
			}
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			return mutate(cmd.Context(), f, "postCommentReaction",
				map[string]any{"file_key": key, "comment_id": args[1], "emoji": emoji},
				func(raw []byte) string {
					return fmt.Sprintf("Reacted %s on comment %s", emoji, args[1])
				})
		},
	}
	cmd.Flags().StringVar(&emoji, "emoji", "", "the emoji shortcode, e.g. :eyes:")
	return cmd
}

func newCommentUnreactCmd(f *cmdutil.Factory) *cobra.Command {
	var emoji string
	cmd := &cobra.Command{
		Use:     "unreact <file-key-or-url> <comment-id>",
		Short:   "Remove a reaction from a comment",
		Args:    cobra.ExactArgs(2),
		Example: `  figma comment unreact abc123 12345 --emoji :eyes:`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if emoji == "" {
				return &cmdutil.ValidationError{Message: "--emoji is required"}
			}
			key, err := fileKeyArg(args[0])
			if err != nil {
				return err
			}
			_, err = callAPI(cmd.Context(), f, "deleteCommentReaction",
				map[string]any{"file_key": key, "comment_id": args[1], "emoji": emoji})
			if err != nil {
				return err
			}
			if !f.Quiet {
				fmt.Fprintf(f.IOStreams.Out, "Removed reaction %s from comment %s\n", emoji, args[1])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&emoji, "emoji", "", "the emoji shortcode, e.g. :eyes:")
	return cmd
}

// truncate shortens s to at most n runes, adding an ellipsis when cut, so a
// long comment stays on one table row.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
