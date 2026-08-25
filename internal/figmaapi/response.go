package figmaapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Response is the decoded body of a completed (2xx) REST response. Figma
// endpoints return either a JSON object or (rarely) a bare JSON array; Body
// holds whichever it was, Fields exposes it as a map for the generic api
// command (a top-level array is wrapped as {"values": [...]}), and Raw keeps
// the untouched bytes for curated commands that decode into typed structs.
type Response struct {
	Status int
	Body   any
	Fields map[string]any
	Raw    []byte
}

// parseResponse decodes a REST response. Non-2xx statuses become *APIError
// (flagged rate-limited when the status and headers say so); a 2xx body
// decodes into a *Response.
func parseResponse(statusCode int, header http.Header, body []byte) (*Response, error) {
	if statusCode < 200 || statusCode > 299 {
		return nil, &APIError{
			Status:      statusCode,
			Message:     errorMessage(body),
			Raw:         body,
			RateLimited: isRateLimited(statusCode, header),
		}
	}

	resp := &Response{Status: statusCode, Raw: body}
	if len(body) == 0 {
		resp.Fields = map[string]any{}
		return resp, nil
	}
	if err := json.Unmarshal(body, &resp.Body); err != nil {
		return nil, &APIError{Status: statusCode, Message: "response was not valid JSON", Raw: body}
	}
	switch v := resp.Body.(type) {
	case map[string]any:
		resp.Fields = v
	case []any:
		resp.Fields = map[string]any{"values": v}
	default:
		resp.Fields = map[string]any{"value": v}
	}
	return resp, nil
}

// isRateLimited reports whether a non-2xx response is a throttling signal to
// back off and retry rather than a hard failure. A 429 always is. A 5xx
// counts only when it carries a Retry-After, so a permanent server error is
// not mistaken for throttling.
func isRateLimited(status int, header http.Header) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status >= 500 && header != nil {
		if _, ok := parseRetryAfter(header.Get("Retry-After")); ok {
			return true
		}
	}
	return false
}

// errorMessage extracts a human-readable message from a Figma error body.
// Figma errors are typically {"status": <int>, "err": "<message>"}, so err
// is read first; message/error cover the handful of endpoints that use those
// keys, and the trimmed raw text is the last resort.
func errorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"err", "message", "error", "detail"} {
			if s, ok := payload[key].(string); ok && s != "" {
				return s
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}
