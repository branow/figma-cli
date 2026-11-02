# figma-cli — design of record

`figma-cli` is one scriptable Go binary (`figma`) for the Figma REST API, in
the mould of `gh`. It pairs a hand-written **curated** command layer (the
commands that do what raw `curl` cannot — parse a Figma URL, download rendered
assets, tabulate a comment thread) with a **generic** `figma api` escape hatch
that reaches every operation in Figma's OpenAPI spec by operationId, backed by
a catalog distilled from that spec at build time.

## Layering

```
main.go                      err := cmd.Execute(); print; os.Exit(cmdutil.ExitCode(err))
cmd/                         cobra tree + Factory DI. Flag parse -> call internal/. No business logic.
internal/
  cmdutil/                   Factory (DI seam), typed errors, ExitCode
  config/                    non-secret profiles; flag > FIGMA_* env > file > default
  credentials/               Store: OS keychain + guarded 0600 fallback. Secrets ONLY here.
  iostreams/                 tty/color/no-input detection; System() and Test()
  output/                    json + table renderers
  figmaapi/                  HTTP client (header-token auth, retry, response/error parsing), Route
    catalog/                 embedded catalog + Lookup (go:embed figma-catalog.json)
  figmaurl/                  file_key + node-id extraction; node-id 1-2 <-> 1:2
  figma/                     curated models, response decoders, asset downloader
specs/figma-openapi.yaml     the OpenAPI spec (catalog input)
scripts/build-catalog/       distills the spec -> catalog JSON
```

Dependencies point inward and down: `cmd` → `internal/*`; `figmaapi` →
`catalog` + `credentials`; nothing in `internal/` imports `cmd`. There are no
cycles. Modules trust each other — validation happens once, at the boundary
(flag parsing, API responses), and internal callers are not re-checked.

### The Factory

`cmdutil.Factory` is the single DI seam: it carries `IOStreams`, `Config`,
`CredentialsStore`, a `ClientFn func() (figmaapi.Client, error)`, and the
resolved global flags (`Profile`, `Output`, `NoInput`, `Quiet`, `Force`).
`newRealFactory()` builds the production wiring; tests build a Factory with
`iostreams.Test()` and a fake `figmaapi.Client`, so every command is
exercisable end-to-end without a network, filesystem, or keychain.

## Authentication

Figma uses a header token, not HTTP Basic. The CLI authenticates with a
personal access token; the stored shape keeps a `Kind` seam so an OAuth
transport could be added later without a migration:

```go
type Credentials struct {
    Kind         string // "pat" | "oauth"
    Token        string
    RefreshToken string
    ExpiresAt    string // RFC3339, oauth only
}
```

- **Personal / plan access token** (`Kind: "pat"`) — sent as `X-Figma-Token`.
  This is the v1 default; the same transport serves plan/org admin tokens.
- **OAuth2** (`Kind: "oauth"`) — sent as `Authorization: Bearer`. Not
  offered by the CLI: a PAT is simpler for single-user terminal use and needs
  no registered app. The store and header selection already handle the kind,
  so an authorization-code flow could be added later without a migration.

`figma auth login` reads the token from `--token-stdin`, then `FIGMA_TOKEN`,
then a hidden terminal prompt. Before storing, it verifies against
`GET /v1/me`: only a 401/403 blocks the login (with `--no-verify` to override);
an unreachable host or a 5xx is inconclusive and stores the token with a
warning, so a transient outage never locks out a valid token.

Secrets live only in `credentials/`, stored in the OS keychain through
`branow/gokey`: macOS Keychain via the Security framework called directly
(loaded at run time with `ebitengine/purego`, so no cgo and no loss of
cross-compilation), Windows Credential Manager, Linux Secret Service. Calling
the framework rather than shelling out to `/usr/bin/security` is what keeps
the item's access control naming this binary instead of that tool, which would
leave the token readable by any process running as the user.
`internal/credentials` keeps the policy around it:

- The plaintext fallback (0600) is **guarded**: it is used only when `gokey`
  reports no credential store on the machine (`gokey.ErrUnavailable`), with a
  warning at write time. A keychain that is present but fails for any other
  reason surfaces that failure rather than silently writing the token to disk.
- An entry the keychain refuses to release (`gokey.ErrBlocked`) fails with a
  remedy naming the profile and
  `security delete-generic-password -s figma-cli -a <profile>`. Only
  `credentials.ErrNotFound` means "not logged in"; a blocked entry exits 4.
