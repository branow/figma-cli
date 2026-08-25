// Package figmaapi is a client for the Figma REST API. It routes
// operationIds to endpoints via the embedded catalog, authenticates with a
// header token (X-Figma-Token for a personal/plan access token, or a Bearer
// token for OAuth), retries transient transport failures with backoff
// (honoring Retry-After), and decodes JSON responses. A lower-level Do
// escape hatch lets curated commands drive endpoints directly.
package figmaapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/branow/figma-cli/internal/credentials"
	"github.com/branow/figma-cli/internal/figmaapi/catalog"
)

const (
	requestTimeout = 30 * time.Second
	// DefaultBaseURL is Figma's single REST API host.
	DefaultBaseURL = "https://api.figma.com"
)

// Client invokes Figma operations. It is an interface so commands can be
// tested against a fake implementation instead of a real network call.
type Client interface {
	// Call routes operation through the catalog, fills path params from
	// fields (case-insensitive by name), sends the rest as query parameters
	// (GET/DELETE) or a JSON body (otherwise), sets the auth header, and
	// returns the parsed response.
	Call(ctx context.Context, operation string, fields map[string]any) (*Response, error)
	// Do is the lower-level escape hatch curated commands use directly:
	// method and a full post-host path, with an explicit query and body.
	Do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error)
}

// HTTPClient is the real Client implementation, sending JSON requests to the
// Figma API base URL with a header token.
type HTTPClient struct {
	baseURL    string
	creds      credentials.Credentials
	httpClient *http.Client
}

// New returns an HTTPClient. baseURL is the scheme+host prefix; when empty
// it defaults to DefaultBaseURL. Tests pass an httptest server URL as
// baseURL directly.
func New(baseURL string, creds credentials.Credentials) *HTTPClient {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		creds:      creds,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// BaseURL resolves the API host for a live client. It is the fixed Figma
// host by default; FIGMA_BASE_URL overrides it for a local proxy or a
// self-hosted gateway. A cleartext http:// override is refused unless
// FIGMA_INSECURE is set, since the token would otherwise travel in the
// clear; tests pass an explicit loopback URL to New directly and are
// unaffected.
func BaseURL() (string, error) {
	override := strings.TrimRight(os.Getenv("FIGMA_BASE_URL"), "/")
	if override == "" {
		return DefaultBaseURL, nil
	}
	switch {
	case strings.HasPrefix(override, "https://"):
		return override, nil
	case strings.HasPrefix(override, "http://"):
		if insecureAllowed() {
			return override, nil
		}
		return "", fmt.Errorf("refusing insecure FIGMA_BASE_URL %q: the token would be sent in cleartext (set FIGMA_INSECURE=1 to override for a local proxy)", override)
	default:
		return "https://" + override, nil
	}
}

func insecureAllowed() bool {
	v := os.Getenv("FIGMA_INSECURE")
	return v == "1" || strings.EqualFold(v, "true")
}

// Call resolves operation via the catalog and invokes its endpoint. An
// unknown id fails before any network call.
func (c *HTTPClient) Call(ctx context.Context, operation string, fields map[string]any) (*Response, error) {
	op, ok := catalog.Lookup(operation)
	if !ok {
		return nil, &UnknownOperationError{Operation: operation}
	}
	if op.Method == "" {
		return nil, &UnsupportedOperationError{Operation: operation}
	}
	method, path, query, body, err := Route(op, fields)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, method, path, query, body)
}

// Do performs one request: baseURL+path, optional query and JSON body, the
// auth header, with retry on transient failures. It is the shared core both
// Call and curated commands funnel through.
func (c *HTTPClient) Do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error) {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	resp, err := c.doWithRetry(ctx, method, requestURL, encoded)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseResponse(resp.StatusCode, resp.Header, responseBody)
}

