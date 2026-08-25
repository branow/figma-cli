package figmaapi

import (
	"net/http"
	"testing"
)

func TestParseResponseFlagsRateLimited(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header http.Header
		want   bool
	}{
		{"429 is rate-limited", 429, nil, true},
		{"503 with Retry-After is rate-limited", 503, http.Header{"Retry-After": {"5"}}, true},
		{"503 without Retry-After is not", 503, nil, false},
		{"500 is not", 500, nil, false},
		{"400 is not", 400, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseResponse(tc.status, tc.header, []byte(`{"status":500,"err":"nope"}`))
			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("got %T, want *APIError", err)
			}
			if apiErr.RateLimited != tc.want {
				t.Errorf("RateLimited = %v, want %v", apiErr.RateLimited, tc.want)
			}
		})
	}
}

// Figma error bodies use "err", not "message"; extraction must read it first.
func TestErrorMessageReadsErrFirst(t *testing.T) {
	cases := map[string]string{
		`{"status":404,"err":"Not found"}`:      "Not found",
		`{"message":"legacy shape"}`:            "legacy shape",
		`{"error":"other shape"}`:               "other shape",
		`plain text error`:                      "plain text error",
		`{"status":400,"err":"","message":"m"}`: "m",
	}
	for body, want := range cases {
		if got := errorMessage([]byte(body)); got != want {
			t.Errorf("errorMessage(%q) = %q, want %q", body, got, want)
		}
	}
}
