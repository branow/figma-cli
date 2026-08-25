package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaapi"
	"github.com/branow/figma-cli/internal/figmaapi/catalog"
	"github.com/branow/figma-cli/internal/output"
)

// newAPICmd builds the "api" command: a generic invoker for any catalogued
// Figma operation, without a bespoke subcommand per operation.
func newAPICmd(f *cmdutil.Factory) *cobra.Command {
	var fields []string
	var inputFile string
	var listOperations bool
	var describe bool

	cmd := &cobra.Command{
		Use:   "api <operation>",
		Short: "Invoke a named Figma REST API operation",
		Long: `Invoke a named Figma REST API operation against the active profile
(e.g. getMe, getFile, postComment). The operationId routes to its REST
endpoint via the embedded catalog: path parameters are filled from matching
fields, and the remaining fields become query parameters for GET/DELETE
operations or a JSON request body otherwise. --list prints every catalogued
operationId.

-f/--field values parse as JSON when they look like it (numbers, booleans,
null, arrays, objects) and as plain strings otherwise. --input reads a full
JSON object from a file (or "-" for stdin) — use it for nested request
bodies — and cannot be combined with -f/--field.

--describe fully documents an operation: its HTTP method, path, and
path/query/body/response fields — each with its type and, for request
fields, whether it is required, with nested component types expanded inline
(no credentials or network needed). Add -o json for a machine-readable shape.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  figma api getMe
  figma api getFile -f file_key=abc123
  figma api postComment -f file_key=abc123 -f message="Looks great"
  figma api postComment --input comment.json
  figma api getFile --describe
  figma api --list`,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return catalog.Operations(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if listOperations {
				if len(args) != 0 {
					return &cmdutil.ValidationError{Message: "--list cannot be combined with an operation name"}
				}
				return runAPIList(f)
			}
			if len(args) != 1 {
				return &cmdutil.ValidationError{Message: "an operation name is required (run api --list to see all operations)"}
			}
			if describe {
				if len(fields) > 0 || inputFile != "" {
					return &cmdutil.ValidationError{Message: "--describe cannot be combined with --field or --input"}
				}
				return runAPIDescribe(f, args[0])
			}
			return runAPI(cmd.Context(), f, args[0], fields, inputFile)
		},
	}

	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "request field as key=value (repeatable)")
	cmd.Flags().StringVar(&inputFile, "input", "", `read request fields as a JSON object from a file ("-" for stdin)`)
	cmd.Flags().BoolVar(&listOperations, "list", false, "list all catalogued operationIds")
	cmd.Flags().BoolVar(&describe, "describe", false, "print the operation's method, path, and documented fields")
	return cmd
}

// resolveOperation looks an operationId up in the catalog, mapping an
// unknown id to an UnknownOperationError (a typo, exit code validation)
// caught before any network call.
func resolveOperation(id string) (catalog.Operation, error) {
	op, ok := catalog.Lookup(id)
	if !ok {
		return catalog.Operation{}, &figmaapi.UnknownOperationError{Operation: id}
	}
	return op, nil
}

// runAPIList prints every catalogued operationId, one per line (or as a JSON
// array with -o json), needing no credentials or network access.
func runAPIList(f *cmdutil.Factory) error {
	ids := catalog.Operations()
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, ids)
	}
	for _, id := range ids {
		fmt.Fprintln(f.IOStreams.Out, id)
	}
	return nil
}

func runAPI(ctx context.Context, f *cmdutil.Factory, operation string, fields []string, inputFile string) error {
	if len(fields) > 0 && inputFile != "" {
		return &cmdutil.ValidationError{Message: "--field and --input cannot be combined"}
	}
	// Resolve the operation before touching credentials, so a typo is
	// reported as such even when the user is not logged in.
	op, err := resolveOperation(operation)
	if err != nil {
		return err
	}

	requestFields, err := buildRequestFields(fields, inputFile, f.IOStreams.In)
	if err != nil {
		return err
	}

	method, path, query, body, err := figmaapi.Route(op, requestFields)
	if err != nil {
		return &cmdutil.ValidationError{Message: err.Error()}
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	resp, callErr := client.Do(ctx, method, path, query, body)
	if resp != nil {
		payload := resp.Body
		if payload == nil {
			payload = resp.Fields
		}
		if err := output.WriteJSON(f.IOStreams.Out, payload); err != nil {
			return err
		}
	}
	if callErr != nil {
		return callErr
	}
	// Figma's batch endpoints (postVariables, postDevResources, …) return
	// HTTP 200 with a populated top-level "errors" array on a partial or total
	// failure. The curated commands detect this via mutate/CheckBatchErrors;
	// the generic path must too, so a batch op invoked through "api" cannot
	// report success (exit 0) while nothing was applied. A normal response
	// carries no such array and is unaffected.
	if resp != nil {
		return figma.CheckBatchErrors(resp.Raw)
	}
	return nil
}

// buildRequestFields assembles the operation's request fields: -f/--field
// values as key=value pairs, or --input's JSON object.
func buildRequestFields(fields []string, inputFile string, stdin io.Reader) (map[string]any, error) {
	if inputFile != "" {
		return readInputFields(inputFile, stdin)
	}
	return parseFields(fields)
}

func parseFields(fields []string) (map[string]any, error) {
	pairs := make(map[string]any, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --field %q, expected key=value", field)}
		}
		pairs[key] = typedValue(value)
	}
	return pairs, nil
}

// typedValue converts a --field value to the JSON type it spells: numbers,
// booleans, null, arrays, and objects parse as themselves so depth=2 is sent
// as a JSON number; anything else stays a string. Numbers decode as
// json.Number rather than float64 so a large id round-trips exactly.
func typedValue(value string) any {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.UseNumber()
	var typed any
	if err := dec.Decode(&typed); err == nil && !dec.More() {
		return typed
	}
	return value
}

func readInputFields(inputFile string, stdin io.Reader) (map[string]any, error) {
	data, err := readInput(inputFile, stdin)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("--input must be a JSON object: %v", err)}
	}
	return fields, nil
}

func readInput(inputFile string, stdin io.Reader) ([]byte, error) {
	if inputFile == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(inputFile)
}
