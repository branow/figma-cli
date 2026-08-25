package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/config"
	"github.com/branow/figma-cli/internal/credentials"
	"github.com/branow/figma-cli/internal/figmaapi"
)

// newAuthLoginCmd builds "figma auth login", supporting both interactive
// prompts and fully non-interactive, scriptable operation via flags so it is
// not a dead end for CI use. It stores a personal access token in the OS
// keychain and the resolved identity (handle/email) in the config profile.
func newAuthLoginCmd(f *cmdutil.Factory) *cobra.Command {
	var tokenStdin, noVerify bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with a Figma token and store it securely",
		Long: `Log in to the Figma REST API. Authentication uses a personal access
token sent as the X-Figma-Token header (create one at Figma → Settings →
Security → Personal access tokens). The token is stored in the OS keychain;
your resolved handle and email are stored in the config profile.

The token is read from --token-stdin, then the FIGMA_TOKEN environment
variable, otherwise it is prompted for with hidden input on a terminal.`,
		Example: `  # Interactive login, prompting for the token
  figma auth login

  # Non-interactive login for CI, reading the token from stdin
  echo "$FIGMA_TOKEN" | figma auth login --token-stdin

  # Log in under a named profile
  figma auth login --profile ci --token-stdin < token.txt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context(), f, tokenStdin, noVerify)
		},
	}

	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the token from stdin")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "store the token without verifying it against the API")
	return cmd
}

func runAuthLogin(ctx context.Context, f *cmdutil.Factory, tokenStdin, noVerify bool) error {
	profile := f.ActiveProfile()
	reader := bufio.NewReader(f.IOStreams.In)

	token, err := resolveToken(f, reader, tokenStdin)
	if err != nil {
		return err
	}

	creds := credentials.Credentials{Kind: "pat", Token: token}
	handle, email := "", ""
	if !noVerify {
		identity, err := verifyLogin(ctx, f, creds)
		if err != nil {
			return err
		}
		handle, email = identity.Handle, identity.Email
	}
	if err := f.CredentialsStore.Set(profile, creds); err != nil {
		return err
	}

	// Preserve any output preference already stored for this profile, but do
	// not bake a resolved default or a transient FIGMA_OUTPUT into it — an
	// unset Output stays unset so the env/default keep applying at read time.
	f.Config.SetProfile(profile, config.Profile{
		Output: f.Config.ProfileOutput(profile),
		TeamID: f.Config.Profiles[profile].TeamID,
		Handle: handle,
		Email:  email,
	})
	if err := f.Config.SwitchProfile(profile); err != nil {
		return err
	}
	if err := f.Config.Save(); err != nil {
		return err
	}

	if !f.Quiet {
		who := handle
		if who == "" {
			who = email
		}
		if who == "" {
			fmt.Fprintf(f.IOStreams.Out, "Logged in using profile %q\n", profile)
		} else {
			fmt.Fprintf(f.IOStreams.Out, "Logged in as %s using profile %q\n", who, profile)
		}
	}
	return nil
}

// resolveToken reads the token from stdin when tokenStdin is set, then from
// FIGMA_TOKEN, then prompts with hidden input on a real terminal, otherwise
// fails with a validation error. Only the trailing line ending is stripped.
func resolveToken(f *cmdutil.Factory, reader *bufio.Reader, tokenStdin bool) (string, error) {
	if tokenStdin {
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return nonEmptyToken(string(data))
	}
	if v := os.Getenv("FIGMA_TOKEN"); v != "" {
		return nonEmptyToken(v)
	}
	if stdin := f.IOStreams.StdinFd(); stdin != nil && f.IOStreams.CanPrompt() {
		fmt.Fprint(f.IOStreams.Out, "Personal access token: ")
		byteToken, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(f.IOStreams.Out)
		if err != nil {
			return "", err
		}
		return nonEmptyToken(string(byteToken))
	}
	return "", &cmdutil.ValidationError{Message: "a token is required (use --token-stdin or set FIGMA_TOKEN in non-interactive mode)"}
}

func nonEmptyToken(raw string) (string, error) {
	token := strings.TrimRight(raw, "\r\n")
	if token == "" {
		return "", &cmdutil.ValidationError{Message: "a token is required"}
	}
	return token, nil
}

// identity is the subset of the getMe response the CLI shows after login.
type identity struct {
	Handle string `json:"handle"`
	Email  string `json:"email"`
}

// verifyLogin checks the token against GET /v1/me before it is stored, so a
// wrong token fails at login rather than on the first real call, and returns
// the identity it resolves. Only definitive rejections block the login —
// 401/403 (bad token). Anything else the probe cannot interpret (an
// unreachable host, a 5xx) is inconclusive: the token is stored with a
// warning, since blocking on it would lock out otherwise-valid logins.
func verifyLogin(ctx context.Context, f *cmdutil.Factory, creds credentials.Credentials) (identity, error) {
	baseURL, err := figmaapi.BaseURL()
	if err != nil {
		return identity{}, err
	}
	client := figmaapi.New(baseURL, creds)
	resp, err := client.Do(ctx, http.MethodGet, "/v1/me", nil, nil)
	if err == nil {
		var id identity
		_ = json.Unmarshal(resp.Raw, &id)
		return id, nil
	}
	var apiErr *figmaapi.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return identity{}, fmt.Errorf("login verification failed (use --no-verify to store the token anyway): %w", err)
		}
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "warning: could not verify the token (%v); storing it anyway\n", err)
	return identity{}, nil
}
