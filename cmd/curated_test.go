package cmd_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/figma-cli/cmd"
	"github.com/branow/figma-cli/internal/cmdutil"
)

func TestMeShowsUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me" {
			t.Errorf("got path %q, want /v1/me", r.URL.Path)
		}
		w.Write([]byte(`{"id": "1", "handle": "Ada", "email": "ada@example.com"}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"me"})
	if err := root.Execute(); err != nil {
		t.Fatalf("me: %v", err)
	}
	if !strings.Contains(out.String(), "Ada") || !strings.Contains(out.String(), "ada@example.com") {
		t.Errorf("got %q, want the handle and email", out.String())
	}
}

func TestFileVersionsTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/abc123/versions" {
			t.Errorf("got path %q, want /v1/files/abc123/versions", r.URL.Path)
		}
		w.Write([]byte(`{"versions":[{"id":"10","label":"v1","created_at":"2026-01-01","user":{"handle":"Ada"}}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	// Pass a full Figma URL to prove the file key is parsed out of it.
	root.SetArgs([]string{"file", "versions", "https://www.figma.com/design/abc123/My-Design"})
	if err := root.Execute(); err != nil {
		t.Fatalf("file versions: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "v1") || !strings.Contains(got, "Ada") {
		t.Errorf("got %q, want the version row", got)
	}
}

func TestFileVersionsPaginateFollowsPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("before") == "" {
			// First page points at the next via pagination.next_page.
			w.Write([]byte(`{"versions":[{"id":"10","label":"v1","user":{"handle":"Ada"}}],"pagination":{"next_page":"/v1/files/abc123/versions?before=10"}}`))
			return
		}
		w.Write([]byte(`{"versions":[{"id":"9","label":"v0","user":{"handle":"Grace"}}],"pagination":{}}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"file", "versions", "abc123", "--paginate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("file versions --paginate: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "v1") || !strings.Contains(got, "v0") {
		t.Errorf("got %q, want rows from both pages", got)
	}
}

func TestCommentListLimitCaps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"comments":[{"id":"1","message":"a","user":{"handle":"Ada"}},{"id":"2","message":"b","user":{"handle":"Ada"}}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"comment", "list", "abc123", "--limit", "1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("comment list --limit: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "a") {
		t.Errorf("got %q, want the first comment", got)
	}
	if strings.Contains(got, "b") {
		t.Errorf("got %q, want only the first comment with --limit 1", got)
	}
}

func TestCommentListNormalizesAndRenders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"comments":[{"id":"1","message":"Hi","user":{"handle":"Ada"},"created_at":"2026-01-01"}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"comment", "list", "abc123"})
	if err := root.Execute(); err != nil {
		t.Fatalf("comment list: %v", err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Errorf("got %q, want the comment", out.String())
	}
}

func TestProjectFilesAcceptsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/55391681/files" {
			t.Errorf("got path %q, want the project id parsed from the URL", r.URL.Path)
		}
		w.Write([]byte(`{"files":[{"key":"k1","name":"Design","last_modified":"2026-01-01"}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"project", "files", "https://www.figma.com/files/project/55391681/My-Project"})
	if err := root.Execute(); err != nil {
		t.Fatalf("project files: %v", err)
	}
	if !strings.Contains(out.String(), "Design") {
		t.Errorf("got %q, want the file row", out.String())
	}
}

func TestProjectListAcceptsTeamURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/teams/1234567890/projects" {
			t.Errorf("got path %q, want the team id parsed from the URL", r.URL.Path)
		}
		w.Write([]byte(`{"projects":[{"id":"7","name":"Proj"}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"project", "list", "https://www.figma.com/files/team/1234567890/My-Team"})
	if err := root.Execute(); err != nil {
		t.Fatalf("project list: %v", err)
	}
	if !strings.Contains(out.String(), "Proj") {
		t.Errorf("got %q, want the project row", out.String())
	}
}

func TestImageRenderDownloadsAssets(t *testing.T) {
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("PNGDATA"))
	}))
	defer assets.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ids"); got != "1:2" {
			t.Errorf("node id must be normalized to colon form, got ids=%q", got)
		}
		w.Write([]byte(`{"err":null,"images":{"1:2":"` + assets.URL + `/a.png"}}`))
	}))
	defer api.Close()

	dir := t.TempDir()
	f, out, _ := newTestFactory(t, api.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"image", "render", "abc123", "--ids", "1-2", "--out", dir})
	if err := root.Execute(); err != nil {
		t.Fatalf("image render: %v", err)
	}

	path := filepath.Join(dir, "1-2.png")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected downloaded asset at %s: %v", path, err)
	}
	if string(data) != "PNGDATA" {
		t.Errorf("got asset contents %q, want PNGDATA", data)
	}
	if !strings.Contains(out.String(), "1-2.png") {
		t.Errorf("expected the saved path reported, got %q", out.String())
	}
}

func TestImageRenderAllNodesFailExitsNonZero(t *testing.T) {
	// Every requested node came back with a null render URL: a total failure
	// must not report success (exit 0) with an empty download.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"err":null,"images":{"1:2":null}}`))
	}))
	defer api.Close()

	dir := t.TempDir()
	f, _, _ := newTestFactory(t, api.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"image", "render", "abc123", "--ids", "1-2", "--out", dir})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a non-zero exit when no node rendered")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitError {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitError)
	}
}

func TestImageRenderSingleFileOut(t *testing.T) {
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SVGDATA"))
	}))
	defer assets.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"err":null,"images":{"1:2":"` + assets.URL + `/a.svg"}}`))
	}))
	defer api.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "logo.svg")
	f, out, _ := newTestFactory(t, api.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"image", "render", "abc123", "--ids", "1-2", "--format", "svg", "--out", target})
	if err := root.Execute(); err != nil {
		t.Fatalf("image render --out file: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected asset at the exact path %s: %v", target, err)
	}
	if string(data) != "SVGDATA" {
		t.Errorf("got %q, want SVGDATA", data)
	}
	if !strings.Contains(out.String(), "Downloaded 1/1") {
		t.Errorf("expected a download summary, got %q", out.String())
	}
}

func TestImageRenderFileOutRejectsMultipleNodes(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"err":null,"images":{"1:2":"https://s3/a.png","3:4":"https://s3/b.png"}}`))
	}))
	defer api.Close()

	dir := t.TempDir()
	f, _, _ := newTestFactory(t, api.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"image", "render", "abc123", "--ids", "1-2,3-4", "--out", filepath.Join(dir, "logo.png")})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error writing multiple nodes to one file")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestImageRenderPrintsURLsWithoutOut(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"err":null,"images":{"1:2":"https://s3/a.png"}}`))
	}))
	defer api.Close()

	f, out, _ := newTestFactory(t, api.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"image", "render", "abc123", "--ids", "1-2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("image render: %v", err)
	}
	if !strings.Contains(out.String(), "https://s3/a.png") {
		t.Errorf("got %q, want the rendered URL", out.String())
	}
}
