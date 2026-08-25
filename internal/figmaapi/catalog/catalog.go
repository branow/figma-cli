// Package catalog maps Figma operationIds to their REST endpoints. The
// embedded catalog is distilled from the published OpenAPI spec by
// scripts/build-catalog; regenerate it there (make catalog) rather than
// editing figma-catalog.json by hand.
//
// Figma is a single product with one host, so operationIds are unique and a
// Lookup returns the single match — there is no product namespacing.
package catalog

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed figma-catalog.json
var catalogJSON []byte

// Field is one path, query, body, or response field of an operation.
type Field struct {
	// Name is the field name in its documented (wire) casing.
	Name string `json:"name"`
	// In is where the field travels: "path", "query", or "body". Body fields
	// omit it.
	In string `json:"in,omitempty"`
	// Type is the OpenAPI type/format, an "X[]" array notation, or a
	// referenced component-schema name, empty when not documented.
	Type string `json:"type,omitempty"`
	// Required marks required request fields; meaningful for request fields
	// only.
	Required bool `json:"required,omitempty"`
}

// Operation describes one operationId's REST binding.
type Operation struct {
	// ID is the operationId in its canonical (documented) casing.
	ID string `json:"id"`
	// Method is the upper-case HTTP method.
	Method string `json:"method"`
	// Path is the full post-host path (the client prepends the API host);
	// path params appear as {name}.
	Path string `json:"path"`
	// Summary is a short human description, when the spec provides one.
	Summary string `json:"summary,omitempty"`
	// Deprecated marks operations the spec flags as deprecated.
	Deprecated bool `json:"deprecated,omitempty"`
	// PathParams lists the {name} parameters embedded in Path.
	PathParams []Field `json:"pathParams,omitempty"`
	// Query lists the documented query parameters.
	Query []Field `json:"query,omitempty"`
	// Body lists the flattened top-level request-body schema properties.
	Body []Field `json:"body,omitempty"`
	// Response lists the flattened top-level success-response properties.
	Response []Field `json:"response,omitempty"`
}

// catalogFile is the on-disk shape of figma-catalog.json: the operations as
// a flat list plus a registry of component schemas used to expand nested
// field shapes.
type catalogFile struct {
	Operations []Operation        `json:"operations"`
	Types      map[string][]Field `json:"types,omitempty"`
}

var (
	byLowerID   map[string]Operation
	canonicalID map[string]string
	allIDs      []string
	typeDefs    map[string][]Field
)

func init() {
	var file catalogFile
	if err := json.Unmarshal(catalogJSON, &file); err != nil {
		panic("figmaapi/catalog: embedded figma-catalog.json is invalid: " + err.Error())
	}
	byLowerID = make(map[string]Operation, len(file.Operations))
	canonicalID = make(map[string]string, len(file.Operations))
	allIDs = make([]string, 0, len(file.Operations))
	for _, op := range file.Operations {
		lower := strings.ToLower(op.ID)
		byLowerID[lower] = op
		canonicalID[lower] = op.ID
		allIDs = append(allIDs, op.ID)
	}
	sort.Strings(allIDs)
	typeDefs = make(map[string][]Field, len(file.Types))
	for name, fields := range file.Types {
		typeDefs[strings.ToLower(name)] = fields
	}
}

// Lookup returns the operation for id, matched case-insensitively.
func Lookup(id string) (Operation, bool) {
	op, ok := byLowerID[strings.ToLower(id)]
	return op, ok
}

// Operations returns every operationId in canonical casing, sorted, for
// listing and shell completion.
func Operations() []string {
	return allIDs
}

// LookupType returns the field list defining a component schema, matched
// case-insensitively, so a field typed as a component can be expanded into
// its own fields. Opaque or unreferenced types are absent.
func LookupType(name string) ([]Field, bool) {
	fields, ok := typeDefs[strings.ToLower(name)]
	return fields, ok
}

// CanonicalID returns the documented casing for id, matched
// case-insensitively; unknown ids pass through unchanged.
func CanonicalID(id string) string {
	if name, ok := canonicalID[strings.ToLower(id)]; ok {
		return name
	}
	return id
}
