package figma_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/branow/figma-cli/internal/figma"
)

func TestCheckBatchErrors(t *testing.T) {
	// The silent-failure shape from the QA review: HTTP 200, nothing created,
	// a populated errors array.
	raw := []byte(`{"links_created":[],"errors":[{"file_key":"k","node_id":"1:2","error":"File not found"}]}`)
	err := figma.CheckBatchErrors(raw)
	if err == nil {
		t.Fatal("expected a BatchError for a populated errors array")
	}
	var be *figma.BatchError
	if !errors.As(err, &be) {
		t.Fatalf("got %T, want *figma.BatchError", err)
	}
	if !strings.Contains(err.Error(), "File not found") {
		t.Errorf("BatchError message %q should surface the item error", err.Error())
	}
}

func TestCheckBatchErrorsClean(t *testing.T) {
	for _, body := range []string{
		`{"links_created":[{"id":"1"}],"errors":[]}`,
		`{"id":"comment-1","message":"hi"}`,
		`not json at all`,
		``,
	} {
		if err := figma.CheckBatchErrors([]byte(body)); err != nil {
			t.Errorf("CheckBatchErrors(%q) = %v, want nil", body, err)
		}
	}
}
