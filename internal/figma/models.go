// Package figma holds the small typed models and helpers the curated
// commands share: the subset of each response they render as a table, a
// decoder for the named array most Figma list endpoints wrap their results
// in, and the asset downloader behind "figma image render". The bulk of the
// API stays reachable generically via the catalog; this package carries only
// the shapes that earn a friendlier rendering.
package figma

import "encoding/json"

// User is the minimal author shape embedded in comments and versions.
type User struct {
	Handle string `json:"handle"`
	Email  string `json:"email"`
}

// Version is one entry of getFileVersions.
type Version struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	Label       string `json:"label"`
	Description string `json:"description"`
	User        User   `json:"user"`
}

// Comment is one entry of getComments.
type Comment struct {
	ID         string `json:"id"`
	Message    string `json:"message"`
	User       User   `json:"user"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at"`
	OrderID    string `json:"order_id"`
}

// Project is one entry of getTeamProjects.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// File is one entry of getProjectFiles / getFolderFiles.
type File struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	LastModified string `json:"last_modified"`
}

// Component is one entry of the components list endpoints.
type Component struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Style is one entry of the styles list endpoints.
type Style struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	StyleType   string `json:"style_type"`
	Description string `json:"description"`
}

// Webhook is one entry of the webhook list endpoints.
type Webhook struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Status    string `json:"status"`
	Endpoint  string `json:"endpoint"`
}

// DevResource is one entry of getDevResources.
type DevResource struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	FileKey string `json:"file_key"`
	NodeID  string `json:"node_id"`
}

// DecodeArray pulls a named array out of a Figma response body and decodes it
// into a typed slice. Most list endpoints wrap their results in a single
// top-level key (comments, versions, projects, ...); this centralizes that
// unwrap so each list command stays a one-liner.
func DecodeArray[T any](raw []byte, key string) ([]T, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	out := []T{}
	if inner, ok := envelope[key]; ok {
		if err := json.Unmarshal(inner, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DecodeMetaArray pulls a named array nested under a top-level "meta" object,
// the shape the component and style list endpoints use
// (meta.components, meta.styles).
func DecodeMetaArray[T any](raw []byte, key string) ([]T, error) {
	var envelope struct {
		Meta map[string]json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	out := []T{}
	if inner, ok := envelope.Meta[key]; ok {
		if err := json.Unmarshal(inner, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}
