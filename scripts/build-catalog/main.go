// Command build-catalog distills the embedded operation catalog from the
// Figma OpenAPI spec in specs/figma-openapi.yaml into
// internal/figmaapi/catalog/figma-catalog.json. Run it via
// `go run ./scripts/build-catalog` (or `make catalog`).
//
// It is deliberately dumb: it reads the spec generically as YAML, walks
// paths/operations, and records each operation's method, full post-host
// path, path/query parameters, flattened request-body and success-response
// properties, plus a registry of the component schemas those fields
// reference (one level of $ref following, transitively closed with a cycle
// guard). The client stays schema-agnostic; all Figma knowledge lives in the
// generated JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const specFile = "figma-openapi.yaml"

// httpMethods is the set of path-item keys that denote an operation.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true,
	"delete": true, "patch": true, "options": true, "head": true,
}

// Field mirrors catalog.Field. Body/type fields omit In.
type Field struct {
	Name     string `json:"name"`
	In       string `json:"in,omitempty"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// Operation mirrors catalog.Operation.
type Operation struct {
	ID         string  `json:"id"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Summary    string  `json:"summary,omitempty"`
	Deprecated bool    `json:"deprecated,omitempty"`
	PathParams []Field `json:"pathParams,omitempty"`
	Query      []Field `json:"query,omitempty"`
	Body       []Field `json:"body,omitempty"`
	Response   []Field `json:"response,omitempty"`
}

// catalogFile mirrors catalog.catalogFile.
type catalogFile struct {
	Operations []Operation        `json:"operations"`
	Types      map[string][]Field `json:"types,omitempty"`
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	specPath := filepath.Join(root, "specs", specFile)
	outPath := filepath.Join(root, "internal", "figmaapi", "catalog", "figma-catalog.json")

	spec, err := readSpec(specPath)
	if err != nil {
		fatal(fmt.Errorf("%s: %w", specFile, err))
	}
	comps := namedComponents(spec, "schemas")
	paramComps := namedComponents(spec, "parameters")
	bodyComps := namedComponents(spec, "requestBodies")
	respComps := namedComponents(spec, "responses")

	operations := distill(spec, comps, paramComps, bodyComps, respComps)
	types := map[string][]Field{}
	collectTypes(operations, comps, types)

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})

	out := catalogFile{Operations: operations, Types: types}
	if err := writeJSON(outPath, out); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %d operations, %d types -> %s\n", len(operations), len(types), outPath)
}

// distill turns the parsed spec into its operations, deduping operationIds
// (last wins) in a stable order.
func distill(spec, comps, paramComps, bodyComps, respComps map[string]any) []Operation {
	base := basePath(spec)
	paths, _ := spec["paths"].(map[string]any)

	byID := map[string]Operation{}
	var order []string
	for _, rawPath := range sortedKeys(paths) {
		item, ok := paths[rawPath].(map[string]any)
		if !ok {
			continue
		}
		sharedParams, _ := item["parameters"].([]any)
		for _, method := range sortedKeys(item) {
			if !httpMethods[strings.ToLower(method)] {
				continue
			}
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				continue
			}
			built := buildOperation(id, method, base+rawPath, op, sharedParams, comps, paramComps, bodyComps, respComps)
			if _, seen := byID[id]; !seen {
				order = append(order, id)
			}
			byID[id] = built
		}
	}

	out := make([]Operation, 0, len(byID))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// buildOperation assembles one Operation from its raw spec node, merging
