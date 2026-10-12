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
- The teacher's note on an individual score (the "notes" icon of the class
  score page), as plain text and as sent, from that same lookup.
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
Alternatively, point the sign-in at a browser running elsewhere: see
[Remote browser](#remote-browser).

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

### Teacher comments on an assignment

A teacher can leave a note on one student's score; the portal marks such a
row with a notes icon and shows the note in the assignment's detail dialog.
`GetAssignments` returns it with the assignment -- it is in the lookup
response already, so there is no extra request:

```go
assignments, _ := client.GetAssignments(ctx, s.ID, g)
for _, a := range assignments {
    if a.HasTeacherComment {
        fmt.Printf("%s:\n%s\n", a.Title, a.TeacherComment)
    }
}
```

| Field | What it holds |
| ----- | ------------- |
| `TeacherComment` | The note as plain text: character references decoded, tags removed, `<br>` and block elements (`<p>`, `<div>`, `<li>`, ...) turned into line breaks, the teacher's own line breaks kept (at most one blank line in a row), other whitespace collapsed, `<script>`/`<style>` content and control characters dropped. |
| `TeacherCommentHTML` | The note as PowerSchool sent it, trimmed. **Not sanitised** -- the portal renders it as HTML; do not put it in a page as is. |
| `HasTeacherComment` | `TeacherComment != ""`: there is text to read. |
| `TeacherCommentTruncated` | The note was longer than `MaxTeacherCommentBytes` (16 KiB) and one or both of the fields above were cut (at a character boundary). |
| `CategoryDescription` | The teacher's description of the assignment's primary category, as plain text. |

Things to know:

- **Which score.** One `Assignment` is returned per section of an assignment
  (normally one), and its score, flags and comment all come from that
  section's first score entry. A comment is never taken from a different
  score than the points beside it.
- **No comment is the normal case.** PowerSchool leaves the comment member
  out of a score that has none. Absent, null, empty, whitespace-only and
  unexpectedly shaped values all give `HasTeacherComment == false` and never
  fail the call.
- **Markup-only notes.** A note with markup but no text (say, a lone image)
  has `HasTeacherComment == false` and an empty `TeacherComment`, while
  `TeacherCommentHTML` still carries it. The portal's icon shows for any
  non-null note, so the two can differ for such a value.
- **It is somebody else's free text.** Treat both fields as untrusted input
  wherever they are shown or passed to another program (a template, a
  shell, a language model).
- **No timestamp.** The score's own dates are not the comment's, and the
  comment object's other members have not been observed, so the library
  does not say when a note was written. Compare the text to detect a new or
  changed note.
- **Verification.** The field names come from the portal's own page
  scripts; no live response containing a note had been captured when this
  was written, so the decoding is tested against invented fixtures only.
  See [docs/API_ENDPOINTS.md](docs/API_ENDPOINTS.md).

`Assignment.Description` is unchanged: tags stripped, character references
left as sent.

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

### Remote browser

By default the sign-in starts a Chrome on the machine running your code.
Set `BrowserURL` to drive it in a browser that is already running somewhere
else instead, for example a shared browser service in the same cluster, so
that no Chrome has to be installed next to your code:

```go
err := client.AuthenticateWithOptions(ctx, &powerschool.AuthOptions{
    BrowserURL: os.Getenv("BROWSER_WS_URL"), // "ws://browser.example.internal:3000?token=..."
    Timeout:    90 * time.Second,
})
```

`BrowserURL` is the browser's DevTools WebSocket endpoint. Only `ws://`
and `wss://` are accepted, and the URL is dialled exactly as given;
`http://` and `https://` endpoints are rejected rather than resolved. When
`BrowserURL` is empty nothing changes: the local sign-in is the same code
path as before.

Only the sign-in uses the remote browser. Combine it with
`WithHTTPPageFetch()` if nothing in your process should start a browser at
all; without that option the page reads after the sign-in still start a
local Chrome.

What the library does on a browser it shares with others:

- The sign-in runs in a browser context created for that one call (an
  incognito-style profile), never in the browser's default context. It
  shares no cookie, storage or cache with anything else in that browser.
- That context is disposed, with its page, cookies and storage, on every
  way out: success, failure, timeout, cancellation of `ctx`, and a panic.
  It is also created so that the browser discards it by itself when the
  connection closes, which covers a process that dies mid-way.
- Nothing else in the browser is touched: the browser is never closed, and
  no other context or page is closed, attached to or changed. (This is
  about what the library does to others, not what others can do to it:
  see the note on other DevTools clients below.)
- `Timeout` covers the dial, the sign-in and the disposal. After `ctx` is
  cancelled the call returns once the context is disposed: normally at
  once, a few seconds at most, and never later than `Timeout`.
- The endpoint is treated as a secret because it usually carries a token.
  Errors and logs show it as `scheme://host[:port]` and nothing else, and
  errors from the dialer are not kept in the error chain.

What it cannot do:

> **In remote mode the password is typed into, and the session cookies
> briefly exist in, a browser you do not own.** Whoever operates that
> browser can observe them.
>
> **A browser context does not isolate the sign-in from other DevTools
> clients of the same browser.** It separates cookies and storage between
> pages; it is not a boundary between the programs that control the
> browser. Anything else that can connect to the same browser (the same
> endpoint and token) while the sign-in runs can attach to its page, read
> what is typed and read its cookies. Share the browser only with clients
> you would trust with the account.
>
> **Not verified against a real browser service.** The behaviour this
> relies on (`Target.createBrowserContext` with `disposeOnDetach`, and
> opening the page with `newWindow` when a tab is refused) has been run
> against a plain headless Chrome only. Whether a given service such as
> Browserless v2 permits and honours them is untested: check, after a
> first sign-in, that nothing of it is left in the service.

The traffic between your process and the browser carries the keystrokes
and the cookies too: use `wss://`, or keep a `ws://` endpoint on a network
you trust. The size of a message the remote browser sends is not bounded
by this library (the DevTools client it uses offers no limit): a hostile
or broken browser can make the process use a lot of memory for the
duration of a sign-in.

Options that describe a browser the library starts do not apply to one it
does not. Setting `UserDataDir`, `NoSandbox` or a false `Headless`
together with `BrowserURL` is an error (`ErrInvalidAuthOptions`) rather
than being silently ignored. `UserAgent` is applied to the sign-in's page.
`DebugLog` prints the names of the steps only.

### What a sign-in checks, and what its errors mean

Both the local and the remote sign-in:

- refuse a username or password containing a line break, a tab, another
  control character, or a character the browser automation would type as
  a key instead of as text (`ErrInvalidAuthOptions`, before any browser
  work). The value is not trimmed for you: a secret read from a file with
  a trailing newline must be trimmed by the caller. A newline typed into
  the form is the Enter key, which submits it half filled;
- type and submit the credentials only while the page is on the origin
  (scheme, host and port) of the client's base URL. If the sign-in URL
  ends up on another origin the sign-in fails with `ErrUnexpectedOrigin`
  and nothing is typed there. There is no allow-list: a portal whose
  sign-in form lives on a second origin is not supported;
- return only the cookies that belong to the portal's host (the cookie's
  domain is that host or a parent domain of it; `Secure` cookies only for
  an `https` portal). Cookies of other sites the signed-in page embeds
  are dropped.