- Every read is silent first. Only when the keychain refuses it, and a
  terminal can show the dialog (`IOStreams.CanPrompt()`), is it repeated with
  the dialog allowed; an unattended run fails with the remedy instead of
  hanging on a dialog nobody can see. An entry released that way belongs to
  another build and is stored again by this one, so the next run is silent.
- Entries written by the `go-keyring` versions carry the permissive access
  control, are owned by `/usr/bin/security`, and store the value as
  `go-keyring-base64:<base64>` (`go-keyring-encoded:<hex>` before v0.2). The
  first read after approval decodes that layout and stores the value again as
  plain JSON. The keychain refuses to let this binary replace an item another
  program owns (OSStatus -25244), so on exactly that refusal the old item is
  deleted through `/usr/bin/security` (delete only, no secret passes through
  it) and added again under this binary's own identity. `auth login` over an
  old entry takes the same path. A rewrite that still fails is a warning, not
  an error: the read succeeds and the dialog keeps coming back.
- Release darwin binaries are signed with a self-signed release certificate
  (`scripts/sign.sh`, identifier `com.branow.figma`, run by goreleaser on a
  macOS runner). That keeps the item's access list valid across upgrades.
  Without an Apple Team ID macOS still partitions the item by cdhash, so the
  first read after each upgrade asks once per profile, and the re-save above
  makes it once.

## Profiles and configuration

A profile is a keychain credential plus non-secret config (`output`, an
optional default `team_id`, and the handle/email resolved at login). There is
no per-profile host — Figma has one. Precedence per field is
flag > `FIGMA_*` env > config file > default; an invalid value in one source
is skipped, not fatal, so a stale env var never leaves the CLI unable to
render. Config is `~/.config/figma/config.yml` (XDG / `%AppData%` aware).

## The HTTP client

`figmaapi.Client` is an interface (`Call` + lower-level `Do`) so commands test
against a fake. The real `HTTPClient` targets the fixed host
`https://api.figma.com` (overridable via `FIGMA_BASE_URL`, which refuses
cleartext `http://` unless `FIGMA_INSECURE=1`).

`Route(op, fields)` is the single place operation → request lives, shared by
`Call` and the `api` command: it substitutes path params (case-insensitive by
name), turns the rest into query parameters for GET/DELETE or a JSON body
otherwise, and canonicalizes field names to their documented casing. A JSON
array given to a query parameter is joined with commas (`ids=1:2,3:4`) — the
form Figma's scalar list parameters parse — unless the catalog documents that
parameter as an array type, in which case it expands into repeated keys. Every
Figma list-style query parameter is a comma-joined string, so sending repeated
keys instead made Figma receive an array it then tried to `.split`; comma
joining is the correct default.

Retry policy: a 30s timeout, `maxAttempts=4`. GET retries on any transport
error or a retryable status; mutations retry only on 429 (the server refused
before processing, so it cannot have partially applied). `Retry-After` is
honored, else exponential backoff with jitter.

Responses: a 2xx body decodes into `Response{Status, Body, Fields, Raw}` (a
top-level array is wrapped as `{"values": [...]}` for the generic command;
`Raw` is kept for typed decode). A non-2xx becomes `*APIError`. Figma error
bodies are `{"status": int, "err": string}`, so message extraction reads `err`
first, then `message`/`error`, then the trimmed raw text.

## Errors and exit codes

Errors are typed, never string-matched. `cmdutil.ExitCode` is the one place
they become process codes:

| Code | Trigger |
|------|---------|
| 0 | success |
| 1 | generic / API business error |
| 2 | `ErrCancelled` |
| 3 | `ValidationError`, unknown/unsupported operation |
| 4 | `ErrNotLoggedIn`, `gokey.ErrBlocked` (keychain entry awaiting approval), API 401/403 |
| 5 | API 404 |
| 6 | API 429, or a 5xx flagged `RateLimited` (a Retry-After) |

`RateLimited` is reserved for genuine throttling so a permanent 5xx does not
make a script branching on code 6 retry forever.

