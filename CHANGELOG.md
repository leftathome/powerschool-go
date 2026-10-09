# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-10-09

The three parsers below were run against a live two-student parent account
on 2026-10-09, before and after the hardening pass: every page identified
the right student; attendance returned 16 and 7 course rows with every date
parsed, none on a weekend, across a school year spanning New Year, and every
code present in the page's 18-entry legend; grade history returned rows for
both students; the teacher-comments term was confirmed for the default term
and for an explicitly requested one. One path has no live counterpart yet: a
non-empty teacher comment (none was posted in any term), so comment text
handling is fixture-tested only. Also unverified live: multi-year grade
history and single-student accounts. See
[docs/PAGES_STRUCTURE.md](docs/PAGES_STRUCTURE.md).

### Added

- `GetAttendance` / `ParseAttendancePage`: `/guardian/attendance.html`, the
  per-course grid of one cell per school day, with real dates (the page
  prints no year; it is taken from each row's enrolment dates, else from a
  reference date), the attendance code in each cell, and the legend parsed
  from the page as `AttendanceCode{Code, Category, Description}`. The page
  has no totals; `AttendanceCourse.Count` counts by legend category.
- `GetGradeHistoryPage` / `ParseGradeHistoryPage`: `/guardian/termgrades.html`,
  which is the stored-grade history (`table#grades-tbl`), not a per-quarter
  view.
- `GetTeacherComments` / `ParseTeacherCommentsPage`:
  `/guardian/teachercomments.html`, with the reporting terms the page offers
  and a fetch by term (`?fg=<term>`, GET). The page must mark the requested
  term as selected, else `ErrTermMismatch`.
- `PageStudent`, returned by all three and by `HomePage.Student`: the id the
  page's own student switcher marks as selected. The three parsers fail
  with `ErrSelectionUnknown` when a page marks nobody or more than one.
- `ErrNotImplemented`, `ErrTermMismatch`,
  `HistoricalGrade.DateCompletedText`.

### Hardened after review

- Attendance dates are tied to the grid's weekday columns. A week header
  that starts mid-week ("9/3-9/5") maps onto the matching columns instead
  of shifting the week; the year is the one that puts the header on school
  days and inside the row's enrolment, so a summer-school row and a page
  read with a July reference date no longer land a year off; every returned
  date is checked against its column's weekday. A header that cannot be
  placed, a header column that is not a week range, and course rows with no
  week columns are parse errors, never wrong or missing dates.
- The attendance legend is split on the page's separator elements, so a
  description containing "|" is kept whole.
- Teacher comments: `<br>` inside the comment becomes a line break and
  `<script>` / `<style>` content is dropped.
- Grade history: the not-in-GPA `*` is recognised before the letter as well
  as after it.

### Changed

- `GetGradeHistory` parses the real table instead of guessing at "any table
  with eight columns". With a student id it now selects that student first
  and requires the history page's own switcher to mark that student.
- `GetProgressReports` and `DownloadProgressReport` return
  `ErrNotImplemented` and contact nothing. The former fetched a placeholder
  host and returned an empty list.

## [0.2.0] - 2026-10-09

Several changes below alter behaviour callers could have relied on -- in
particular `GetGrades(ctx, id)` now submits the portal's switch-student form
(an HTTP POST) when `id` is not the selected student, in browser mode as well
as HTTP mode.

Verified against a live two-student parent account on 2026-10-09: HTTP page
fetch with explicitly attached cookies; `SelectStudent` in both directions,
with the page confirming the selection each time; no cookie rotation on
switch; selection is per session (an independent login still saw its own
default); a course's `scores.html` reflects the SESSION's selected student,
not the course link, and carries a distinct id per student. One further
observation, seen once: a second login appeared to invalidate the first
session. See [docs/SWITCH_STUDENT.md](docs/SWITCH_STUDENT.md) for what is
still unconfirmed (single-student accounts, real session lifetime).

### Fixed

- A redirect was read as a dead session whenever the status was any 3xx.
  Only a redirect whose `Location` is under `/public/` (or a 401/403) counts
  now; a 304 or a redirect elsewhere no longer discards a working session.
