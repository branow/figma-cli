package figmaapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/branow/figma-cli/internal/credentials"
	"github.com/branow/figma-cli/internal/figmaapi"
	"github.com/branow/figma-cli/internal/figmaapi/catalog"
)

func TestRouteJoinsScalarArrayQueryWithCommas(t *testing.T) {
	// ids is a scalar (string) query parameter, so a JSON array must be joined
	// with commas into the form Figma parses — not sent as repeated keys, which
	// makes Figma receive an array it then tries to .split (a cryptic upstream
	// error the QA review flagged).
	op := catalog.Operation{
		ID:     "getFileNodes",
		Method: http.MethodGet,
		Path:   "/v1/files/{file_key}/nodes",
		Query:  []catalog.Field{{Name: "ids", Type: "string"}, {Name: "scale"}, {Name: "use_absolute_bounds"}},
	}
	_, _, query, _, err := figmaapi.Route(op, map[string]any{
		"file_key":            "abc",
		"ids":                 []any{"1:2", "3:4"},
		"scale":               2,
		"use_absolute_bounds": true,
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got := query["ids"]; len(got) != 1 || got[0] != "1:2,3:4" {
		t.Errorf("a scalar array param must join with commas, got ids=%v", got)
	}
	if got := query.Get("scale"); got != "2" {
		t.Errorf("number param must render as its literal, got scale=%q", got)
	}
	if got := query.Get("use_absolute_bounds"); got != "true" {
		t.Errorf("bool param must render as true/false, got %q", got)
	}
}

func TestRouteExpandsExplicitArrayQueryIntoRepeatedKeys(t *testing.T) {
	// A parameter the catalog explicitly types as an array still expands into
	// repeated keys.
	op := catalog.Operation{
		ID:     "hypothetical",
		Method: http.MethodGet,
		Path:   "/v1/x",
		Query:  []catalog.Field{{Name: "tag", Type: "string[]"}},
	}
	_, _, query, _, err := figmaapi.Route(op, map[string]any{"tag": []any{"a", "b"}})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got := query["tag"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("array-typed param must expand into repeated keys, got tag=%v", got)
	}
}

func TestBaseURLDefaultAndOverride(t *testing.T) {
	if got, err := figmaapi.BaseURL(); err != nil || got != figmaapi.DefaultBaseURL {
		t.Errorf("default BaseURL = %q err %v, want %q", got, err, figmaapi.DefaultBaseURL)
	}
	t.Setenv("FIGMA_BASE_URL", "http://127.0.0.1:9000")
	if _, err := figmaapi.BaseURL(); err == nil {
		t.Error("cleartext http override must be rejected without FIGMA_INSECURE")
	}
	t.Setenv("FIGMA_INSECURE", "1")
	if got, err := figmaapi.BaseURL(); err != nil || got != "http://127.0.0.1:9000" {
		t.Errorf("FIGMA_INSECURE must allow http, got %q err %v", got, err)
	}
}

func testCreds() credentials.Credentials {
	return credentials.Credentials{Kind: "pat", Token: "figd_t0ken"}
}

func TestCallGETFillsPathParamQueryAndTokenHeader(t *testing.T) {
	var gotMethod, gotPath, gotToken string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotToken = r.Header.Get("X-Figma-Token")
		w.Write([]byte(`{"name": "My File"}`))
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	resp, err := client.Call(context.Background(), "getFile", map[string]any{
		"file_key": "abc123",
		"depth":    2,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/files/abc123" {
		t.Errorf("got %s %s, want GET /v1/files/abc123", gotMethod, gotPath)
	}
	if got := gotQuery.Get("depth"); got != "2" {
		t.Errorf("got depth=%q, want 2", got)
	}
	if _, present := gotQuery["file_key"]; present {
		t.Error("path parameter must not leak into the query string")
	}
	if gotToken != "figd_t0ken" {
		t.Errorf("got X-Figma-Token %q, want figd_t0ken", gotToken)
	}
	if resp.Fields["name"] != "My File" {
		t.Errorf("got name %v, want My File", resp.Fields["name"])
	}
}

func TestOAuthCredsUseBearer(t *testing.T) {
	var gotAuth, gotFigma string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotFigma = r.Header.Get("X-Figma-Token")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, credentials.Credentials{Kind: "oauth", Token: "oauth_tok"})
	if _, err := client.Call(context.Background(), "getMe", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotAuth != "Bearer oauth_tok" {
		t.Errorf("got Authorization %q, want Bearer oauth_tok", gotAuth)
	}
	if gotFigma != "" {
		t.Errorf("OAuth requests must not send X-Figma-Token, got %q", gotFigma)
	}
}

func TestCallPOSTSendsJSONBodyAndRoutesQueryParams(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.Write([]byte(`{"id": "42"}`))
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "postComment", map[string]any{
		"file_key": "abc123",
		"message":  "hi",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/files/abc123/comments" {
		t.Errorf("got %s %s, want POST /v1/files/abc123/comments", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", gotContentType)
	}
	if gotBody["message"] != "hi" {
		t.Errorf("got body %v, want message=hi", gotBody)
	}
	if _, present := gotBody["file_key"]; present {
		t.Errorf("path param must not appear in the JSON body: %v", gotBody)
	}
}

func TestCallDecodes404AsFigmaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status": 404, "err": "Not found"}`))
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	resp, err := client.Call(context.Background(), "getFile", map[string]any{"file_key": "nope"})

	var apiErr *figmaapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got error %v, want *figmaapi.APIError", err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Message != "Not found" {
		t.Errorf("got %d %q, want 404 Not found", apiErr.Status, apiErr.Message)
	}
	if resp != nil {
		t.Errorf("got resp %v, want nil for a non-2xx response", resp)
	}
}

func TestCallGETRetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"name": "ok"}`))
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "getFile", map[string]any{"file_key": "abc"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("got %d attempts, want 3", got)
	}
}

func TestCallPOSTDoesNotRetryOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "postComment", map[string]any{"file_key": "abc", "message": "x"})

	var apiErr *figmaapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got error %v, want *figmaapi.APIError", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("got %d attempts, want exactly 1 (mutating calls must not retry on 5xx)", got)
	}
}

func TestCallRejectsUnknownOperationWithoutRequest(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := figmaapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "noSuchOperation", nil)
	var unknownErr *figmaapi.UnknownOperationError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("got error %v, want *figmaapi.UnknownOperationError", err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("server was hit %d times, want 0 (routing must fail before any HTTP request)", got)
	}
}