Cobra's own usage errors — an unknown command, an unknown subcommand, an
invalid enum argument, a bad flag — are wrapped as `ValidationError` at the
command tree's edge (`wireValidationErrors`) so they map to exit 3 like the
rest of the validation surface rather than leaking Cobra's default exit 1.
A batch mutation whose HTTP 200 body carries a populated `errors`
array (Figma's dev-resource/variable batch endpoints report per-item failures
this way) becomes a `figma.BatchError` → exit 1, so a silent success can no
longer slip past a script — the curated commands and the generic `api` command
both run the body through `figma.CheckBatchErrors`, so a batch op invoked either
way reports the failure identically. `cmdutil.Explain` rewrites Figma's raw 403
scope dump into a one-line hint naming the missing scope, without altering the
exit code computed from the original error.

## The catalog and `figma api`

```
specs/figma-openapi.yaml --(scripts/build-catalog)--> figma-catalog.json --(go:embed)--> catalog.Lookup(id)
```

The builder is deliberately dumb: it reads the YAML generically, walks
paths → methods → operations, and records each operation's method, full
post-host path, path/query params, flattened top-level request-body and
success-response properties, plus a registry of referenced component schemas
(one `$ref` level followed, transitively closed with a cycle guard). It
resolves the `components/responses` and `components/requestBodies` indirections
Figma's spec uses, and handles OpenAPI 3.1's array-typed `type`. Figma is a
single product, so operationIds are unique and `catalog.Lookup` returns the
single match — no product namespacing.

`figma api` uses that catalog to route any operationId: `--list` prints all 54
ids, `--describe` documents an operation's fields (with nested types expanded)
entirely offline, and a call sends `-f k=v` fields or an `--input` JSON body.
The client stays schema-agnostic; all Figma knowledge lives in the generated
JSON. This is why we never hand-write 54 command trees — the catalog gives the
boring 90% for free, and the curated commands add value on top (URL parsing,
asset download, pagination, friendly tables).

## Curated value

The pieces raw API access does not give you:

- **`figmaurl`** — pull the file key and node id out of any pasted Figma URL,
  and normalize a node id between the URL form (`1-2`) and the API form
  (`1:2`). The file/image/comment/dev-resource commands accept a URL or a bare
  key interchangeably; the team/project/folder commands likewise accept a
  `figma.com/files/team|project|folder/<id>/…` URL or a bare id, closing the
  discoverability gap around ids Figma exposes no "list my teams" endpoint for.
- **`figma image render --out`** — `getImages` returns short-lived S3 URLs;
  the command downloads each rendered node to disk (named by node id, with the
  format's extension) rather than leaving you to `curl` an expiring URL. `--out`
  may name a directory or, for a single node, a file path; downloads run up to
  `--concurrency` at a time and end with a `Downloaded N/M` summary.
- **Pagination** — `comment list`, `file versions`, and `analytics` take
  `--limit`; the paged endpoints (`file versions`, `analytics`) also take
  `--paginate` to follow every cursor to the end. The page-following loop
  (`figma.Paginate`) is pure over a fetch callback so it is table-testable.
- **Friendly tables and confirmations** — list commands decode the small
  subset of each response worth tabulating, and every mutation prints a
  one-line confirmation in table mode; `-o json` always falls back to the raw
  body.

## Testing

Command/E2E tests build a Factory with `iostreams.Test()` and a fake or
`httptest`-backed client, run the real cobra command, and assert on the
buffers and the exit classification. Logic-dense units are tested directly:
`Route`, response/error parsing, retry/backoff, config precedence, the guarded
credential fallback, `figmaurl` parsing and node-id normalization, the asset
downloader, and the catalog against the real embedded JSON. No test makes a
live API call. In `internal/credentials` the `gokey` calls are struct fields,
so the fallback and authorization branches are driven by an in-memory stand-in
— `go test ./...` never touches a real keychain; the round trip against the
live OS credential store lives in `gokey` itself, behind its `integration`
build tag.

## Distribution

`goreleaser` builds `CGO_ENABLED=0` binaries for darwin/linux/windows ×
amd64/arm64, signs the darwin builds with the release certificate
(`scripts/sign.sh`, so the release runs on `macos-latest` and needs the
`MACOS_SIGN_P12`, `MACOS_SIGN_P12_PASSWORD` and `MACOS_SIGN_IDENTITY`
secrets), publishes a Homebrew cask to `branow/homebrew-tap` (with a
quarantine-strip post-install hook, since the binaries are not notarized), a
Scoop manifest to `branow/scoop-bucket`, and deb/rpm/apk via nfpm. CI gates on
gofmt, `go vet`, tests, a catalog-is-current check, and a cross-compile.
