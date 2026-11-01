package cmdutil

import (
	"errors"

	"github.com/branow/gokey"

	"github.com/branow/figma-cli/internal/figmaapi"
)

// Exit codes, per DESIGN.md's exit-code table.
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitCancelled   = 2
	ExitValidation  = 3
	ExitAuth        = 4
	ExitNotFound    = 5
	ExitRateLimited = 6
)

// ExitCode maps a command error to the documented process exit code,
// giving main.go one place to translate errors into process behavior.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ErrCancelled) {
		return ExitCancelled
	}
	// A keychain entry this binary may not read yet is an auth failure too:
	// the stored login exists, and a person has to approve its use.
	if errors.Is(err, ErrNotLoggedIn) || errors.Is(err, gokey.ErrBlocked) {
		return ExitAuth
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return ExitValidation
	}
	var apiErr *figmaapi.APIError
	if errors.As(err, &apiErr) {
		return exitCodeForAPI(apiErr)
	}
	var unknownOp *figmaapi.UnknownOperationError
	var unsupportedOp *figmaapi.UnsupportedOperationError
	if errors.As(err, &unknownOp) || errors.As(err, &unsupportedOp) {
		return ExitValidation
	}
	return ExitError
}

// exitCodeForAPI classifies a non-2xx REST response. Auth and not-found are
// read from the status; ExitRateLimited is reserved for genuine throttling
// (a 429, or a 5xx carrying a Retry-After — the APIError.RateLimited flag),
// so a permanent 5xx does not make a script branching on code 6 retry it
// forever.
func exitCodeForAPI(e *figmaapi.APIError) int {
	switch {
	case e.Status == 401 || e.Status == 403:
		return ExitAuth
	case e.Status == 404:
		return ExitNotFound
	case e.Status == 429 || e.RateLimited:
		return ExitRateLimited
	}
	return ExitError
}
