package catalog_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/branow/figma-cli/internal/figmaapi/catalog"
)

func TestLookupMatchesCaseInsensitively(t *testing.T) {
	for _, id := range []string{"getFile", "getfile", "GETFILE", "GetFile"} {
		op, ok := catalog.Lookup(id)
		if !ok {
			t.Errorf("Lookup(%q) not found", id)
			continue
		}
		if op.Method != http.MethodGet || op.Path != "/v1/files/{file_key}" {
			t.Errorf("Lookup(%q) = %s %s, want GET /v1/files/{file_key}", id, op.Method, op.Path)
		}
	}
}

func TestLookupUnknownOperation(t *testing.T) {
	if _, ok := catalog.Lookup("noSuchOperation"); ok {
		t.Error("Lookup(noSuchOperation) found an operation, want not found")
	}
}

// The Figma spec publishes exactly 54 operationIds (v0.42.0).
func TestOperationsCountAndConsistency(t *testing.T) {
	ids := catalog.Operations()
	if got, want := len(ids), 54; got != want {
		t.Errorf("got %d operationIds, want %d", got, want)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("Operations() not strictly sorted at %q, %q", ids[i-1], ids[i])
		}
	}
}

// Every operation must have a known method and a well-formed path. Guards
// against a distiller bug leaking malformed paths into the embedded catalog.
func TestCataloguedOperationsAreWellFormed(t *testing.T) {
	pathShape := regexp.MustCompile(`^(/[A-Za-z0-9._{}-]+)+$`)
	methods := map[string]bool{
		http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
		http.MethodPatch: true, http.MethodDelete: true,
	}
	for _, id := range catalog.Operations() {
		op, ok := catalog.Lookup(id)
		if !ok {
			t.Fatalf("Operations() listed %q but Lookup could not resolve it", id)
		}
		if !methods[op.Method] {
			t.Errorf("operation %s has unknown method %q", id, op.Method)
		}
		if !pathShape.MatchString(op.Path) {
			t.Errorf("operation %s has a malformed path %q", id, op.Path)
		}
	}
}

func TestOperationFieldsCarryMetadata(t *testing.T) {
	op, ok := catalog.Lookup("getFile")
	if !ok {
		t.Fatal("Lookup(getFile) not found")
	}
	pp, ok := findField(op.PathParams, "file_key")
	if !ok || pp.In != "path" || !pp.Required {
		t.Errorf("getFile path param file_key = %+v, want in=path required=true", pp)
	}

	// postComment's body must record the required message field.
	post, _ := catalog.Lookup("postComment")
	msg, ok := findField(post.Body, "message")
	if !ok || !msg.Required {
		t.Errorf("postComment body message = %+v, want required=true", msg)
	}
}

func TestLookupTypeResolvesComponentSchemas(t *testing.T) {
	fields, ok := catalog.LookupType("Comment")
	if !ok || len(fields) == 0 {
		t.Fatalf("LookupType(Comment) = %v, %v; want a non-empty definition", fields, ok)
	}
	if _, ok := catalog.LookupType("comment"); !ok {
		t.Error("LookupType should match type names case-insensitively")
	}
	if _, ok := catalog.LookupType("NotARealType"); ok {
		t.Error("LookupType returned a definition for an unknown type")
	}
}

func TestCanonicalID(t *testing.T) {
	if got := catalog.CanonicalID("getfile"); got != "getFile" {
		t.Errorf("CanonicalID(getfile) = %q, want getFile", got)
	}
	if got := catalog.CanonicalID("stillUnknown"); got != "stillUnknown" {
		t.Errorf("CanonicalID(stillUnknown) = %q, want it unchanged", got)
	}
}

func findField(fields []catalog.Field, name string) (catalog.Field, bool) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return catalog.Field{}, false
}
