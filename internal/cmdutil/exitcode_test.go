package cmdutil_test

import (
	"fmt"
	"testing"

	"github.com/branow/gokey"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figmaapi"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, cmdutil.ExitSuccess},
		{"generic", fmt.Errorf("boom"), cmdutil.ExitError},
		{"cancelled", cmdutil.ErrCancelled, cmdutil.ExitCancelled},
		{"not logged in", cmdutil.ErrNotLoggedIn, cmdutil.ExitAuth},
		{"keychain entry blocked", fmt.Errorf("cannot read the keychain entry: %w", gokey.ErrBlocked), cmdutil.ExitAuth},
		{"validation", &cmdutil.ValidationError{Message: "bad flag"}, cmdutil.ExitValidation},
		{"api 401", &figmaapi.APIError{Status: 401, Message: "unauthorized"}, cmdutil.ExitAuth},
		{"api 403", &figmaapi.APIError{Status: 403, Message: "forbidden"}, cmdutil.ExitAuth},
		{"api 404", &figmaapi.APIError{Status: 404, Message: "not found"}, cmdutil.ExitNotFound},
		{"api 429", &figmaapi.APIError{Status: 429, Message: "too many requests"}, cmdutil.ExitRateLimited},
		{"api 503 rate-limited", &figmaapi.APIError{Status: 503, RateLimited: true, Message: "unavailable"}, cmdutil.ExitRateLimited},
		{"api 503 permanent", &figmaapi.APIError{Status: 503, Message: "gone"}, cmdutil.ExitError},
		{"api 500", &figmaapi.APIError{Status: 500, Message: "server error"}, cmdutil.ExitError},
		{"api 400", &figmaapi.APIError{Status: 400, Message: "bad request"}, cmdutil.ExitError},
		{"unknown operation", &figmaapi.UnknownOperationError{Operation: "getFilez"}, cmdutil.ExitValidation},
		{"unsupported operation", &figmaapi.UnsupportedOperationError{Operation: "doThing"}, cmdutil.ExitValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cmdutil.ExitCode(c.err); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
