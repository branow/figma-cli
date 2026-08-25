package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/output"
)

const (
	keyOutput = "output"
	keyTeam   = "team_id"
)

// newConfigCmd groups the config get/set/list subcommands for reading and
// writing figma's non-secret preferences.
func newConfigCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage figma configuration",
	}
	cmd.AddCommand(newConfigGetCmd(f), newConfigSetCmd(f), newConfigUnsetCmd(f), newConfigListCmd(f))
	return cmd
}

func newConfigGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a configuration value for the active profile",
		Args:  cobra.ExactArgs(1),
		Example: `  figma config get output
  figma config get --profile ci team_id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(f, args[0])
		},
	}
}

func newConfigSetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value for the active profile",
		Args:  cobra.ExactArgs(2),
		Example: `  figma config set output json
  figma config set team_id 1234567890`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(f, args[0], args[1])
		},
	}
}

func newConfigUnsetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Clear a configuration value for the active profile",
		Args:  cobra.ExactArgs(1),
		Example: `  figma config unset output
  figma config unset team_id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigUnset(f, args[0])
		},
	}
}

func newConfigListCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		Example: `  figma config list
  figma config list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(f)
		},
	}
}

func runConfigGet(f *cmdutil.Factory, key string) error {
	value, err := configValue(f, f.ActiveProfile(), key)
	if err != nil {
		return err
	}
	fmt.Fprintln(f.IOStreams.Out, value)
	return nil
}

func configValue(f *cmdutil.Factory, profile, key string) (string, error) {
	switch key {
	case keyOutput:
		return f.Config.OutputFormat("", profile), nil
	case keyTeam:
		return f.Config.TeamID("", profile), nil
	default:
		return "", unknownConfigKeyError(key)
	}
}

// validOutputFormat reports whether value names a supported -o/--output
// format.
func validOutputFormat(value string) bool {
	return value == "table" || value == "json"
}

func runConfigSet(f *cmdutil.Factory, key, value string) error {
	profile := f.ActiveProfile()
	p := f.Config.Profiles[profile]
	switch key {
	case keyOutput:
		// An empty value clears the preference (reverting to the built-in
		// default), the same as "config unset output"; a non-empty value must
		// name a supported format.
		if value != "" && !validOutputFormat(value) {
			return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid output format %q (expected table or json)", value)}
		}
		p.Output = value
	case keyTeam:
		p.TeamID = value
	default:
		return unknownConfigKeyError(key)
	}
	f.Config.SetProfile(profile, p)
	return f.Config.Save()
}

// runConfigUnset clears a configuration value for the active profile, reverting
// it to the built-in default without hand-editing config.yml.
func runConfigUnset(f *cmdutil.Factory, key string) error {
	profile := f.ActiveProfile()
	p := f.Config.Profiles[profile]
	switch key {
	case keyOutput:
		p.Output = ""
	case keyTeam:
		p.TeamID = ""
	default:
		return unknownConfigKeyError(key)
	}
	f.Config.SetProfile(profile, p)
	return f.Config.Save()
}

func runConfigList(f *cmdutil.Factory) error {
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, f.Config.Profiles)
	}

	current := f.ActiveProfile()
	names := make([]string, 0, len(f.Config.Profiles))
	for name := range f.Config.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		p := f.Config.Profiles[name]
		marker := ""
		if name == current {
			marker = "*"
		}
		rows = append(rows, []string{marker, name, p.Handle, p.Output, p.TeamID})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"", "PROFILE", "HANDLE", "OUTPUT", "TEAM"}, rows)
}

func unknownConfigKeyError(key string) error {
	return &cmdutil.ValidationError{Message: fmt.Sprintf("unknown config key %q (expected one of: output, team_id)", key)}
}
