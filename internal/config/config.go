// Package config manages figma's non-secret preferences: named profiles
// (output format and an optional default team) and which one is active,
// resolved with flag > FIGMA_* env var > config file > built-in default
// precedence. Credentials never live here; see internal/credentials.
package config

import "os"

// Profile holds the non-secret settings for one named profile. Figma has a
// single host and header-token auth, so a profile carries no site and no
// token — only display preferences and an optional default team id used by
// team-scoped commands. Handle and Email are the identity resolved from the
// login probe, mirrored here so "auth status" can show who a profile is
// without unlocking the OS keychain.
type Profile struct {
	Output string `yaml:"output"`
	TeamID string `yaml:"team_id,omitempty"`
	Handle string `yaml:"handle,omitempty"`
	Email  string `yaml:"email,omitempty"`
}

// Config is the on-disk preferences file: the active profile name and the
// set of named profiles.
type Config struct {
	CurrentProfileName string             `yaml:"current_profile"`
	Profiles           map[string]Profile `yaml:"profiles"`
}

const (
	defaultProfileName = "default"
	defaultOutput      = "table"
)

// New returns an empty Config, as used when no config file exists yet.
func New() *Config {
	return &Config{Profiles: map[string]Profile{}}
}

// CurrentProfile resolves the active profile name: flagOverride (if
// non-empty), then FIGMA_PROFILE, then the config file's stored value, then
// "default".
func (c *Config) CurrentProfile(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("FIGMA_PROFILE"); v != "" {
		return v
	}
	if c.CurrentProfileName != "" {
		return c.CurrentProfileName
	}
	return defaultProfileName
}

// SetProfile stores profile as the named profile's settings. It never
// changes which profile is active — activation is a separate, explicit step
// via SwitchProfile.
func (c *Config) SetProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = profile
}

// ProfileOutput returns the output format stored for a profile, or "" when
// the profile is absent or has no explicit preference. It lets a caller
// persist a profile while leaving its output preference untouched.
func (c *Config) ProfileOutput(name string) string {
	return c.Profiles[name].Output
}

// SwitchProfile makes name the active profile without changing its stored
// settings. It fails if the profile has never been configured.
func (c *Config) SwitchProfile(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return &ProfileNotFoundError{Name: name}
	}
	c.CurrentProfileName = name
	return nil
}

// ProfileNotFoundError reports that a named profile has no stored settings.
type ProfileNotFoundError struct{ Name string }

func (e *ProfileNotFoundError) Error() string {
	return "profile not found: " + e.Name
}

// TeamID resolves the default team id for profile: flagOverride, then
// FIGMA_TEAM_ID, then the config file's stored value. Team-scoped commands
// use it so a user need not repeat --team on every call.
func (c *Config) TeamID(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("FIGMA_TEAM_ID"); v != "" {
		return v
	}
	return c.Profiles[profile].TeamID
}

// OutputFormat resolves the output format for profile: flagOverride, then
// FIGMA_OUTPUT, then the config file's stored value, then "table". A source
// holding an unrecognized value is skipped rather than silently disabling
// table rendering, so a stale env var or a hand-edited config never leaves
// the CLI in a no-output state.
func (c *Config) OutputFormat(flagOverride, profile string) string {
	if validOutput(flagOverride) {
		return flagOverride
	}
	if v := os.Getenv("FIGMA_OUTPUT"); validOutput(v) {
		return v
	}
	if p, ok := c.Profiles[profile]; ok && validOutput(p.Output) {
		return p.Output
	}
	return defaultOutput
}

// validOutput reports whether v is a supported output format.
func validOutput(v string) bool {
	return v == "table" || v == "json"
}
