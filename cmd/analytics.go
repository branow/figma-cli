package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/branow/figma-cli/internal/cmdutil"
	"github.com/branow/figma-cli/internal/figma"
)

// newAnalyticsCmd builds "figma analytics", a thin front for the six library
// analytics endpoints (component/style/variable × actions/usages), which
// share a shape: a library file key plus a required group-by and optional
// date range. The operationId is assembled from the asset and metric.
func newAnalyticsCmd(f *cmdutil.Factory) *cobra.Command {
	var groupBy, startDate, endDate, cursor string
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "analytics <component|style|variable> <actions|usages> <file-key-or-url>",
		Short: "Get library analytics for a published file",
		Long: `Get library analytics for a published library file. The first two
arguments select the report: the asset type (component, style, or variable)
and the metric (actions or usages). --group-by is required and its accepted
values depend on the report (e.g. "component"/"team" for component actions).

Results are paged; --limit caps the rows returned and --paginate follows every
page until exhausted (or --limit is reached). --cursor resumes from a cursor
returned by an earlier call.`,
		Args: cobra.ExactArgs(3),
		Example: `  figma analytics component actions abc123 --group-by component
  figma analytics style usages abc123 --group-by file --paginate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			asset, metric := strings.ToLower(args[0]), strings.ToLower(args[1])
			op, err := analyticsOperation(asset, metric)
			if err != nil {
				return err
			}
			if groupBy == "" {
				return &cmdutil.ValidationError{Message: "--group-by is required"}
			}
			if err := validateEnum("--group-by", groupBy, analyticsGroupByValues(asset, metric)...); err != nil {
				return err
			}
			key, err := fileKeyArg(args[2])
			if err != nil {
				return err
			}
			fields := map[string]any{"file_key": key, "group_by": groupBy}
			addOptional(fields, "start_date", startDate)
			addOptional(fields, "end_date", endDate)
			addOptional(fields, "cursor", cursor)

			// Default (no paging flags) preserves the original single-page body
			// verbatim, including its own next_page/cursor fields.
			if !paginate && limit <= 0 {
				return runJSONCall(cmd.Context(), f, op, fields)
			}
			fetch := pageFetcher(cmd.Context(), f, op, fields)
			items, err := figma.Paginate("rows", fetch, figma.NextAnalyticsPage, paginate, limit)
			if err != nil {
				return err
			}
			merged, err := figma.MergeItems("rows", items)
			if err != nil {
				return err
			}
			return writeResultJSON(f, merged)
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "", "dimension to group results by (required)")
	cmd.Flags().StringVar(&startDate, "start-date", "", "ISO 8601 start date (inclusive)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "ISO 8601 end date (inclusive)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of rows to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	return cmd
}

// analyticsOperation maps the (asset, metric) pair to the catalogued
// operationId, e.g. ("component","actions") ->
// getLibraryAnalyticsComponentActions.
func analyticsOperation(asset, metric string) (string, error) {
	assets := map[string]string{"component": "Component", "style": "Style", "variable": "Variable"}
	metrics := map[string]string{"actions": "Actions", "usages": "Usages"}
	a, ok := assets[strings.ToLower(asset)]
	if !ok {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("unknown asset %q (expected component, style, or variable)", asset)}
	}
	m, ok := metrics[strings.ToLower(metric)]
	if !ok {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("unknown metric %q (expected actions or usages)", metric)}
	}
	return "getLibraryAnalytics" + a + m, nil
}
