package figma_test

import (
	"encoding/json"
	"testing"

	"github.com/branow/figma-cli/internal/figma"
)

func TestPaginateFollowsVersionsCursor(t *testing.T) {
	// Two pages: the first carries a pagination.next_page URL with before=100,
	// the second has no next page.
	pages := map[string]string{
		"":    `{"versions":[{"id":"1"},{"id":"2"}],"pagination":{"next_page":"https://api.figma.com/v1/files/k/versions?before=100"}}`,
		"100": `{"versions":[{"id":"3"}],"pagination":{}}`,
	}
	fetch := func(extra map[string]any) ([]byte, error) {
		before, _ := extra["before"].(string)
		return []byte(pages[before]), nil
	}

	items, err := figma.Paginate("versions", fetch, figma.NextVersionsPage, true, 0)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items across pages, want 3", len(items))
	}

	// Without --paginate, only the first page is returned.
	first, err := figma.Paginate("versions", fetch, figma.NextVersionsPage, false, 0)
	if err != nil {
		t.Fatalf("Paginate first page: %v", err)
	}
	if len(first) != 2 {
		t.Errorf("got %d items on first page, want 2", len(first))
	}

	// --limit caps the accumulated set.
	capped, err := figma.Paginate("versions", fetch, figma.NextVersionsPage, true, 2)
	if err != nil {
		t.Fatalf("Paginate limit: %v", err)
	}
	if len(capped) != 2 {
		t.Errorf("got %d items with limit 2, want 2", len(capped))
	}
}

func TestNextAnalyticsPage(t *testing.T) {
	if extra := figma.NextAnalyticsPage([]byte(`{"rows":[],"next_page":true,"cursor":"abc"}`)); extra["cursor"] != "abc" {
		t.Errorf("expected cursor abc, got %v", extra)
	}
	if extra := figma.NextAnalyticsPage([]byte(`{"rows":[],"next_page":false}`)); extra != nil {
		t.Errorf("expected nil at last page, got %v", extra)
	}
}

func TestMergeItemsRoundTrips(t *testing.T) {
	items := []json.RawMessage{json.RawMessage(`{"id":"1"}`), json.RawMessage(`{"id":"2"}`)}
	merged, err := figma.MergeItems("versions", items)
	if err != nil {
		t.Fatalf("MergeItems: %v", err)
	}
	back, err := figma.RawArray(merged, "versions")
	if err != nil || len(back) != 2 {
		t.Errorf("round-trip = %d items err %v, want 2", len(back), err)
	}
}