// path-item-level parameters with the operation's own.
func buildOperation(id, method, fullPath string, op map[string]any, sharedParams []any, comps, paramComps, bodyComps, respComps map[string]any) Operation {
	result := Operation{
		ID:         id,
		Method:     strings.ToUpper(method),
		Path:       fullPath,
		Summary:    str(op["summary"]),
		Deprecated: boolVal(op["deprecated"]),
	}

	seen := map[string]bool{}
	addParam := func(p map[string]any) {
		if ref := str(p["$ref"]); ref != "" {
			if resolved := mapOf(paramComps[refName(ref)]); resolved != nil {
				p = resolved
			}
		}
		name := str(p["name"])
		in := str(p["in"])
		if name == "" || in == "" {
			return
		}
		key := in + ":" + name
		if seen[key] {
			return
		}
		seen[key] = true
		f := Field{Name: name, In: in, Required: boolVal(p["required"]), Type: schemaType(mapOf(p["schema"]), comps)}
		switch in {
		case "path":
			result.PathParams = append(result.PathParams, f)
		case "query":
			result.Query = append(result.Query, f)
		}
	}
	for _, rp := range asMaps(op["parameters"]) {
		addParam(rp)
	}
	for _, rp := range asMaps(sharedParams) {
		addParam(rp)
	}
	sortFields(result.PathParams)
	sortFields(result.Query)

	result.Body = bodyFields(op["requestBody"], comps, bodyComps)
	result.Response = responseFields(op["responses"], comps, respComps)
	return result
}

// bodyFields flattens the JSON request-body schema's top-level properties. A
// requestBody may itself be a $ref into components.requestBodies; resolve it
// first.
func bodyFields(rawBody any, comps, bodyComps map[string]any) []Field {
	body, ok := rawBody.(map[string]any)
	if !ok {
		return nil
	}
	if ref := str(body["$ref"]); ref != "" {
		if resolved := mapOf(bodyComps[refName(ref)]); resolved != nil {
			body = resolved
		}
	}
	schema := jsonSchema(body["content"])
	if schema == nil {
		return nil
	}
	return flatten(schema, comps)
}

// responseFields flattens the first available success response's JSON
// schema. Figma responses are usually a $ref into components.responses, so
// each candidate is dereferenced before its content is read.
func responseFields(rawResponses any, comps, respComps map[string]any) []Field {
	responses, ok := rawResponses.(map[string]any)
	if !ok {
		return nil
	}
	for _, code := range successCodes(responses) {
		resp := mapOf(responses[code])
		if resp == nil {
			continue
		}
		if ref := str(resp["$ref"]); ref != "" {
			if resolved := mapOf(respComps[refName(ref)]); resolved != nil {
				resp = resolved
			}
		}
		if schema := jsonSchema(resp["content"]); schema != nil {
			return flatten(schema, comps)
		}
	}
	return nil
}

