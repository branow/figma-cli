// Package credentials stores and retrieves Figma login secrets, backed by
// the OS keychain with a plaintext-file fallback. It is kept separate from
// internal/config so secrets never land in the preferences file.
package credentials

import "errors"

// Credentials are the values needed to authenticate against the Figma REST
// API. Kind selects the auth transport: a "pat" (a personal or plan access
// token, sent as the X-Figma-Token header) or an "oauth" access token (sent
// as Authorization: Bearer, with a refresh token and expiry). v1 writes
// only "pat"; the shape carries the OAuth fields so adding the OAuth flow
// later needs no stored-credential migration.
type Credentials struct {
	Kind         string `json:"kind"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// Store persists and retrieves Credentials per named profile.
type Store interface {
	Get(profile string) (Credentials, error)
	Set(profile string, creds Credentials) error
	Delete(profile string) error
}

// ErrNotFound indicates no credentials are stored for the requested
// profile.
var ErrNotFound = errors.New("no credentials stored for profile")
