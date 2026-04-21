# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-21

First tagged release. The library drives PowerSchool through a real
browser for authentication and then makes XHR calls the way
PowerSchool's own Angular frontend does, covering the paths a parent
account surfaces: login, students, grades, and per-course assignments.
Verified end-to-end against one reference PowerSchool instance.
See [docs/REFERENCE_STRUCTURE.md](docs/REFERENCE_STRUCTURE.md) for the
observed selectors and URL shapes the parsers rely on, and
[docs/ASSIGNMENTS_API_DEBUG.md](docs/ASSIGNMENTS_API_DEBUG.md) for the
three-bug chain that blocked assignment retrieval before this release.

### Added

#### Authentication

- `NewClient(baseURL, opts...)` with options `WithCredentials`,
  `WithSession`, `WithHTTPClient`, `WithLogLevel`.
- `Client.Authenticate(ctx)` and `AuthenticateWithOptions(ctx, *AuthOptions)`.
- `AuthOptions` fields: `Headless`, `Timeout`, `DebugLog`, `UserDataDir`,
  `UserAgent`, `NoSandbox` (pass `--no-sandbox` to Chrome; required when
  running as root in WSL or containers).
- `internal/browser.GetPageContentWithWait` auto-enables `--no-sandbox`
  on Linux-as-root so follow-up page fetches work in the same
  environments.
- Browser-based login via `chromedp`: navigates to `/public/`, fills
  username/password through multiple selector fallbacks, clicks the
  login button, extracts session cookies.
- `IsAuthenticated` / `GetSessionExpiry` / `RefreshSession` /
  `ClearSession` helpers.
- `Client.ExportSession()` / `Client.ImportSession(*SessionExport)`
  for reusing a warm 24-hour session across runs without re-opening a
  browser.
- `ensureAuthenticated` transparently re-authenticates on every public
  method call when credentials are present and the prior session is
  expired.

#### Resources

- `Student` type with nav ID, official student number, state ID, grade
  level, school, and portal username.
- `Client.GetStudents(ctx)` — parses `javascript:switchStudent(ID)`
  from the home-page nav and enriches the currently-displayed
  student's details from `table.student-demo`.
- `Grade` type including per-term grades (`Q1`–`Q4`, `S1`/`S2`),
  attendance counts, teacher, room, period, and cached
  `ScoresURL` / `SectionID` / `StudentAPIID` used by
  `GetAssignments`.
- `Client.GetGrades(ctx, studentID)` — parses `table#tblgrades`
  from the home page, capturing each course's scores-page URL
  (including the `begdate`/`enddate`/`fg`/`schoolid` query params
  required to avoid the login redirect).
- `Client.GetGPA(ctx, studentID)` — heuristic GPA extraction from
  the home page.
- `Assignment` type with `AssignmentFlag` set (`late`, `missing`,
  `collected`, `exempt`, `incomplete`, `absent`) and
  `AssignmentStatus` state.
- `Client.GetAssignments(ctx, studentID, *Grade)` — POSTs to
  `/ws/xte/assignment/lookup` and decodes the full response into
  `[]*Assignment`, including per-assignment category, due date,
  score, percentage, and letter grade.
- `Client.GetAssignmentCategories(ctx, studentID, *Grade)` —
  aggregates `GetAssignments` output into category totals.
- `ScoresMetadata` type + `Client.FetchScoresMetadata(ctx, scoresURL)`
  — extracts both `SectionID` (from
  `div[data-pss-student-assignment-scores][data-sectionid]`) and
  `StudentAPIID` (from `studentFRN` in the inline `data-ng-init`
  expression, with the `001` prefix stripped) in one round trip.
- `Client.FetchSectionID(ctx, scoresURL)` — thin wrapper kept for
  call sites that only need the section ID.
- `GradeHistory` type + `Client.GetGradeHistory(ctx, studentID)` —
  parses `/guardian/termgrades.html`.
- `ProgressReport` type + `Client.GetProgressReports(ctx, studentID)`
  — targets the separate progress-reports host with the shared
  session cookies.

#### Errors

- `*Error` with `Code` / `Op` / `Message` / `Err` fields.
- Sentinel errors: `ErrAuthFailed`, `ErrSessionExpired`, `ErrNotFound`,
  `ErrRateLimited`, `ErrInvalidCredentials`, `ErrNoCredentials`,
  `ErrInvalidBaseURL`, `ErrParseError`, `ErrNetworkError`.
