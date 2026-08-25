package cmd_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

func TestCommentAddPrintsFriendlyConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"1901170878","message":"hi","user":{"handle":"Ada"}}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"comment", "add", "abc123", "--message", "hi"})
	if err := root.Execute(); err != nil {
		t.Fatalf("comment add: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != "Added comment 1901170878" {
		t.Errorf("got %q, want a friendly one-line confirmation, not raw JSON", got)
	}
}

func TestCommentAddJSONModeStillEmitsRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"1901170878","message":"hi"}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "comment", "add", "abc123", "--message", "hi"})
	if err := root.Execute(); err != nil {
		t.Fatalf("comment add -o json: %v", err)
	}
	if !strings.Contains(out.String(), `"message": "hi"`) {
		t.Errorf("got %q, want the raw JSON body under -o json", out.String())
	}
}

func TestCommentReactConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":false,"status":200,"i18n":null}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"comment", "react", "abc123", "12345", "--emoji", ":eyes:"})
	if err := root.Execute(); err != nil {
		t.Fatalf("comment react: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != "Reacted :eyes: on comment 12345" {
		t.Errorf("got %q, want a friendly confirmation, not the raw envelope", got)
	}
}

func TestDevResourceCreateSilentFailureExitsNonZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTTP 200 but nothing created — the silent-failure bug.
		w.Write([]byte(`{"links_created":[],"errors":[{"file_key":"k","node_id":"2:2","error":"File not found"}]}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"dev-resource", "create", "--file", "abc123", "--node", "2-2", "--name", "Spec", "--url", "https://x/y"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a non-zero exit when the batch body reports nothing created")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitError {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitError)
	}
	if !strings.Contains(err.Error(), "File not found") {
		t.Errorf("error %q should surface the API item error", err.Error())
	}
}

func TestDevResourceCreateSilentFailureExitsNonZeroInJSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"links_created":[],"errors":[{"error":"File not found"}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "dev-resource", "create", "--file", "abc123", "--node", "2-2", "--name", "Spec", "--url", "https://x/y"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a non-zero exit even in JSON mode")
	}
	// JSON body is still emitted for the machine reader.
	if !strings.Contains(out.String(), "File not found") {
		t.Errorf("expected the raw body on stdout under -o json, got %q", out.String())
	}
}

func TestDevResourceCreateSuccessConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"links_created":[{"id":"42","name":"Spec"}],"errors":[]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"dev-resource", "create", "--file", "abc123", "--node", "2-2", "--name", "Spec", "--url", "https://x/y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("dev-resource create: %v", err)
	}
	if !strings.Contains(out.String(), "Created dev resource") {
		t.Errorf("got %q, want a friendly confirmation", out.String())
	}
}
