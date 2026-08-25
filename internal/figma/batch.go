package figma

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BatchError reports a batch mutation that returned HTTP 200 but whose body
// carried per-item failures — the "errors" array Figma's dev-resource and
// variable batch endpoints use. The transport succeeded, so the status code is
// 2xx, yet nothing (or only part) of the request was applied. Surfacing it as
// a typed error lets a script detect the failure the status code hid; it maps
// to the generic non-zero exit rather than a transport or validation code.
type BatchError struct {
	Messages []string
}

func (e *BatchError) Error() string {
	if len(e.Messages) == 1 {
		return "the API rejected the request: " + e.Messages[0]
	}
	return fmt.Sprintf("the API reported %d errors: %s", len(e.Messages), strings.Join(e.Messages, "; "))
}

// CheckBatchErrors inspects a 200 batch-mutation body for a non-empty "errors"
// array and returns a BatchError describing it. Figma's dev-resource and
// variable batch endpoints report per-item failures this way while still
// returning HTTP 200, so this array is the only signal a script has that the
// write did not take. A body that is not such an envelope, or whose errors
// array is empty, yields nil.
func CheckBatchErrors(raw []byte) error {
	var env struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	if len(env.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(env.Errors))
	for _, e := range env.Errors {
		msgs = append(msgs, batchErrorMessage(e))
	}
	return &BatchError{Messages: msgs}
}

// batchErrorMessage renders one entry of an errors array as display text,
// accepting either a bare string or an object carrying an error/message field.
func batchErrorMessage(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"error", "message", "err", "reason"} {
			if v, ok := obj[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(string(raw))
}
