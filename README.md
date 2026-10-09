# powerschool-go

A Go client library for the PowerSchool e-learning platform.

> **v0.1.0.** The library covers the paths a parent account needs
> — login, students, grades, and per-course assignments — against at
> least one real PowerSchool instance. See
> [CHANGELOG.md](CHANGELOG.md) for release notes and known
> limitations.

## Overview

`powerschool-go` drives PowerSchool through the same flow a browser
does: it logs in with real credentials via
[chromedp](https://github.com/chromedp/chromedp), extracts session
cookies, and then makes HTTP/XHR calls the way PowerSchool's own
Angular frontend does. That avoids OAuth dances, works with districts
that front PowerSchool with an F5/BigIP load balancer, and tolerates
the JavaScript-heavy scores pages.

Intended audience: parents who want to programmatically access their
own child's data (dashboards, notifications, personal archives).

## Features

- Browser-driven authentication (handles SSO, BigIP sessions).
- Student discovery for multi-child parent accounts.
- Grades parsing from the home-page table (Q1–Q4, S1/S2, attendance).
- Assignments via the `/ws/xte/assignment/lookup` XHR, including
  categories, due dates, scores, and the full flag set
  (late, missing, collected, exempt, incomplete, absent).
- Attendance by day: the per-course grid of `attendance.html`, with real
  dates, codes and the legend.
- Grade history (`termgrades.html`): stored grades and credits.
- Teacher comments per reporting term (`teachercomments.html`).
- Every page read reports the student that same page marks as selected
  (`PageStudent`), and fails closed when it marks none.
- Session export/import for reusing a warm login across runs.
- Typed errors with `IsAuthError` / `IsSessionExpired` /
  `IsNotFound` / `IsRateLimited` helpers.
- `context.Context` on every call for cancellation and timeouts.

## Installation

```bash
go get github.com/leftathome/powerschool-go
```

You'll also need Chrome or Chromium installed on the machine running
your code — `chromedp` launches it to handle login.

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/leftathome/powerschool-go"
)

