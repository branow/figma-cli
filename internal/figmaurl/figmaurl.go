// Package figmaurl extracts the identifiers curated commands need from the
// things users actually have: a pasted Figma URL, or a bare key. It pulls
// the file key and node id out of a URL and normalizes a node id between the
// two spellings Figma uses — the hyphen form in URLs (1-2) and the colon
// form the REST API expects (1:2).
package figmaurl

import (
	"net/url"
	"regexp"
	"strings"
)

// Ref is what a Figma URL (or bare key) resolves to: the file key and, when
// the URL carried one, a single node id in API (colon) form.
type Ref struct {
	FileKey string
	NodeID  string
}

// fileURLPattern matches the key segment of a Figma file/design/board/proto
// URL: figma.com/<kind>/<KEY>/<title>. Keys are the URL-safe token Figma
// mints; the title and query are ignored.
var fileURLPattern = regexp.MustCompile(`(?:figma\.com|figma\.site)/(?:file|design|board|proto|slides|deck)/([A-Za-z0-9]+)`)

// FileKey returns the file key from input, which may be a full Figma URL or
// an already-bare key. A bare token (no slashes, no host) is returned as-is;
// anything URL-shaped is parsed. It returns ("", false) when no key can be
// found.
func FileKey(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	if m := fileURLPattern.FindStringSubmatch(input); m != nil {
		return m[1], true
	}
	// Not URL-shaped: treat a plain token as the key itself.
	if !strings.ContainsAny(input, "/?:") {
		return input, true
	}
	return "", false
}

// NodeID returns the node id carried in a URL's node-id query parameter, in
// API (colon) form, or "" when the URL has none. A bare node id passed
// directly is also accepted and normalized.
func NodeID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if u, err := url.Parse(input); err == nil && u.Query().Get("node-id") != "" {
		return NormalizeNodeID(u.Query().Get("node-id"))
	}
	// A value that looks like a bare node id (digits with a - or : separator).
	if bareNodeID.MatchString(input) {
		return NormalizeNodeID(input)
	}
	return ""
}

var bareNodeID = regexp.MustCompile(`^[0-9]+[:-][0-9]+$`)

// Parse resolves both the file key and (optional) node id from a single
// pasted URL or bare key.
func Parse(input string) (Ref, bool) {
	key, ok := FileKey(input)
	if !ok {
		return Ref{}, false
	}
	return Ref{FileKey: key, NodeID: NodeID(input)}, true
}

// NormalizeNodeID converts a node id to the API (colon) form the REST API
// expects, accepting either the URL hyphen form (1-2) or the colon form
// (1:2). An empty or non-matching value is returned unchanged so callers can
// pass through ids in shapes this helper does not recognize.
func NormalizeNodeID(id string) string {
	id = strings.TrimSpace(id)
	if bareNodeID.MatchString(id) {
		return strings.Replace(id, "-", ":", 1)
	}
	return id
}

// URLNodeID converts a node id to the URL (hyphen) form Figma puts in the
// address bar, accepting either spelling.
func URLNodeID(id string) string {
	id = strings.TrimSpace(id)
	if bareNodeID.MatchString(id) {
		return strings.Replace(id, ":", "-", 1)
	}
	return id
}

// orgIDPattern matches a keyword-labelled id segment in a Figma files URL,
// e.g. figma.com/files/team/<id>/..., /project/<id>/..., or /folder/<id>/...
// The %s is the keyword; ids are the URL-safe token Figma mints (digits for
// team/project ids, alphanumeric for folder ids).
var orgIDPattern = map[string]*regexp.Regexp{
	"team":    regexp.MustCompile(`/team/([A-Za-z0-9]+)`),
	"project": regexp.MustCompile(`/project/([A-Za-z0-9]+)`),
	"folder":  regexp.MustCompile(`/folder/([A-Za-z0-9]+)`),
}

// orgID returns the id following keyword in a pasted Figma URL, or the input
// itself when it is already a bare id (no URL structure). It returns
// ("", false) when the input is URL-shaped but carries no such segment.
func orgID(input, keyword string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	// A bare id has no URL structure; take it as-is.
	if !strings.ContainsAny(input, "/?") {
		return input, true
	}
	if m := orgIDPattern[keyword].FindStringSubmatch(input); m != nil {
		return m[1], true
	}
	return "", false
}

// TeamID resolves a team id from a bare id or a pasted Figma team URL
// (figma.com/files/team/<id>/...).
func TeamID(input string) (string, bool) { return orgID(input, "team") }

// ProjectID resolves a project id from a bare id or a pasted Figma project URL
// (figma.com/files/project/<id>/...).
func ProjectID(input string) (string, bool) { return orgID(input, "project") }

// FolderID resolves a folder id from a bare id or a pasted Figma folder URL
// (figma.com/files/folder/<id>/...).
func FolderID(input string) (string, bool) { return orgID(input, "folder") }

// NormalizeIDList normalizes a comma-separated list of node ids to API
// (colon) form and rejoins it, the shape the file/image --ids flag needs.
// Blank entries are dropped.
func NormalizeIDList(ids string) string {
	parts := strings.Split(ids, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, NormalizeNodeID(p))
	}
	return strings.Join(out, ",")
}
