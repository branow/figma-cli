package figmaurl_test

import (
	"testing"

	"github.com/branow/figma-cli/internal/figmaurl"
)

func TestFileKeyFromURLForms(t *testing.T) {
	cases := map[string]string{
		"https://www.figma.com/file/abc123XYZ/My-Design":           "abc123XYZ",
		"https://www.figma.com/design/KEY456/Board?node-id=1-2":    "KEY456",
		"https://figma.com/proto/pk9/Prototype?node-id=10%3A20":    "pk9",
		"figma.com/board/BRD1/Whiteboard":                          "BRD1",
		"abc123XYZ":                                                "abc123XYZ",
		"https://www.figma.com/design/hSlM/Name?node-id=3-4&t=x-0": "hSlM",
	}
	for input, want := range cases {
		got, ok := figmaurl.FileKey(input)
		if !ok || got != want {
			t.Errorf("FileKey(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
}

func TestFileKeyRejectsUnparseable(t *testing.T) {
	for _, input := range []string{"", "   ", "https://example.com/nope"} {
		if got, ok := figmaurl.FileKey(input); ok {
			t.Errorf("FileKey(%q) = %q, want not found", input, got)
		}
	}
}

func TestNodeIDFromURL(t *testing.T) {
	cases := map[string]string{
		"https://www.figma.com/design/KEY/Name?node-id=1-2":   "1:2",
		"https://www.figma.com/design/KEY/Name?node-id=10:20": "10:20",
		"1-2": "1:2",
		"1:2": "1:2",
		"KEY": "",
		"":    "",
	}
	for input, want := range cases {
		if got := figmaurl.NodeID(input); got != want {
			t.Errorf("NodeID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAndURLNodeID(t *testing.T) {
	if got := figmaurl.NormalizeNodeID("1-2"); got != "1:2" {
		t.Errorf("NormalizeNodeID(1-2) = %q, want 1:2", got)
	}
	if got := figmaurl.NormalizeNodeID("1:2"); got != "1:2" {
		t.Errorf("NormalizeNodeID(1:2) = %q, want 1:2", got)
	}
	if got := figmaurl.URLNodeID("1:2"); got != "1-2" {
		t.Errorf("URLNodeID(1:2) = %q, want 1-2", got)
	}
	// A shape the helper does not recognize passes through unchanged.
	if got := figmaurl.NormalizeNodeID("I2:3;4"); got != "I2:3;4" {
		t.Errorf("NormalizeNodeID passthrough = %q", got)
	}
}

func TestNormalizeIDList(t *testing.T) {
	if got := figmaurl.NormalizeIDList("1-2, 3-4 ,5:6"); got != "1:2,3:4,5:6" {
		t.Errorf("NormalizeIDList = %q, want 1:2,3:4,5:6", got)
	}
	if got := figmaurl.NormalizeIDList(" , "); got != "" {
		t.Errorf("NormalizeIDList(blank) = %q, want empty", got)
	}
}

func TestOrgIDsFromURLsAndBareIDs(t *testing.T) {
	team := map[string]string{
		"1234567890": "1234567890",
		"https://www.figma.com/files/team/1234567890/My-Team":            "1234567890",
		"https://www.figma.com/files/1099/project/55/x?fuid=1#team/9/no": "",
	}
	for in, want := range team {
		got, ok := figmaurl.TeamID(in)
		if want == "" {
			if ok {
				t.Errorf("TeamID(%q) = %q, want not found", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("TeamID(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}

	if got, ok := figmaurl.ProjectID("https://www.figma.com/files/project/55391681/My-Project"); !ok || got != "55391681" {
		t.Errorf("ProjectID URL = %q, %v; want 55391681", got, ok)
	}
	if got, ok := figmaurl.ProjectID("55391681"); !ok || got != "55391681" {
		t.Errorf("ProjectID bare = %q, %v; want 55391681", got, ok)
	}
	if got, ok := figmaurl.FolderID("https://www.figma.com/files/folder/abc987/Shared"); !ok || got != "abc987" {
		t.Errorf("FolderID URL = %q, %v; want abc987", got, ok)
	}
	// A URL with no matching segment is rejected rather than guessed.
	if got, ok := figmaurl.ProjectID("https://www.figma.com/files/team/42/x"); ok {
		t.Errorf("ProjectID(team URL) = %q, want not found", got)
	}
}

func TestParse(t *testing.T) {
	ref, ok := figmaurl.Parse("https://www.figma.com/design/KEY/Name?node-id=7-8")
	if !ok || ref.FileKey != "KEY" || ref.NodeID != "7:8" {
		t.Errorf("Parse = %+v, %v; want {KEY 7:8} true", ref, ok)
	}
}
