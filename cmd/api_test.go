package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

// jsonHandler responds with body and records each request's method, path,
// query, and JSON payload for assertions.
type jsonHandler struct {
	body string

	requests  int
	gotMethod string
	gotPath   string
	gotQuery  map[string][]string
	gotBody   map[string]any
}

func (h *jsonHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests++
	h.gotMethod = r.Method
	h.gotPath = r.URL.Path
	h.gotQuery = r.URL.Query()
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &h.gotBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(h.body))
}

func TestAPIHappyPathFillsPathParam(t *testing.T) {
	handler := &jsonHandler{body: `{"name": "My File"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getFile", "-f", "file_key=abc123", "-f", "depth=2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodGet || handler.gotPath != "/v1/files/abc123" {
		t.Errorf("got %s %s, want GET /v1/files/abc123", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotQuery["depth"]; len(got) != 1 || got[0] != "2" {
		t.Errorf("got query depth=%v, want [2]", got)
	}
	if !strings.Contains(out.String(), `"name": "My File"`) {
		t.Errorf("got output %q, want the response name", out.String())
	}
}

func TestAPIPostSendsJSONBody(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "42"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "postComment", "-f", "file_key=abc123", "-f", "message=hi"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if handler.gotMethod != http.MethodPost || handler.gotPath != "/v1/files/abc123/comments" {
		t.Errorf("got %s %s, want POST /v1/files/abc123/comments", handler.gotMethod, handler.gotPath)
	}
	if handler.gotBody["message"] != "hi" {
		t.Errorf("got body %v, want message=hi", handler.gotBody)
	}
}

func TestAPIBatchFailureExitsNonZero(t *testing.T) {
	// postVariables returns HTTP 200 with a populated errors array on a
	// partial or total failure; the generic api path must detect it too.
	handler := &jsonHandler{body: `{"meta":{},"errors":[{"error":"Variable not found"}]}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "postVariables", "-f", "file_key=abc123"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a non-zero exit when a 200 batch body reports errors")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitError {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitError)
	}
	if !strings.Contains(err.Error(), "Variable not found") {
		t.Errorf("error %q should surface the batch item error", err.Error())
	}
	// The body is still written for the reader before the failure is reported.
	if !strings.Contains(out.String(), "Variable not found") {
		t.Errorf("expected the raw body on stdout, got %q", out.String())
	}
}

func TestAPINormalResponseWithoutErrorsSucceeds(t *testing.T) {
	// A normal response carrying no errors array must not be misread as a
	// batch failure — and an empty errors array is not a failure either.
	handler := &jsonHandler{body: `{"links_created":[{"id":"1"}],"errors":[]}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "postDevResources"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected success for an empty errors array, got: %v", err)
	}
}

func TestAPIForbiddenMapsToAuthExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"status": 403, "err": "Invalid token"}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getMe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an HTTP 403 response")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPINotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getMe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when no credentials are configured")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPIUnknownOperationBeatsNotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "noSuchOperation"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown operation")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d (a typo must not be masked by auth state)", got, cmdutil.ExitValidation)
	}
}

func TestAPIListPrintsOperationsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "--list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{"getFile", "getMe", "postComment"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected the operation list to contain %q", want)
		}
	}
}

func TestAPIDescribePrintsFieldsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getFile", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"getFile", "GET /v1/files/{file_key}",
		"Path parameters", "file_key", "required",
		"Query parameters", "depth",
		"Response",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output missing %q, got:\n%s", want, got)
		}
	}
}

func TestAPIDescribeJSON(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "api", "postComment", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var view map[string]any
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("describe -o json is not valid JSON: %v", err)
	}
	if view["operation"] != "postComment" || view["method"] != "POST" {
		t.Errorf("got %v, want postComment/POST", view)
	}
	if _, ok := view["body"].([]any); !ok {
		t.Errorf("want a body field list in %v", view)
	}
}

func TestAPIDescribeRejectsFieldCombination(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getFile", "--describe", "-f", "file_key=abc"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for --describe with --field")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIRejectsInvalidOutputFlag(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "yaml", "api", "--list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for -o yaml")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}
