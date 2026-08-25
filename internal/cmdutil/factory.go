// Package cmdutil bundles the dependencies every command needs — IO
// streams, config, a client constructor, and a credentials store — into
// one Factory so commands take it as a constructor argument instead of
// reaching for globals, keeping them unit-testable. It also owns the CLI's
// typed error values and the mapping from errors to process exit codes.
package cmdutil

import (
	"github.com/branow/figma-cli/internal/config"
	"github.com/branow/figma-cli/internal/credentials"
	"github.com/branow/figma-cli/internal/figmaapi"
	"github.com/branow/figma-cli/internal/iostreams"
)

// Factory bundles the dependencies commands need, plus the resolved values
// of figma's global persistent flags.
type Factory struct {
	IOStreams        *iostreams.IOStreams
	Config           *config.Config
	CredentialsStore credentials.Store
	ClientFn         func() (figmaapi.Client, error)

	Profile string
	Output  string
	NoInput bool
	Quiet   bool
	Force   bool
}

// ActiveProfile resolves which profile this invocation targets, applying
// the --profile flag over env/config/default precedence.
func (f *Factory) ActiveProfile() string {
	return f.Config.CurrentProfile(f.Profile)
}

// OutputFormat resolves which output format this invocation should use,
// applying the -o/--output flag over env/config/default precedence.
func (f *Factory) OutputFormat() string {
	return f.Config.OutputFormat(f.Output, f.ActiveProfile())
}
