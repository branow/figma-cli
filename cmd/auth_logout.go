package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/credentials"
)

// newAuthLogoutCmd builds "figma auth logout", removing stored credentials
// for a profile. --purge also drops the profile's non-secret config entry.
func newAuthLogoutCmd(f *cmdutil.Factory) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for a profile",
		Example: `  figma auth logout
  figma auth logout --profile ci --purge`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout(f, purge)
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove the profile's non-secret config entry")
	return cmd
}

func runAuthLogout(f *cmdutil.Factory, purge bool) error {
	profile := f.ActiveProfile()
	if err := f.CredentialsStore.Delete(profile); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return err
	}
	if purge {
		delete(f.Config.Profiles, profile)
		if err := f.Config.Save(); err != nil {
			return err
		}
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Logged out of profile %q\n", profile)
	}
	return nil
}