- **One student's details could be attached to another student.**
  `GetStudents` copied the page-level details (student number, grade level,
  school) onto `students[0]` whenever the page carried no
  `#students-list li.selected` marker. Those details describe whichever
  student the session has selected, so on markup without the marker a
  second child's data was reported under the first child's id. The library
  no longer guesses: only an explicit marker selects a student, and without
  one nobody is enriched.
- **`SwitchStudent` reported success without switching anything**, and
  `GetGrades(ctx, studentID)` ignored `studentID`, so "switch, then read"
  returned the previously selected student's grades under the new id.
  `SwitchStudent` / `SelectStudent` now submit the portal's own switcher
  form (`POST /guardian/home.html`, the single field
  `selected_student_id`), read the home page again and require it to mark
  the requested student as selected (`ErrSwitchNotConfirmed` otherwise;
  `ErrSelectionUnknown` and nothing posted when the page carries no
  marker). `GetGrades` switches first when needed and returns grades only
  from the page that confirmed the selection.
- A dead session answered with HTTP 200 and the sign-in form (no redirect)
  was parsed as a page with no students. Page reads and the assignment
  lookup now report it as a session-expired `*Error`.
- Errors no longer carry response bodies, query strings or student ids.
  The assignment lookup embedded 200 bytes of the response in its error;
  `http.Client` errors carried the full URL with `frn` and `schoolid`.
  Unexpected statuses are reported as `*HTTPStatusError` (operation, status,
  body length).
- INFO log lines no longer contain student ids, course ids or report
  titles.

### Added

- `WithHTTPPageFetch` / `SetHTTPPageFetch` / `UsesHTTPPageFetch`: route
  post-login page reads through the client's `http.Client` as plain GETs
  instead of a headless browser, so a caller's transport sees every request.
  Session cookies are attached to each request explicitly; no cookie jar is
  involved.
- `GetHomePage`: students and grades from a single fetch of the home page.
- `SelectStudent`: switch (if needed), confirm, and return that student's
  home page.
- `Student.Selected`, `Grade.StudentID` (the selected student's id as parsed
  from the grade's own page), `HomePage.SelectedStudent`.
- `AssignmentLookupPath`, `SwitchStudentPath`, `SwitchStudentField`: the two
  requests the library POSTs after login, for callers that allowlist them.

### Changed

- `GetStudents` lists each student once even when the switcher is rendered
  more than once on the page.
- Because a switch changes the session's selected student, calls for
  different students must not run concurrently on one session.

## [0.1.2] - 2026-10-09

### Fixed

- **Login could not start Chrome as root.** `Authenticate` did not pass
  `--no-sandbox` when running as root (WSL, containers), so Chrome refused
  to start and login failed before reaching the site. The page-fetch path
  already handled this; login now uses the same check.
- **A base URL without a scheme produced an invalid login URL.**
  `NewClient("ps.example.org")` kept the bare host, and the browser rejected
  `ps.example.org/public/` as an invalid URL. A value with no scheme is now
  treated as an `https` host; surrounding whitespace is trimmed; a value with
  no host is rejected.

Both found on the first live login attempt after 0.1.1; login, students,
grades and assignments were then verified end-to-end against a live parent
account.

## [0.1.1] - 2026-10-09

### Fixed

- **Did not compile with Go 1.27.** The pinned
  `github.com/go-json-experiment/json` (an indirect dependency through
  `chromedp/cdproto`) aliases the standard library's `encoding/json/v2`,
  whose API changed in Go 1.27 (`undefined: json.SkipFunc`,
  `json.DiscardUnknownMembers`). Bumped it to a release that builds on Go
  1.26 and 1.27. `chromedp` stays on v0.14.2 with its matching `cdproto`.
- Reverted an automated `cdproto` digest bump that had been merged on its
  own and left `main` uncompilable against `chromedp` v0.14.2. Renovate now
  treats the two as a pair and holds `chromedp` below v0.15, whose rewritten
  Action API needs a port of `internal/browser` first.

### Added

- A real-browser smoke test (`internal/browser`) that sets a cookie,
  navigates, and reads back a JavaScript-modified DOM through headless
  Chrome. It is the only runtime check of the DevTools-protocol encoding and
  skips when no Chrome binary is installed or under `-short`.

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