// Route computes the concrete request for an operation and its fields: path
// params are substituted into the path (case-insensitive by name), and the
// remaining fields become query parameters for GET/DELETE or a JSON body
// otherwise (a field matching a documented query parameter always goes to
// the query). Field names are canonicalized to their documented casing. This
// is the single place the operation-to-request rule lives, shared by
// Client.Call and the api command.
func Route(op catalog.Operation, fields map[string]any) (method, path string, query url.Values, body any, err error) {
	path = op.Path
	consumed := map[string]bool{}

	for _, param := range op.PathParams {
		key, value, found := findField(fields, param.Name)
		if !found {
			return "", "", nil, nil, fmt.Errorf("missing required path parameter %q for operation %s", param.Name, op.ID)
		}
		path = strings.ReplaceAll(path, "{"+param.Name+"}", url.PathEscape(queryValue(value)))
		consumed[key] = true
	}

	query = url.Values{}
	payload := map[string]any{}
	isQueryMethod := op.Method == http.MethodGet || op.Method == http.MethodDelete
	for key, value := range fields {
		if consumed[key] {
			continue
		}
		if isQueryMethod {
			addQueryField(query, op.Query, key, value)
			continue
		}
		if _, ok := matchField(op.Query, key); ok {
			addQueryField(query, op.Query, key, value)
			continue
		}
		payload[canonicalName(op.Body, key)] = value
	}

	if len(payload) > 0 {
		body = payload
	}
	return op.Method, path, query, body, nil
}

// findField returns the field entry matching name case-insensitively, along
// with the caller's original key.
func findField(fields map[string]any, name string) (string, any, bool) {
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			return key, value, true
		}
	}
	return "", nil, false
}

// matchField returns a documented field's canonical name when one matches
// key case-insensitively.
func matchField(fields []catalog.Field, key string) (string, bool) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, key) {
			return f.Name, true
		}
	}
	return "", false
}

// canonicalName returns the documented casing for key when it matches a
// field, else key unchanged (the API is the authority; the catalog may be
// incomplete).
func canonicalName(fields []catalog.Field, key string) string {
	if name, ok := matchField(fields, key); ok {
		return name
	}
	return key
}

// addQueryField adds a query parameter, choosing between the two spellings
// Figma uses for a multi-valued parameter based on the catalog's documented
// type. A JSON array given to a scalar parameter (string or number — every
// Figma list-style query parameter, ids/node_ids/…, is a comma-joined string)
// is joined with commas, matching what the endpoint parses; sending repeated
// keys instead makes Figma receive an array it then tries to .split, leaking a
// cryptic upstream error. A parameter the catalog explicitly types as an array
// still expands into repeated keys. An undocumented parameter is treated as
// scalar, since that is the Figma convention.
func addQueryField(query url.Values, fields []catalog.Field, key string, value any) {
	name := canonicalName(fields, key)
	if arr, ok := value.([]any); ok && !isArrayParam(fields, name) {
		parts := make([]string, 0, len(arr))
		for _, elem := range arr {
			parts = append(parts, queryValue(elem))
		}
		query.Add(name, strings.Join(parts, ","))
		return
	}
	addQuery(query, name, value)
}

// isArrayParam reports whether the catalog documents name as an array-typed
// query parameter (its type carries the "[]" notation), which is the only case
// that should expand into repeated keys.
func isArrayParam(fields []catalog.Field, name string) bool {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return strings.HasSuffix(f.Type, "[]")
		}
	}
	return false
}

// addQuery appends value to query under name. An array value expands into
// repeated keys (ids=1&ids=2); it uses Add, not Set, so a multi-valued
// parameter survives as repeated keys. Scalars are rendered without JSON
// array/quote syntax.
func addQuery(query url.Values, name string, value any) {
	if arr, ok := value.([]any); ok {
		for _, elem := range arr {
			query.Add(name, queryValue(elem))
		}
		return
	}
	query.Add(name, queryValue(value))
}

// queryValue renders a single field value as a query-parameter or path
// string: strings pass through raw, numbers and booleans as their literal
// text (so depth=2 is "2", not a JSON-quoted value), and anything else as
// its JSON encoding.
func queryValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func (c *HTTPClient) doWithRetry(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err = c.attempt(ctx, method, requestURL, body)
		lastAttempt := attempt == maxAttempts-1
		if !shouldRetry(method, resp, err) || lastAttempt {
			return resp, err
		}
		delay := retryDelay(resp, attempt)
		if resp != nil {
			resp.Body.Close()
		}
		if werr := wait(ctx, delay); werr != nil {
			return nil, werr
		}
	}
	return resp, err
}

func (c *HTTPClient) attempt(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	c.setAuth(req)
	return c.httpClient.Do(req)
}

// setAuth applies the auth header selected by the credential kind: OAuth
// tokens travel as Authorization: Bearer, and everything else (a personal or
// plan access token) as the X-Figma-Token header.
func (c *HTTPClient) setAuth(req *http.Request) {
	if c.creds.Token == "" {
		return
	}
	if c.creds.Kind == "oauth" {
		req.Header.Set("Authorization", "Bearer "+c.creds.Token)
		return
	}
	req.Header.Set("X-Figma-Token", c.creds.Token)
}