- Typed helpers: `IsAuthError`, `IsSessionExpired`, `IsNotFound`,
  `IsRateLimited`.
- Wrap functions: `WrapAuthError`, `WrapNetworkError`, `WrapParseError`.

#### Logger

- Leveled logger (`LogLevelNone` / `Error` / `Warn` / `Info` / `Debug`)
  wired through every public call; `WithLogLevel` sets it at client
  construction.

#### Tooling

- `hack/redact` CLI — byte-level substring redactor with longest-first,
  deterministic, idempotence-guarded substitution (ported from
  schoology-go). Driven by a gitignored `hack/redact.config.json`;
  the schema lives in `hack/redact.config.example.json`.
- `cmd/test-auth` — interactive end-to-end probe (`-headless`,
  `-debug`, `-no-sandbox` auto-enabled when running as root).
- `cmd/capture-html`, `cmd/inspect-page` — debug helpers for
  dumping rendered HTML and inspecting individual pages.
- `scripts/test-with-1password.sh` and `.ps1` — optional helpers that
  load credentials from a gitignored `.env.local` via `op read`.

#### Tests + docs

- `assignments_test.go` — `httptest.Server`-backed unit test that
  drives `GetAssignments` against the committed fixture, asserts the
  outbound Referer (the stub returns 400 + HTML when it's missing to
  mimic the real server), and verifies flag/category decoding across
  `missing`, `collected`, and `exempt` states.
- `assignments_integration_test.go` (`//go:build integration`) —
  live end-to-end pipeline gated on
  `POWERSCHOOL_HOST` / `POWERSCHOOL_USERNAME` / `POWERSCHOOL_PASSWORD`
  with shape-level assertions only, so it survives school-year
  rollover.
- `testdata/assignments_lookup.json` — redacted fixture (35 real
  assignments) capturing the full `/ws/xte/assignment/lookup`
  response shape; `testdata/README.md` documents the refresh recipe.
- [README.md](README.md) — Getting Started for library consumers.
- [CONTRIBUTING.md](CONTRIBUTING.md) — local-dev workflow, running
  unit and integration tests, credential handling, fixture refresh,
  and the playbook for porting to a new district.
- [docs/REFERENCE_STRUCTURE.md](docs/REFERENCE_STRUCTURE.md) —
  observed HTML / URLs / selectors on the reference instance.
- [docs/ASSIGNMENTS_API_DEBUG.md](docs/ASSIGNMENTS_API_DEBUG.md) —
  post-mortem on the three-bug chain that blocked assignments.
- [docs/SECTION_ID_ISSUE.md](docs/SECTION_ID_ISSUE.md),
  [docs/API_ENDPOINTS.md](docs/API_ENDPOINTS.md),
  [docs/FINDING_API_ENDPOINTS.md](docs/FINDING_API_ENDPOINTS.md) —
  investigation notes that fed the final design.

### Known limitations

- **Multi-student accounts**: `GetGrades` / `GetAssignments` return
  data for whichever student the home page is currently displaying.
  The `switchStudent(ID)` JavaScript hook is identified but not yet
  invoked programmatically.
- **Session establishment**: authentication requires a visible or
  headless Chrome instance. Raw HTTP re-auth (posting against the
  login form) is not supported.
- **Page-fetch overhead**: `getPageHTMLWithWait` spins up a fresh
  chromedp context per call (~6 s). The `data-sectionid` needed by
  `FetchScoresMetadata` lives in the raw HTML response, so a plain
  HTTP path would be a straightforward future optimization.
- **Attendance** and **calendar/events** are not implemented.
- **GPA** parsing is a heuristic; may not work on every district.
- `GetAssignments` accepts a `studentID` parameter but the effective
  ID comes from the scores page's `studentFRN`. The parameter is
  retained for API symmetry until a v0.2 cleanup.
- Verified against one reference PowerSchool instance only. Selector
  fallbacks exist for the login flow, but district-specific
  customizations (custom login portals, SSO-only setups, non-standard
  table structures) may require adjustments. See
  [CONTRIBUTING.md](CONTRIBUTING.md#supporting-a-new-district).

[0.1.0]: https://github.com/leftathome/powerschool-go/releases/tag/v0.1.0
