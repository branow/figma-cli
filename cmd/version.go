package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
)

// version is the figma release version, injected at build time via
// -ldflags "-X github.com/branow/figma-cli/cmd.version=...". It defaults to
// "dev" for `go install` and local builds.
var version = "dev"

// newVersionCmd builds "figma version".
func newVersionCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the figma version",
		Example: `  figma version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(f.IOStreams.Out, "figma version %s\n", version)
			return nil
		},
	}
}
