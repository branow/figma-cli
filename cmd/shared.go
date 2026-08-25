package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
	"github.com/branow/figma-cli/internal/figmaapi"
)

// wantJSON reports whether the resolved output format is JSON.
func wantJSON(f *cmdutil.Factory) bool {
	return f.OutputFormat() == "json"
}

// callAPI resolves the live client and invokes a catalogued operation. It is
// the one path every curated command funnels through, so credential
// resolution and routing behave identically to the generic api command.
func callAPI(ctx context.Context, f *cmdutil.Factory, operation string, fields map[string]any) (*figmaapi.Response, error) {
	client, err := f.ClientFn()
	if err != nil {
		return nil, err
	}
	return client.Call(ctx, operation, fields)
}

// runJSONCall invokes an operation and prints the raw response as JSON. It is
// the shared body of the curated commands whose payloads (a file document, a
// node subtree) are too large and nested to tabulate — they exist for the
// friendlier argument handling (URL parsing, node-id normalization), not a
// bespoke table.
func runJSONCall(ctx context.Context, f *cmdutil.Factory, operation string, fields map[string]any) error {
	resp, err := callAPI(ctx, f, operation, fields)
	if err != nil {
		return err
	}
	return writeResultJSON(f, resp.Raw)
}

// mutate runs a mutation and renders its outcome consistently across the
// command surface: the raw response as JSON under -o json, otherwise the
// one-line confirmation confirm builds from the response body. A 200 batch body
// that reports per-item errors (see figma.CheckBatchErrors) becomes a non-zero
// exit in either mode, so a silent success — exit 0 with a populated errors
// array — can no longer slip past a script. --quiet suppresses the confirmation
// line but not the error.
func mutate(ctx context.Context, f *cmdutil.Factory, operation string, fields map[string]any, confirm func(raw []byte) string) error {
	resp, err := callAPI(ctx, f, operation, fields)
	if err != nil {
		return err
	}
	batchErr := figma.CheckBatchErrors(resp.Raw)
	if wantJSON(f) {
		if err := writeResultJSON(f, resp.Raw); err != nil {
			return err
		}
		return batchErr
	}
	if batchErr != nil {
		return batchErr
	}
	if !f.Quiet {
		fmt.Fprintln(f.IOStreams.Out, confirm(resp.Raw))
	}
	return nil
}

// decodeID pulls a top-level string "id" from a JSON object body, for the
// confirmation lines that echo a created resource's id. It returns "" when the
// body carries no string id.
func decodeID(raw []byte) string {
	var env struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &env)
	return env.ID
}

// addOptional adds key=value to fields only when value is non-empty, so an
// unset flag never sends a blank query parameter.
func addOptional(fields map[string]any, key, value string) {
	if value != "" {
		fields[key] = value
	}
}

// mergeFields returns a new map holding base plus extra, so a paginated fetch
// can layer a page's cursor overrides on top of the unchanging base request
// without mutating it.
func mergeFields(base, extra map[string]any) map[string]any {
	fields := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		fields[k] = v
	}
	for k, v := range extra {
		fields[k] = v
	}
	return fields
}

// pageFetcher builds the FetchPage callback figma.Paginate drives: it merges a
// page's cursor overrides onto base, invokes operation, and returns the raw
// response body. The list/history commands share it so their page loops are
// identical.
func pageFetcher(ctx context.Context, f *cmdutil.Factory, operation string, base map[string]any) figma.FetchPage {
	return func(extra map[string]any) ([]byte, error) {
		resp, err := callAPI(ctx, f, operation, mergeFields(base, extra))
		if err != nil {
			return nil, err
		}
		return resp.Raw, nil
	}
}

// scalarString renders a decoded JSON scalar as display text without JSON
// quoting, for key/value tables built from a generic map.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// writeResultJSON is the canonical -o json path for every curated command:
// it pretty-prints the untouched server response, preserving the API's field
// order and losing nothing, the same way "figma api" does. A body that is
// not JSON (an empty 204, say) is emitted verbatim.
func writeResultJSON(f *cmdutil.Factory, raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		_, err := fmt.Fprintln(f.IOStreams.Out, string(trimmed))
		return err
	}
	buf.WriteByte('\n')
	_, err := f.IOStreams.Out.Write(buf.Bytes())
	return err
}

// writeKeyValues prints an aligned key/value block, the canonical table-mode
// rendering for a single-object result (a user, a file's metadata) where a
// columnar table reads oddly.
func writeKeyValues(f *cmdutil.Factory, pairs [][2]string) error {
	width := 0
	for _, p := range pairs {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}
	for _, p := range pairs {
		if _, err := fmt.Fprintf(f.IOStreams.Out, "%-*s  %s\n", width, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}
