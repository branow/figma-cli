package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/branow/figma-cli/internal/cmdutil"
)

// This file holds the client-side validators for the enum-ish and numeric
// flags whose accepted values are known ahead of the request. Catching a bad
// value here fails fast with exit 3 and a message listing the valid options,
// instead of round-tripping to the API for a 400/403 — and it works offline.

// requireFlags returns a ValidationError naming the first empty required flag,
// checked in argument order so the message is deterministic when more than one
// is omitted (a map's randomized iteration order would otherwise pick one at
// random). Each pair is {flag-name, value}.
func requireFlags(pairs ...[2]string) error {
	for _, p := range pairs {
		if p[1] == "" {
			return &cmdutil.ValidationError{Message: p[0] + " is required"}
		}
	}
	return nil
}

// validateEnum checks value against allowed (an empty value is treated as
// unset and accepted), returning a ValidationError naming the flag and the
// accepted set otherwise.
func validateEnum(flag, value string, allowed ...string) error {
	if value == "" || slices.Contains(allowed, value) {
		return nil
	}
	return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid %s %q (expected one of: %s)", flag, value, strings.Join(allowed, ", "))}
}

// validateRenderFormat checks image render --format against Figma's supported
// output formats.
func validateRenderFormat(format string) error {
	return validateEnum("--format", strings.ToLower(format), "png", "jpg", "svg", "pdf")
}

// validateRenderScale checks image render --scale is within Figma's 0.01–4
// range.
func validateRenderScale(scale string) error {
	if scale == "" {
		return nil
	}
	v, err := strconv.ParseFloat(scale, 64)
	if err != nil || v < 0.01 || v > 4 {
		return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --scale %q (expected a number between 0.01 and 4)", scale)}
	}
	return nil
}

// validateWebhookStatus checks a webhook --status against the two states the
// API accepts.
func validateWebhookStatus(status string) error {
	return validateEnum("--status", status, "ACTIVE", "PAUSED")
}

// validateDepth checks a --depth is a positive integer, the shape the file
// endpoints expect for tree traversal depth.
func validateDepth(depth string) error {
	if depth == "" {
		return nil
	}
	if n, err := strconv.Atoi(depth); err != nil || n < 1 {
		return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --depth %q (expected a positive integer)", depth)}
	}
	return nil
}

// validateGeometry checks a --geometry value; Figma accepts only "paths".
func validateGeometry(geometry string) error {
	return validateEnum("--geometry", geometry, "paths")
}

// analyticsGroupByValues returns the group-by dimensions a given analytics
// report accepts: the asset itself plus "team" for the actions metric or
// "file" for usages, matching the spec's per-report enums.
func analyticsGroupByValues(asset, metric string) []string {
	dimension := "file"
	if metric == "actions" {
		dimension = "team"
	}
	return []string{asset, dimension}
}
