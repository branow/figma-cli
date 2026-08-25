// Package cmd wires figma's Cobra command tree to a cmdutil.Factory;
// commands hold no business logic beyond flag parsing and calling into
// internal packages.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/config"
	"github.com/branow/figma-cli/internal/credentials"
	"github.com/branow/figma-cli/internal/figmaapi"
	"github.com/branow/figma-cli/internal/iostreams"
)

// Execute builds the real Factory and runs the CLI, returning any error for
// main.go to translate into a process exit code.
func Execute() error {
	f, err := newRealFactory()
	if err != nil {
		return err
	}
	return NewRootCmd(f).Execute()
}

// NewRootCmd builds the root "figma" command and its full subcommand tree
// around f.
func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	var noColor bool

	root := &cobra.Command{
		Use:           "figma",
		Short:         "Command-line interface for the Figma REST API",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: `  # Authenticate, then confirm who you are
  figma auth login
  figma me

  # Work from a pasted Figma URL or a bare file key
  figma file get "https://www.figma.com/design/abc123/My-Design"
  figma image render abc123 --ids 1-2,3-4 --out ./assets
  figma comment list abc123

  # Reach any of the 54 API operations generically
  figma api getFile --describe
  figma api getFile -f file_key=abc123`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if f.Output != "" && !validOutputFormat(f.Output) {
				return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --output %q (expected table or json)", f.Output)}
			}
			f.IOStreams.SetNoColor(noColor)
			f.IOStreams.SetNoInput(f.NoInput)
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&f.Output, "output", "o", "", "output format: table|json")
	root.PersistentFlags().StringVarP(&f.Profile, "profile", "p", "", "profile to use")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	root.PersistentFlags().BoolVar(&f.NoInput, "no-input", false, "disable interactive prompts")
	root.PersistentFlags().BoolVarP(&f.Quiet, "quiet", "q", false, "suppress non-essential output")
	// No -f shorthand here: "api" already uses -f for --field, and pflag
	// panics on a shorthand collision when persistent flags are merged in.
	root.PersistentFlags().BoolVar(&f.Force, "force", false, "skip confirmation prompts")

	root.AddCommand(
		newAuthCmd(f),
		newConfigCmd(f),
		newAPICmd(f),
		newMeCmd(f),
		newFileCmd(f),
		newImageCmd(f),
		newCommentCmd(f),
		newProjectCmd(f),
		newFolderCmd(f),
		newComponentCmd(f),
		newComponentSetCmd(f),
		newStyleCmd(f),
		newWebhookCmd(f),
		newVariableCmd(f),
		newDevResourceCmd(f),
		newAnalyticsCmd(f),
		newCompletionCmd(),
		newVersionCmd(f),
	)
	wireValidationErrors(root)
	return root
}

// wireValidationErrors makes Cobra's own usage errors — an unknown flag, a bad
// flag value, an unknown subcommand, an invalid enum argument — surface as a
// cmdutil.ValidationError so they map to exit code 3 like the rest of the
// validation surface, instead of leaking Cobra's default exit 1. It sets the
// flag-error hook once at the root (Cobra inherits it down the tree) and wraps
// each command's argument validation, adding the "unknown command" check for
// group commands that have no run of their own.
func wireValidationErrors(root *cobra.Command) {
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &cmdutil.ValidationError{Message: err.Error()}
	})
	wrapArgsValidation(root)
}

func wrapArgsValidation(cmd *cobra.Command) {
	orig := cmd.Args
	isGroup := cmd.HasSubCommands() && cmd.Run == nil && cmd.RunE == nil
	cmd.Args = func(c *cobra.Command, args []string) error {
		if orig != nil {
			if err := orig(c, args); err != nil {
				return &cmdutil.ValidationError{Message: err.Error()}
			}
			return nil
		}
		if isGroup && len(args) > 0 {
			return &cmdutil.ValidationError{Message: fmt.Sprintf("unknown command %q for %q", args[0], c.CommandPath())}
		}
		return nil
	}
	// A group command (subcommands, no run of its own) is not "runnable", so
	// Cobra shows its help and returns before argument validation runs — which
	// would let "figma file bogus" exit 0. Giving it a RunE that just prints
	// help makes it runnable so the Args check above fires on a stray argument.
	if isGroup {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return c.Help()
		}
	}
	for _, sub := range cmd.Commands() {
		wrapArgsValidation(sub)
	}
}

func newRealFactory() (*cmdutil.Factory, error) {
	streams := iostreams.System()
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	credPath, err := credentials.DefaultPlaintextPath()
	if err != nil {
		return nil, err
	}

	f := &cmdutil.Factory{
		IOStreams:        streams,
		Config:           cfg,
		CredentialsStore: credentials.NewKeyringStore(streams, credPath),
	}
	f.ClientFn = func() (figmaapi.Client, error) {
		return newClient(f)
	}
	return f, nil
}

// newClient constructs the real API client from the active profile's stored
// credentials against the fixed Figma host.
func newClient(f *cmdutil.Factory) (figmaapi.Client, error) {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return nil, cmdutil.ErrNotLoggedIn
	}
	baseURL, err := figmaapi.BaseURL()
	if err != nil {
		return nil, err
	}
	return figmaapi.New(baseURL, creds), nil
}