func main() {
    ctx := context.Background()

    client, err := powerschool.NewClient(
        "https://"+os.Getenv("POWERSCHOOL_HOST"),
        powerschool.WithCredentials(
            os.Getenv("POWERSCHOOL_USERNAME"),
            os.Getenv("POWERSCHOOL_PASSWORD"),
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
    if err := client.Authenticate(ctx); err != nil {
        log.Fatal(err)
    }

    // One fetch: the account's students and the grades of the student
    // the session has selected.
    page, err := client.GetHomePage(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, s := range page.Students {
        fmt.Printf("%s (selected: %v)\n", s.Name, s.Selected)
    }

    s := page.SelectedStudent()
    if s == nil {
        log.Fatal("the page does not mark which student is selected")
    }
    for _, g := range page.Grades {
        fmt.Printf("  %-25s %s\n", g.CourseName, g.CurrentGrade)

        // GetAssignments takes the *Grade (not a raw courseID)
        // because the scores-page URL and the API-facing student
        // DCID are both captured during grade parsing and cached
        // on the Grade for reuse.
        assignments, err := client.GetAssignments(ctx, s.ID, g)
        if err != nil {
            log.Printf("    assignments unavailable: %v", err)
            continue
        }
        fmt.Printf("    %d assignment(s)\n", len(assignments))
    }
}
```

**One student at a time.** The portal shows one student per session and
every read returns that student's data. `SelectStudent(ctx, id)` submits
the portal's own switcher form, re-reads the home page and confirms the
marker moved (`ErrSwitchNotConfirmed` if not); `GetGrades(ctx, id)` uses it.
`Student.Selected` is set only from the page's explicit marker -- with no
marker the library returns `ErrSelectionUnknown` rather than guess. Do not
read different students concurrently on one session. See
[docs/SWITCH_STUDENT.md](docs/SWITCH_STUDENT.md).

### More pages

```go
att, _ := client.GetAttendance(ctx)             // attendance.html
for _, course := range att.Courses {
    for _, day := range course.Days {
        if day.Kind == powerschool.CellCode {
            entry, _ := att.Describe(day.Code)
            fmt.Println(course.CourseName, day.Date.Format("2006-01-02"), day.Code, entry.Description)
        }
    }
}

history, _ := client.GetGradeHistoryPage(ctx)   // termgrades.html
comments, _ := client.GetTeacherComments(ctx, "Q1") // teachercomments.html?fg=Q1
```

Each result has a `Student` field (`PageStudent`): the student that page's
own switcher marks as selected. These pages show the session's selected
student whatever you had in mind, so compare `Student.SelectedID` with the
student you meant, for every page; the parsers fail with
`ErrSelectionUnknown` rather than return a page that marks nobody. Page
shapes and what is still unverified:
[docs/PAGES_STRUCTURE.md](docs/PAGES_STRUCTURE.md).

A complete runnable version lives in
[`cmd/test-auth/main.go`](cmd/test-auth/main.go) — a good first
target for trying the library against your own instance.

## Authentication options

```go
headless := false // show the browser (useful for debugging)
err := client.AuthenticateWithOptions(ctx, &powerschool.AuthOptions{
    Headless:  &headless,
    Timeout:   90 * time.Second,
    DebugLog:  true,
    NoSandbox: os.Geteuid() == 0, // WSL/containers as root
})
```

Session reuse (skip the browser on subsequent runs while the
24-hour cookie is still valid):

```go
// Save after a successful Authenticate.
data, _ := json.Marshal(client.ExportSession())
_ = os.WriteFile("session.json", data, 0o600)

// Restore later.
var s powerschool.SessionExport
_ = json.Unmarshal(sessionBytes, &s)
_ = client.ImportSession(&s)
```

## Error handling

```go
grades, err := client.GetGrades(ctx, studentID)
switch {
case powerschool.IsSessionExpired(err):
    _ = client.Authenticate(ctx)
case powerschool.IsAuthError(err):
    log.Fatal("credentials rejected")
case powerschool.IsNotFound(err):
    // ...
case err != nil:
    log.Printf("grades: %v", err)
}
```

See [`errors.go`](errors.go) for the full set of sentinel errors and
`Is*` helpers.

## District compatibility

PowerSchool HTML varies between versions and district customizations.
The library was developed against one reference instance; observed
selectors and quirks are documented in
[`docs/REFERENCE_STRUCTURE.md`](docs/REFERENCE_STRUCTURE.md).

Known pitfalls (fixed in the library but worth understanding if a
new district behaves oddly):

- `scores.html?frn=X` alone redirects to the login page — `begdate`,
  `enddate`, `fg`, and `schoolid` are all required. `Grade.ScoresURL`
  captures the full URL from the home-page table.
- The assignment-lookup API expects the *studentFRN*-derived DCID,
  not the `switchStudent()` nav ID on `Student.ID`. The library
  parses both.
- `Referer` is mandatory on the lookup POST; missing it returns 400 +
  HTML. See [`docs/ASSIGNMENTS_API_DEBUG.md`](docs/ASSIGNMENTS_API_DEBUG.md).

If you want to add support for a different district, see
[CONTRIBUTING.md](CONTRIBUTING.md#supporting-a-new-district).

## Testing

```bash
# Fast, hermetic unit tests (no network, no Chrome).
go test ./...

# Live integration test (drives real Chrome against real PowerSchool).
export POWERSCHOOL_HOST=...
export POWERSCHOOL_USERNAME=...
export POWERSCHOOL_PASSWORD=...
go test -tags integration -run TestIntegration -timeout 5m ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on fixture
management, the PII redactor, and adding tests for new behavior.

## Status

| Area                       | State                            |
| -------------------------- | -------------------------------- |
| Authentication             | working                          |
| Students                   | working                          |
| Grades                     | working                          |
| Assignments (via XTE API)  | working + unit + integration test |
| Attendance by day          | structure verified live; code run on fixtures |
| Grade history              | structure verified live; code run on fixtures |
| Teacher comments           | structure verified live; non-empty comment fixture-only |
| Multi-student switching    | working (`SelectStudent`), verified live |
| Progress reports           | not implemented (`ErrNotImplemented`) |
| GPA (`GetGPA`)             | text heuristic, unverified       |
| Calendar/events            | not yet                          |

## Security

- Treat session cookies as credentials. `.schoology-session.json`
  and `*session*.json` are gitignored by default.
- Never commit raw PowerSchool captures. Use
  [`hack/redact`](hack/redact) to scrub names, UIDs, and hosts
  before committing test fixtures. See
  [CONTRIBUTING.md](CONTRIBUTING.md#fixture-management).
- This library is for personal/family use against accounts you own.
  Use it in accordance with your district's acceptable use policies.

## License

MIT — see [LICENSE](LICENSE).

## Related projects

- [schoology-go](https://github.com/leftathome/schoology-go) — sibling
  library for Schoology; same philosophy, shared patterns (including
  the `hack/redact` tool).

## Disclaimer

Not affiliated with or endorsed by PowerSchool Group LLC. "PowerSchool"
is a registered trademark of PowerSchool Group LLC.
