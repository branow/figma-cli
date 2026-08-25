package figma

import (
	"encoding/json"
	"net/url"
)

// Figma paginates its list and history endpoints a couple of different ways —
// the file-versions endpoint hands back a next_page URL carrying a before/after
// cursor, the library-analytics endpoints hand back an opaque cursor plus a
// next_page boolean — and the curated commands must be able to follow either to
// completion. These helpers are the page-following loop kept pure over a
// "fetch one page" callback and the raw bodies it returns, so they can be
// table-tested without Cobra or HTTP and every command paginates identically.

// FetchPage fetches one raw page body, given the field overrides to merge into
// the base request (nil for the first page). It returns the untouched response
// bytes so the caller can both accumulate typed items and read the pagination
// cursor from the same body.
type FetchPage func(extra map[string]any) ([]byte, error)

// NextPage derives the field overrides that fetch the page after the one in
// raw, or nil when raw is the last page. It encapsulates each endpoint's own
// cursor shape.
type NextPage func(raw []byte) map[string]any

// Paginate follows an endpoint's pages via fetch, accumulating the raw items
// found under arrayKey in order. It stops when next returns nil (last page),
// when paginate is false (only the first page is fetched, still honoring
// limit), or when limit items have been collected (limit <= 0 means no cap). On
// a mid-pagination fetch or decode failure it returns the items already
// collected alongside the error, so a caller can still render the partial set
// rather than losing data it had in hand.
func Paginate(arrayKey string, fetch FetchPage, next NextPage, paginate bool, limit int) ([]json.RawMessage, error) {
	items := []json.RawMessage{}
	var extra map[string]any
	for {
		raw, err := fetch(extra)
		if err != nil {
			return items, err
		}
		pageItems, err := RawArray(raw, arrayKey)
		if err != nil {
			return items, err
		}
		items = append(items, pageItems...)
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if !paginate {
			return items, nil
		}
		extra = next(raw)
		if extra == nil {
			return items, nil
		}
	}
}

// RawArray pulls the named array out of a page body as its raw JSON elements,
// preserving each element's bytes so it can be re-decoded with the endpoint's
// own typed model or re-emitted losslessly. A body without the key yields nil.
func RawArray(raw []byte, key string) ([]json.RawMessage, error) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	inner, ok := env[key]
	if !ok {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(inner, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// MergeItems wraps collected raw items back under key, the combined envelope a
// paginated -o json emits so a multi-page result reads like a single page and
// re-parses with the endpoint's own decoder unchanged.
func MergeItems(key string, items []json.RawMessage) ([]byte, error) {
	if items == nil {
		items = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{key: items})
}

// DecodeItems decodes raw JSON items into a typed slice, for rendering the
// accumulated pages as a table.
func DecodeItems[T any](items []json.RawMessage) ([]T, error) {
	out := make([]T, 0, len(items))
	for _, it := range items {
		var v T
		if err := json.Unmarshal(it, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// NextVersionsPage reads the file-versions endpoint's pagination.next_page URL
// and returns the before/after cursor it carries as request field overrides,
// or nil when there is no next page.
func NextVersionsPage(raw []byte) map[string]any {
	var env struct {
		Pagination struct {
			NextPage string `json:"next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Pagination.NextPage == "" {
		return nil
	}
	u, err := url.Parse(env.Pagination.NextPage)
	if err != nil {
		return nil
	}
	q := u.Query()
	extra := map[string]any{}
	for _, key := range []string{"before", "after"} {
		if v := q.Get(key); v != "" {
			extra[key] = v
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// NextAnalyticsPage reads a library-analytics response's next_page flag and
// cursor, returning the cursor as a request field override, or nil when there
// is no next page.
func NextAnalyticsPage(raw []byte) map[string]any {
	var env struct {
		NextPage bool   `json:"next_page"`
		Cursor   string `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || !env.NextPage || env.Cursor == "" {
		return nil
	}
	return map[string]any{"cursor": env.Cursor}
}