A remote sign-in, in addition:

- submits the form at most once per call. A click that fails is never
  followed by another control or by the Enter key, and a page that
  submits itself while being filled is not clicked;
- reports success only on positive evidence: the browser is on the
  portal's origin, on a new document whose path is not the sign-in
  page's, with no password field. The portal's error banner is
  `ErrInvalidCredentials`. Anything else (no answer before `Timeout`, a
  page that cannot be read, a browser that went away) is an error, never
  a success;
- says how far a failed sign-in got. The line is drawn before the first
  keystroke of the password, not at the click, because a page can submit
  itself while it is being filled.

This matters when an account allows one session at a time and locks after
repeated bad submissions. Check the cases in this order:

```go
err := client.AuthenticateWithOptions(ctx, opts)
switch {
case err == nil:
    if client.BrowserContextDisposalUnconfirmed() {
        // Signed in, but the remote browser did not confirm that it
        // disposed the sign-in's browser context. The connection was
        // closed, which disposes it; act on this if you need certainty.
    }
case errors.Is(err, powerschool.ErrInvalidAuthOptions):
    // Input that can never work (conflicting options, bad BrowserURL,
    // a credential with a line break in it). Nothing was sent. Do NOT
    // retry: fix the input. This never matches
    // ErrCredentialsNotSubmitted.
case errors.Is(err, powerschool.ErrInvalidCredentials):
    // The portal rejected the one submission. Do not retry with the
    // same credentials.
case errors.Is(err, powerschool.ErrCredentialsNotSubmitted):
    // The password never reached the page (browser unreachable, sign-in
    // page did not load or is on another origin, cancelled early...).
    // Retrying does not count against the account.
    // errors.Is(err, powerschool.ErrBrowserConnection) tells a browser
    // problem from a portal problem.
default:
    // ErrCredentialsSubmitted, or any error that matches neither
    // sentinel: the outcome is unknown. The portal may have accepted or
    // rejected the sign-in. Do not retry blindly.
}
```

All of these are still `IsAuthError(err)`.

**A failed local sign-in matches neither `ErrCredentialsNotSubmitted` nor
`ErrCredentialsSubmitted`** (except for the two shared checks above). Treat
it as outcome unknown, which is what the `default` branch does: never
write `if !errors.Is(err, ErrCredentialsSubmitted) { retry }`.

The local sign-in also keeps its older success rule: it reports success
when no error banner is showing three seconds after the click. Against a
portal slower than that it can report a sign-in that did not happen (the
session then has no session cookie and the first read fails as expired).
The remote sign-in does not have this weakness.

### Session reuse

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
# Unit tests. No network. Tests that need a browser start a local
# headless Chrome and are skipped when none is installed or with -short.
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
| Assignment teacher comment | field names from the portal's scripts; fixture-tested, no live comment seen |
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
- With `AuthOptions.BrowserURL` the password and the session cookies
  pass through a browser you do not start. The library isolates the
  sign-in in its own browser context and disposes it afterwards, but
  cannot protect them from whoever runs that browser. The URL itself
  usually carries a token: keep it out of source control and logs (the
  library redacts it in its own errors and logs). See
  [Remote browser](#remote-browser).
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
