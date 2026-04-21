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

    students, err := client.GetStudents(ctx)
    if err != nil {
        log.Fatal(err)
    }

    for _, s := range students {
        fmt.Printf("%s (grade %d)\n", s.Name, s.GradeLevel)

        grades, err := client.GetGrades(ctx, s.ID)
        if err != nil {
            log.Fatal(err)
        }
        for _, g := range grades {
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
}
```

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
| Progress reports           | partial                          |
| Attendance                 | not yet                          |
| Calendar/events            | not yet                          |
| Multi-student switching    | not yet (home page shows one at a time) |

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
