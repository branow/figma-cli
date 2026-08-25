package cmdutil

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/branow/figma-cli/internal/figmaapi"
)

// scopePattern pulls the missing scope out of Figma's 403 body, whose text
// reads like "Invalid scope(s): current_user:read, file_comments:read, ...
// This endpoint requires the projects:read scope". The leading list of the
// token's granted scopes is noise; the actionable part is the scope named
// after "requires the".
var scopePattern = regexp.MustCompile(`requires the ([a-z_:]+) scope`)

// Explain returns the user-facing message for err. For most errors that is
// simply err.Error(); for Figma's 403 scope dump it replaces the raw
// granted-scope list with a short, actionable hint that names the missing
// scope and where to add it, since that is the next step a PAT user needs.
// Exit-code classification is untouched — main.go still calls ExitCode on the
// original err — so this only changes what is printed, not how the process
// exits.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *figmaapi.APIError
	if errors.As(err, &apiErr) {
		if m := scopePattern.FindStringSubmatch(apiErr.Message); m != nil {
			return fmt.Sprintf("your token is missing the %q scope — regenerate the personal access token at Figma > Settings > Security with that scope enabled", m[1])
		}
	}
	return err.Error()
}
