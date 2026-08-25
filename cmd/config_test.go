package cmd_test

import (
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

func TestConfigSetGetRoundTrip(t *testing.T) {
	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "team_id", "1234567890"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "team_id"})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := out.String(); got != "1234567890\n" {
		t.Errorf("got %q, want %q", got, "1234567890\n")
	}
}

func TestConfigSetDoesNotSwitchActiveProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "--profile", "ci", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := f.Config.CurrentProfile(""); got != "default" {
		t.Errorf("config set switched the active profile to %q", got)
	}
	if got := f.Config.OutputFormat("", "ci"); got != "json" {
		t.Errorf("got output %q for profile ci, want json", got)
	}
}

func TestConfigSetRejectsInvalidOutputFormat(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for an unsupported output format")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "nonsense"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown config key")
	}
}

func TestConfigUnsetClearsValue(t *testing.T) {
	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "unset", "output"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if got := f.Config.ProfileOutput("default"); got != "" {
		t.Errorf("after unset, stored output = %q, want empty", got)
	}

	// get falls back to the built-in default once the value is cleared.
	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "output"})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "table" {
		t.Errorf("get after unset = %q, want the default \"table\"", got)
	}
}

func TestConfigSetEmptyValueClears(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", ""})
	if err := root.Execute(); err != nil {
		t.Fatalf("set empty should clear, not error: %v", err)
	}
	if got := f.Config.ProfileOutput("default"); got != "" {
		t.Errorf("after set empty, stored output = %q, want empty", got)
	}
}

func TestConfigList(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "default") {
		t.Errorf("expected output to mention profile default, got %q", out.String())
	}
}
