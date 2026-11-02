package cmd_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/gokey"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/credentials"
)

func TestAuthLoginNonInteractiveThenStatusThenLogout(t *testing.T) {
	// The login probe and the status probe both hit this server; point the
	// live-client base URL at it and allow the loopback http override.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id": "1", "handle": "Ada", "email": "ada@example.com"}`))
	}))
	defer server.Close()
	t.Setenv("FIGMA_BASE_URL", server.URL)
	t.Setenv("FIGMA_INSECURE", "1")

	f, out, _ := newTestFactory(t, server.URL)

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "login", "--token-stdin"})
	f.IOStreams.In = strings.NewReader("figd_s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out.String(), "Ada") {
		t.Errorf("expected login confirmation to mention the handle, got %q", out.String())
	}
	creds, err := f.CredentialsStore.Get("default")
	if err != nil || creds.Token != "figd_s3cret" || creds.Kind != "pat" {
		t.Errorf("stored creds = %+v, err %v; want pat/figd_s3cret", creds, err)
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Ada") {
		t.Errorf("expected status to show the handle, got %q", out.String())
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	err = root.Execute()
	if err == nil {
		t.Fatal("expected an error after logout")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAuthLoginVerificationRejectsBadTokenWithoutStoringIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"status": 403, "err": "Invalid token"}`))
	}))
	defer server.Close()
	t.Setenv("FIGMA_BASE_URL", server.URL)
	t.Setenv("FIGMA_INSECURE", "1")

	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "login", "--token-stdin"})
	f.IOStreams.In = strings.NewReader("wrong\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected login to fail against a 403 server")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
	if _, err := f.CredentialsStore.Get("default"); err != credentials.ErrNotFound {
		t.Errorf("rejected token must not be stored, got %v", err)
	}
}

func TestAuthLoginInconclusiveVerificationStoresTokenWithWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status": 400, "err": "unexpected"}`))
	}))
	defer server.Close()
	t.Setenv("FIGMA_BASE_URL", server.URL)
	t.Setenv("FIGMA_INSECURE", "1")

	f, _, errOut := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "login", "--token-stdin"})
	f.IOStreams.In = strings.NewReader("figd_s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login must succeed on an inconclusive probe: %v", err)
	}
	if !strings.Contains(errOut.String(), "could not verify") {
		t.Errorf("expected a could-not-verify warning, got %q", errOut.String())
	}
	if _, err := f.CredentialsStore.Get("default"); err != nil {
		t.Errorf("token must be stored on an inconclusive probe, got %v", err)
	}
}

func TestAuthLoginNoVerifySkipsTheAPICall(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "login", "--token-stdin", "--no-verify"})
	f.IOStreams.In = strings.NewReader("figd_s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login --no-verify: %v", err)
	}
	if !strings.Contains(out.String(), "default") {
		t.Errorf("expected login confirmation to mention the profile, got %q", out.String())
	}
	if _, err := f.CredentialsStore.Get("default"); err != nil {
		t.Errorf("token must be stored with --no-verify, got %v", err)
	}
}

func TestAuthLoginRequiresTokenNonInteractively(t *testing.T) {
	t.Setenv("FIGMA_TOKEN", "") // ensure no ambient token satisfies the prompt
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "login", "--no-verify"})
	// No stdin token, no FIGMA_TOKEN, not a TTY: must fail with validation.
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error when no token is available")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAuthSwitchRequiresExistingProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "switch", "--profile", "ci"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error switching to an unconfigured profile")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

// blockedStore holds an entry the keychain will not release, as an entry
// stored by another build of the CLI looks to an unattended run.
type blockedStore struct{ *credentials.FakeStore }

var errBlocked = fmt.Errorf("cannot read the keychain entry: %w; approve the keychain prompt on an interactive terminal", gokey.ErrBlocked)

func (blockedStore) Get(string) (credentials.Credentials, error) {
	return credentials.Credentials{}, errBlocked
}

// A blocked entry is not a missing one: "not logged in" would send the
// user to log in again instead of approving the dialog.
func TestAuthStatusReportsBlockedKeychainEntryWithItsRemedy(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	f.CredentialsStore = blockedStore{credentials.NewFakeStore()}

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	err := root.Execute()
	if !errors.Is(err, gokey.ErrBlocked) {
		t.Fatalf("error = %v, want the blocked keychain error", err)
	}
	if errors.Is(err, cmdutil.ErrNotLoggedIn) {
		t.Errorf("error = %v, reported as not logged in", err)
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}
