package figma

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Getter fetches the bytes at an asset URL. It is an interface seam so the
// download loop can be tested without real network access.
type Getter func(ctx context.Context, url string) ([]byte, error)

// Saved records one downloaded asset: the node it renders and the file it
// landed in.
type Saved struct {
	NodeID string
	Path   string
}

// RenderResult is the getImages response: a map from node id to a rendered
// asset URL (null when that node failed to render) plus an optional error
// string.
type RenderResult struct {
	Err    string             `json:"err"`
	Images map[string]*string `json:"images"`
}

// ParseRenderResult decodes a getImages response body.
func ParseRenderResult(raw []byte) (RenderResult, error) {
	var r RenderResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return RenderResult{}, err
	}
	return r, nil
}

// Download writes each rendered asset in result to dir, naming files by node
// id and the format's extension (a colon in a node id becomes a dash so the
// name is filesystem-safe). Nodes whose render URL is null are skipped and
// returned as failures. Up to concurrency assets are fetched at once (a value
// below 1 means serial); the returned saved and failed slices are ordered by
// node id regardless of fetch order, so the result stays deterministic. The
// directory is created if missing. On the first fetch or write error it returns
// that error with whatever it had already collected.
func Download(ctx context.Context, get Getter, result RenderResult, dir, format string, concurrency int) (saved []Saved, failed []string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	nodes := make([]string, 0, len(result.Images))
	for node := range result.Images {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	if concurrency < 1 {
		concurrency = 1
	}
	ext := extensionFor(format)

	type outcome struct {
		saved  *Saved
		failed string
		err    error
	}
	outcomes := make([]outcome, len(nodes))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, node := range nodes {
		url := result.Images[node]
		if url == nil || *url == "" {
			outcomes[i] = outcome{failed: node}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, node, u string) {
			defer wg.Done()
			defer func() { <-sem }()
			path := filepath.Join(dir, assetName(node, ext))
			if err := SaveAsset(ctx, get, u, path); err != nil {
				outcomes[i] = outcome{err: fmt.Errorf("downloading node %s: %w", node, err)}
				return
			}
			s := Saved{NodeID: node, Path: path}
			outcomes[i] = outcome{saved: &s}
		}(i, node, *url)
	}
	wg.Wait()

	for _, o := range outcomes {
		if o.err != nil {
			return saved, failed, o.err
		}
		if o.saved != nil {
			saved = append(saved, *o.saved)
		}
		if o.failed != "" {
			failed = append(failed, o.failed)
		}
	}
	return saved, failed, nil
}

// SaveAsset fetches the asset at url and writes it to path, creating the
// parent directory if needed. It backs both the multi-asset Download and the
// single-file "image render --out logo.png" path.
func SaveAsset(ctx context.Context, get Getter, url, path string) error {
	data, err := get(ctx, url)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// assetName builds a filesystem-safe file name from a node id and extension.
func assetName(node, ext string) string {
	safe := strings.NewReplacer(":", "-", "/", "-").Replace(node)
	return safe + ext
}

// extensionFor maps a render format to its file extension, defaulting to the
// format itself prefixed with a dot.
func extensionFor(format string) string {
	switch strings.ToLower(format) {
	case "", "png":
		return ".png"
	case "jpg", "jpeg":
		return ".jpg"
	case "svg":
		return ".svg"
	case "pdf":
		return ".pdf"
	default:
		return "." + strings.ToLower(format)
	}
}
