package cmdutil_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figmaapi"
)

func TestExplainSurfacesMissingScope(t *testing.T) {
	err := &figmaapi.APIError{
		Status:  403,
		Message: "Invalid scope(s): current_user:read, file_comments:read, file_content:read. This endpoint requires the projects:read scope",
	}
	got := cmdutil.Explain(err)
	if !strings.Contains(got, `"projects:read"`) {
		t.Errorf("Explain = %q, want it to name the missing projects:read scope", got)
	}
	if strings.Contains(got, "current_user:read") {
		t.Errorf("Explain = %q, should drop the granted-scope noise", got)
	}
}

func TestExplainPassesThroughOtherErrors(t *testing.T) {
	err := &figmaapi.APIError{Status: 401, Message: "Invalid token"}
	if got := cmdutil.Explain(err); got != err.Error() {
		t.Errorf("Explain = %q, want the raw message for a non-scope error", got)
	}
	plain := errors.New("boom")
	if got := cmdutil.Explain(plain); got != "boom" {
		t.Errorf("Explain(plain) = %q, want boom", got)
	}
}
