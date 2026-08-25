package figma_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/branow/figma-cli/internal/figma"
)

func ptr(s string) *string { return &s }

func TestDownloadWritesAssetsAndReportsFailures(t *testing.T) {
	dir := t.TempDir()
	result := figma.RenderResult{
		Images: map[string]*string{
			"1:2": ptr("https://s3/a.png"),
			"3:4": ptr("https://s3/b.png"),
			"5:6": nil, // failed render — must be reported, not written
		},
	}
	get := func(ctx context.Context, url string) ([]byte, error) {
		return []byte("bytes-of:" + url), nil
	}

	saved, failed, err := figma.Download(context.Background(), get, result, dir, "png", 4)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("got %d saved, want 2", len(saved))
	}
	if len(failed) != 1 || failed[0] != "5:6" {
		t.Errorf("got failed %v, want [5:6]", failed)
	}
	// Node id colon becomes a dash in the file name.
	wantPath := filepath.Join(dir, "1-2.png")
	if saved[0].Path != wantPath {
		t.Errorf("got path %q, want %q", saved[0].Path, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read asset: %v", err)
	}
	if string(data) != "bytes-of:https://s3/a.png" {
		t.Errorf("got contents %q", data)
	}
}

func TestDownloadExtensionFollowsFormat(t *testing.T) {
	dir := t.TempDir()
	result := figma.RenderResult{Images: map[string]*string{"1:2": ptr("https://s3/x")}}
	get := func(ctx context.Context, url string) ([]byte, error) { return []byte("x"), nil }

	saved, _, err := figma.Download(context.Background(), get, result, dir, "svg", 1)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Ext(saved[0].Path) != ".svg" {
		t.Errorf("got %q, want a .svg extension", saved[0].Path)
	}
}
