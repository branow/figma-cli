package cmd

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
)

// newMeCmd builds "figma me", the curated shortcut for GET /v1/me showing
// the authenticated user.
func newMeCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the authenticated user",
		Example: `  figma me
  figma me -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMe(cmd.Context(), f)
		},
	}
}

// meUser is the getMe response shape shown in table mode.
type meUser struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Email  string `json:"email"`
	ImgURL string `json:"img_url"`
}

func runMe(ctx context.Context, f *cmdutil.Factory) error {
	resp, err := callAPI(ctx, f, "getMe", nil)
	if err != nil {
		return err
	}
	if wantJSON(f) {
		return writeResultJSON(f, resp.Raw)
	}
	var u meUser
	if err := json.Unmarshal(resp.Raw, &u); err != nil {
		return err
	}
	return writeKeyValues(f, [][2]string{
		{"ID", u.ID},
		{"Handle", u.Handle},
		{"Email", u.Email},
		{"Image", u.ImgURL},
	})
}
