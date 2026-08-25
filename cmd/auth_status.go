package cmd

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/output"
)

// newAuthStatusCmd builds "figma auth status", showing the active profile,
// its token kind, and the identity from a live GET /v1/me. It errors with
// the auth exit code when the active profile is not logged in.
func newAuthStatusCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active profile's authentication status",
		Example: `  figma auth status
  figma auth status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(cmd.Context(), f)
		},
	}
}

// authStatus is the resolved status of the active profile.
type authStatus struct {
	Profile string `json:"profile"`
	Kind    string `json:"kind"`
	Handle  string `json:"handle,omitempty"`
	Email   string `json:"email,omitempty"`
}

func runAuthStatus(ctx context.Context, f *cmdutil.Factory) error {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return cmdutil.ErrNotLoggedIn
	}

	status := authStatus{Profile: profile, Kind: creds.Kind}
	// A live probe both proves the token still works and resolves the current
	// identity; a failure surfaces as the call's own typed error (e.g. exit 4
	// on 401), which is the honest status.
	resp, err := callAPI(ctx, f, "getMe", nil)
	if err != nil {
		return err
	}
	var id identity
	_ = json.Unmarshal(resp.Raw, &id)
	status.Handle = id.Handle
	status.Email = id.Email

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, status)
	}
	return writeKeyValues(f, [][2]string{
		{"Profile", status.Profile},
		{"Kind", status.Kind},
		{"Handle", status.Handle},
		{"Email", status.Email},
	})
}
