# Contributing to powerschool-go

This guide covers local development, testing against your own
PowerSchool instance, and extending the library to new districts.

## Prerequisites

- Go 1.23 or later.
- Git.
- Chrome or Chromium. The library drives a real browser via
  [chromedp](https://github.com/chromedp/chromedp) for authentication.
- A PowerSchool **parent account** (the library does not currently
  support student or admin roles).
- (Optional) The [1Password CLI](https://developer.1password.com/docs/cli/)
  for keeping credentials out of shell history.

## Getting the code

```bash
git clone https://github.com/leftathome/powerschool-go.git
cd powerschool-go
go mod download
```

## Running tests

### Unit tests (hermetic, no network)

```bash
go test ./...
```

These run against the committed fixture in
[`testdata/assignments_lookup.json`](testdata/assignments_lookup.json)
and an in-process `httptest.Server`. No credentials, no Chrome, no
network. Safe to run in CI.

### Integration tests (live PowerSchool)

The full end-to-end test lives behind a `//go:build integration` tag
and drives a real Chrome against a real PowerSchool instance. It
performs auth, grade retrieval, and assignment lookup, asserting
**shape only** so it survives school-year rollover.

```bash
export POWERSCHOOL_HOST=ps.yourdistrict.example.org
export POWERSCHOOL_USERNAME=parent@example.com
export POWERSCHOOL_PASSWORD='your-password'

go test -tags integration -run TestIntegration -timeout 5m ./...
```

Expect ~60–90 seconds (browser startup + several chromedp page
fetches). If you run as root (WSL, containers), the browser
auto-enables `--no-sandbox`.

### The `test-auth` probe

For quick interactive checking during development:

```bash
POWERSCHOOL_URL="https://$POWERSCHOOL_HOST" \
  go run ./cmd/test-auth -headless -debug
```

It prints the full pipeline — students → grades → assignments for the
first course that returns a non-empty list.

## Managing credentials locally

**Never commit credentials.** Two supported patterns:

### Environment variables (simplest)

```bash
export POWERSCHOOL_HOST=ps.yourdistrict.example.org
export POWERSCHOOL_USERNAME=...
export POWERSCHOOL_PASSWORD=...
```

### 1Password CLI

Store your PowerSchool credentials in a 1Password item and wire up a
local `.env.local` based on [`.env.local.example`](.env.local.example).
Helper scripts in `scripts/` (`test-with-1password.sh`,
`test-with-1password.ps1`) read that file and inject the credentials
into the test-auth run via `op read`.

`.env.local` is gitignored. So is `.schoology-session.json` / any
`*session*.json` you generate via `client.ExportSession()`.

## Fixture management

### Why fixtures

PowerSchool data rotates each school year — assignments disappear,
scores change, section IDs shift. A live-only test would silently
stop running once the data was gone. So the unit test reads a
recorded JSON response from `testdata/assignments_lookup.json` and
serves it via `httptest.Server`, letting the parsing + Referer
contract stay covered forever.

### Redacting captures before they land in the repo

Raw captures contain real student DCIDs, course IDs, and sometimes
names. The `hack/redact` tool does deterministic, longest-first
substring substitution against a **gitignored** config file.

1. Copy the example schema to your local config:

   ```bash
   cp hack/redact.config.example.json hack/redact.config.json
   ```

2. Edit `hack/redact.config.json` to map your real values to stable
   placeholders. Use the example's placeholder conventions (numeric
   IDs like `111111`, `999999`, `654321`; names like "Student Alpha").

3. Run the redactor:

   ```bash
   go run ./hack/redact \
     -in captured-response.json \
     -out testdata/assignments_lookup.json
   ```

See [`testdata/README.md`](testdata/README.md) for the full
fixture-refresh recipe including the `jq` pre-scrub for
`$breach_mitigation` tokens and `studentsdcid` anonymization.

## Supporting a new district

PowerSchool HTML varies between versions and district
customizations. The library was developed against one reference
instance; selectors and behaviors observed there are documented in
[`docs/REFERENCE_STRUCTURE.md`](docs/REFERENCE_STRUCTURE.md).

If your district's PowerSchool doesn't work out of the box:

1. Run `cmd/test-auth` with `-debug` and with `Headless: false` so you
   can watch the browser. The login selector list in
   `internal/browser/browser.go` tries several common patterns; add
   yours if needed.
2. If `GetGrades` returns empty, capture the home-page HTML (the
   `cmd/capture-html` helper can help) and check whether your instance
   also uses `table#tblgrades` with `tr.center` rows.
3. If `GetAssignments` returns an error, capture the XHR sequence from
   your browser's DevTools. Check: does
   `/ws/xte/assignment/lookup` exist on your host, does the scores
   page expose `data-sectionid` and `studentFRN`, and what does
   Referer look like. See
   [`docs/ASSIGNMENTS_API_DEBUG.md`](docs/ASSIGNMENTS_API_DEBUG.md)
   for the three known pitfalls.

Share findings (with PII redacted via `hack/redact`) in a PR or
issue — we'll add them to `docs/REFERENCE_STRUCTURE.md` and the
selector fallback lists.

## Making changes

1. Fork and branch.
2. Add tests. For new behavior, extend `assignments_test.go` or add a
   new `*_test.go`. For behavior that can only be observed live, add
   to `assignments_integration_test.go` (build tag: `integration`)
   with shape-level assertions only.
3. Run `go vet ./...` and `go test ./...` before pushing.
4. If you captured new response data, redact it via `hack/redact`
   before committing.
5. Open a PR. Mention which district/version you tested against in
   the description — it helps future contributors.

## Project layout

```
.
├── *.go                    — public library
├── assignments_test.go     — httptest-backed unit test
├── assignments_integration_test.go — live integration test (build tag)
├── internal/browser/       — chromedp wrapper (not part of the API)
├── cmd/
│   ├── test-auth/          — interactive end-to-end probe
│   ├── capture-html/       — dump raw HTML for a page
│   └── inspect-page/       — stand-alone page inspector
├── hack/redact/            — byte-level PII redactor for fixtures
├── examples/               — short usage examples
├── testdata/               — committed, scrubbed fixtures
└── docs/                   — observed behavior + debug notes
```

## Security reminders

- This library handles sensitive student data. Assume any capture
  you haven't redacted is PII.
- Sessions cookies are equivalent to credentials. `*session*.json`
  and `.schoology-session.json` are gitignored; keep `ExportSession`
  output out of the repo.
- If you need to share a bug reproducer, redact it with
  `hack/redact` first.
