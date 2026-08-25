// Command figma is the entry point for figma-cli: one scriptable CLI for
// the Figma REST API, with curated commands plus a generic "figma api"
// escape hatch backed by a catalog distilled from Figma's OpenAPI spec.
package main

import (
	"fmt"
	"os"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, cmdutil.Explain(err))
	}
	os.Exit(cmdutil.ExitCode(err))
}