// successCodes returns the 2xx response codes present, 200/201 first.
func successCodes(responses map[string]any) []string {
	var out []string
	for _, preferred := range []string{"200", "201"} {
		if _, ok := responses[preferred]; ok {
			out = append(out, preferred)
		}
	}
	var rest []string
	for code := range responses {
		if strings.HasPrefix(code, "2") && code != "200" && code != "201" {
			rest = append(rest, code)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// jsonSchema returns the application/json schema node from a content map.
func jsonSchema(rawContent any) map[string]any {
	content, ok := rawContent.(map[string]any)
	if !ok {
		return nil
	}
	media, ok := content["application/json"].(map[string]any)
	if !ok {
		return nil
	}
	return mapOf(media["schema"])
}

// flatten resolves a schema (following one $ref and merging allOf) and
// returns its top-level properties as sorted Fields.
func flatten(schema map[string]any, comps map[string]any) []Field {
	props, required := properties(schema, comps, map[string]bool{})
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Field, 0, len(names))
	for _, name := range names {
		out = append(out, Field{
			Name:     name,
			Type:     schemaType(props[name], comps),
			Required: required[name],
		})
	}
	return out
}

// properties collects a schema's top-level properties and required set,
// dereferencing a single $ref and merging allOf members. seen guards against
// $ref cycles.
func properties(schema map[string]any, comps map[string]any, seen map[string]bool) (map[string]map[string]any, map[string]bool) {
	props := map[string]map[string]any{}
	required := map[string]bool{}
	if schema == nil {
		return props, required
	}
	if ref := str(schema["$ref"]); ref != "" {
		name := refName(ref)
		if seen[name] {
			return props, required
		}
		seen[name] = true
		return properties(mapOf(comps[name]), comps, seen)
	}
	for _, r := range asStrings(schema["required"]) {
		required[r] = true
	}
	if p, ok := schema["properties"].(map[string]any); ok {
		for name, v := range p {
			props[name] = mapOf(v)
		}
	}
	for _, member := range asMaps(schema["allOf"]) {
		mp, mr := properties(member, comps, seen)
		for name, v := range mp {
			props[name] = v
		}
		for name := range mr {
			required[name] = true
		}
	}
	return props, required
}

// schemaType renders a field schema's type: a referenced component name, an
// "X[]" array notation, or the scalar type/format. OpenAPI 3.1 allows a
// JSON-Schema array type (["string","null"]); typeName reduces it to its
// first concrete member.
func schemaType(schema map[string]any, comps map[string]any) string {
	if schema == nil {
		return ""
	}
	if ref := str(schema["$ref"]); ref != "" {
		return refName(ref)
	}
	if typeName(schema["type"]) == "array" {
		inner := schemaType(mapOf(schema["items"]), comps)
		if inner == "" {
			return "array"
		}
		return inner + "[]"
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		if members := asMaps(schema[key]); len(members) == 1 {
			if ref := str(members[0]["$ref"]); ref != "" {
				return refName(ref)
			}
		}
	}
	if format := str(schema["format"]); format != "" {
		return format
	}
	return typeName(schema["type"])
}

// typeName reduces an OpenAPI type to a single scalar name. In 3.1 a type may
// be a list (e.g. ["string","null"]); the first non-null member is used.
func typeName(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}

// collectTypes records the definition of every component schema referenced by
// the operations' fields, transitively, guarding against cycles.
func collectTypes(ops []Operation, comps map[string]any, types map[string][]Field) {
	var queue []string
	enqueue := func(fields []Field) {
		for _, f := range fields {
			base := strings.TrimSuffix(f.Type, "[]")
			if _, ok := comps[base]; ok {
				queue = append(queue, base)
			}
		}
	}
	for _, op := range ops {
		enqueue(op.PathParams)
		enqueue(op.Query)
		enqueue(op.Body)
		enqueue(op.Response)
	}
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, done := types[name]; done {
			continue
		}
		comp, ok := comps[name].(map[string]any)
		if !ok {
			continue
		}
		fields := flatten(comp, comps)
		types[name] = fields
		enqueue(fields)
	}
}

// basePath returns the server URL's path portion (scheme+host stripped),
// which is prepended to every operation path. Figma's single server is
// https://api.figma.com, so the base path is empty.
func basePath(spec map[string]any) string {
	servers, ok := spec["servers"].([]any)
	if !ok || len(servers) == 0 {
		return ""
	}
	server, ok := servers[0].(map[string]any)
	if !ok {
		return ""
	}
	u := str(server["url"])
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "//")
	if i := strings.Index(u, "/"); i >= 0 {
		return strings.TrimRight(u[i:], "/")
	}
	return ""
}

// namedComponents returns one components sub-map (schemas, parameters,
// requestBodies, or responses), keyed by component name, or an empty map when
// absent.
func namedComponents(spec map[string]any, kind string) map[string]any {
	c, _ := spec["components"].(map[string]any)
	if c == nil {
		return map[string]any{}
	}
	m, _ := c[kind].(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

// sortedKeys returns a map's keys in sorted order, for deterministic
// iteration over spec paths and path-item methods.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func asMaps(raw any) []map[string]any {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asStrings(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapOf(raw any) map[string]any {
	m, _ := raw.(map[string]any)
	return m
}

func str(raw any) string {
	s, _ := raw.(string)
	return s
}

func boolVal(raw any) bool {
	b, _ := raw.(bool)
	return b
}

func sortFields(fields []Field) {
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
}

func readSpec(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec map[string]any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// writeJSON writes v as indented, newline-terminated JSON. HTML escaping is
// disabled so paths and descriptions stay readable.
func writeJSON(path string, v any) error {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// repoRoot walks up from the working directory to the module root (the
// directory containing go.mod), so the tool works regardless of where
// `go run` is invoked from.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "build-catalog:", err)
	os.Exit(1)
}
