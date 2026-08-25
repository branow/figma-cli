package cmd_test

import (
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

// TestRequiredFlagMessageIsDeterministic pins that when several required flags
// are omitted, the reported one is always the first in declaration order, not a
// map's randomized pick. Runs repeatedly so a randomized order would flake.
func TestRequiredFlagMessageIsDeterministic(t *testing.T) {
	for i := 0; i < 20; i++ {
		f, _, _ := newTestFactory(t, "")
		root := cmd.NewRootCmd(f)
		root.SetArgs([]string{"webhook", "create"})
		err := root.Execute()
		if err == nil {
			t.Fatal("expected a validation error when required flags are missing")
		}
		if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
			t.Fatalf("got exit code %d, want %d", got, cmdutil.ExitValidation)
		}
		if !strings.Contains(err.Error(), "--event-type is required") {
			t.Fatalf("got %q, want the first-in-order flag reported deterministically", err.Error())
		}
	}
}

// TestEnumFlagsValidatedOffline pins that the enum-ish and numeric flags are
// rejected client-side (exit 3) with no network call, per the QA review — a
// bad value never round-trips to the API. newTestFactory("") has no client, so
// any request would fail differently.
func TestEnumFlagsValidatedOffline(t *testing.T) {
	cases := map[string][]string{
		"image bad format":         {"image", "render", "abc123", "--ids", "1-2", "--format", "webp"},
		"image out-of-range scale": {"image", "render", "abc123", "--ids", "1-2", "--scale", "9"},
		"file zero depth":          {"file", "get", "abc123", "--depth", "0"},
		"file bad geometry":        {"file", "get", "abc123", "--geometry", "curves"},
		"webhook bad status":       {"webhook", "update", "42", "--status", "OFF"},
		"analytics bad group-by":   {"analytics", "component", "actions", "abc123", "--group-by", "banana"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			f, _, _ := newTestFactory(t, "")
			root := cmd.NewRootCmd(f)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("expected a validation error for %v", args)
			}
			if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
				t.Errorf("got exit code %d, want %d for %v", got, cmdutil.ExitValidation, args)
			}
		})
	}
}

// TestAnalyticsGroupByAcceptsValidDimension confirms a per-report-valid
// dimension passes the client-side check (component actions accepts "team").
func TestAnalyticsGroupByAcceptsValidDimension(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	// No client is wired, so getting past validation surfaces as "not logged
	// in" (exit 4), proving the group-by check itself did not reject "team".
	root.SetArgs([]string{"analytics", "component", "actions", "abc123", "--group-by", "team"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error (no credentials) after validation passes")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d (validation should have passed)", got, cmdutil.ExitAuth)
	}
}
