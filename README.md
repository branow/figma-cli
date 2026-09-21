# figma-cli

A single, scriptable command-line interface for the [Figma REST API](https://developers.figma.com/docs/rest-api/).
`figma` gives you curated commands for everyday work — parse a pasted Figma
URL, render nodes and download the assets to disk, list comments, projects,
components, and more — plus a generic `figma api` escape hatch that can invoke
any of the 54 operations in Figma's published OpenAPI spec by operationId.

It is the same class of tool as `gh`, built on a token stored in your OS
keychain, JSON output, meaningful exit codes, and shell completion.

## Install

```sh
# Homebrew (macOS / Linux)
brew install branow/tap/figma-cli

# Scoop (Windows)
scoop bucket add branow https://github.com/branow/scoop-bucket
scoop install figma

# Go
go install github.com/branow/figma-cli@latest   # installs as "figma-cli"
```

Or download a prebuilt binary for your platform from the
[latest release](https://github.com/branow/figma-cli/releases/latest).

macOS note: release binaries are not Apple-notarized. The Homebrew cask strips
the quarantine attribute automatically; for a browser-downloaded archive run
`xattr -d com.apple.quarantine ./figma` once.

## Authenticate

Create a personal access token at Figma → Settings → Security → Personal
access tokens, then:

```sh
# Interactive (prompts for the token with hidden input)
figma auth login

# Non-interactive / CI
echo "$FIGMA_TOKEN" | figma auth login --token-stdin

figma auth status          # who am I, is the token valid
figma auth switch -p work  # change the active profile
figma auth logout
```

The token is stored in the OS keychain (macOS Keychain, libsecret, or Windows
Credential Manager) — never in a config file, command output, logs, or error
messages. When no keychain backend exists at all, it falls back to a `0600`
file with a warning.

## Everyday commands

```sh
figma me                                   # the authenticated user

# Files — accept a bare key or a pasted Figma URL
figma file get   abc123
figma file nodes "https://www.figma.com/design/abc123/Design?node-id=1-2"
figma file meta  abc123
figma file versions abc123

# Images — resolve render URLs, or download the assets to disk
figma image render abc123 --ids 1-2,3-4                    # print URLs
figma image render abc123 --ids 1-2 --format svg --out ./assets
figma image render abc123 --ids 1-2 --out ./logo.png       # single node -> a file
figma image fills abc123

# Collaboration and org — team/project/folder ids or pasted URLs both work
figma comment list abc123 --limit 20
figma comment add  abc123 --message "Looks great"
figma project list "https://www.figma.com/files/team/1234567890/My-Team"
figma project files "https://www.figma.com/files/project/55391681/My-Project"
figma component list --file abc123
figma style list --team 1234567890

# Automation
figma webhook list --team 1234567890
figma variable list abc123 --published
figma dev-resource list abc123
figma analytics component actions abc123 --group-by component
```

Node ids are accepted in either spelling — the URL form `1-2` or the API form
`1:2` — and normalized for you. The team/project/folder commands accept either a
bare numeric id or a pasted `figma.com/files/…` URL, the same way the file
commands accept a bare key or a file URL.

List and history commands (`comment list`, `file versions`, `analytics`) take
`--limit N`; `file versions` and `analytics` also take `--paginate` to follow
every page to the end. Mutations print a one-line confirmation in table mode
(and the raw body under `-o json`); a batch write whose 200 response reports
per-item errors exits non-zero so a script can detect it.

## The generic `api` command

Every operation in the spec is reachable generically, no bespoke subcommand
required:

```sh
figma api --list                       # all 54 operationIds
figma api getFile --describe           # method, path, and fields — no network
figma api getFile -f file_key=abc123 -f depth=2
figma api postComment -f file_key=abc123 -f message="Hi"
figma api postDevResources --input body.json
```

`-f/--field` values parse as JSON when they look like it (numbers, booleans,
arrays, objects) and as plain strings otherwise; `--input` reads a full nested
JSON body from a file or stdin.

## Scripting

- `-o json` on any command emits the raw, unmodified API response.
- Exit codes are stable and meaningful:

  | Code | Meaning |
  |------|---------|
  | 0 | success |
  | 1 | generic / API business error |
  | 2 | cancelled |
  | 3 | validation (bad flags, unknown command/operation, invalid enum value) |
  | 4 | auth (401/403) |
  | 5 | not found (404) |
  | 6 | rate-limited (429, or a 5xx with Retry-After) |

- `--no-input` disables prompts; `--quiet` suppresses non-essential output.
- `figma completion bash|zsh|fish|powershell` generates a completion script.

## Configuration

Non-secret preferences live in `~/.config/figma/config.yml`
(`$XDG_CONFIG_HOME/figma` or `%AppData%\figma` when set). Values resolve with
precedence flag > `FIGMA_*` env var > config file > built-in default.

```sh
figma config set output json
figma config set team_id 1234567890
figma config list
figma config unset output        # revert to the built-in default
```

Relevant environment variables: `FIGMA_TOKEN` (a token source for
`auth login`), `FIGMA_PROFILE`, `FIGMA_OUTPUT`, `FIGMA_TEAM_ID`.

## Development

```sh
make catalog   # regenerate the embedded catalog from specs/figma-openapi.yaml
make build     # build ./figma
make test      # run the tests
make check     # catalog-current + build + vet + test + gofmt gate
```

The client is schema-agnostic: all endpoint knowledge lives in the generated
catalog (`internal/figmaapi/catalog/figma-catalog.json`), distilled from the
OpenAPI spec by `scripts/build-catalog` and embedded with `go:embed`. See
[docs/DESIGN.md](docs/DESIGN.md) for the architecture.

## License

MIT — see [LICENSE](LICENSE). Not affiliated with or endorsed by Figma, Inc.
