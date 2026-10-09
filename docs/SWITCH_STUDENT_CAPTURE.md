# Switching students: what has to be captured first

`Client.SwitchStudent` is not implemented. It returns
`ErrSwitchStudentUnsupported` and contacts nothing. Until it is implemented
the library reads exactly one student per session: the one the portal marks
as selected on `/guardian/home.html`.

This is deliberate. The parent portal keeps "which student am I showing" as
state on the server side of the session, changed by the page's
`switchStudent(<id>)` JavaScript. What that function sends has never been
recorded, and the candidates differ in ways that matter:

- a GET that navigates with a query parameter is a read;
- a form POST to `home.html` is a write-shaped request that a read-only
  caller must explicitly allow, and whose body may need a per-page token.

Implementing either by guesswork risks the worst outcome this library has:
believing the switch happened when it did not, and returning one child's
grades under another child's id.

## What to capture

From a signed-in browser session on an account with at least two students,
with DevTools open (Network tab, "Preserve log" on). Redact before sharing:
replace student ids, names, school ids and cookie values with placeholders.

1. **The function.** The source of `switchStudent` as the page defines it
   (Console: `switchStudent.toString()`), and the markup of the element it
   submits, if any: the form's `id`, `action`, `method`, and the `name` of
   every input including hidden ones.
2. **The request it makes when a student is clicked.** Method; path; query
   parameter names; `Content-Type`; body field names (which field carries
   the student id, and whether that id is the `switchStudent()` nav id or
   another identifier); whether a token field is present and where its
   value comes from on the page.
3. **The response.** Status code; `Location` header if it redirects;
   whether any `Set-Cookie` is returned (does the session cookie change?).
4. **The page after the switch.** The `#students-list` markup: is the newly
   chosen student the one carrying `class="selected"`, and is exactly one
   `li` marked? The `table.student-demo` "Student ID #" row for that
   student.
5. **Scope of the selection.** Sign in a second time in a private window
   (a second session for the same account). Switch student in the first
   window, reload the second. Does the second session follow, or is the
   selection per session? This decides whether a connector's session can be
   moved by a parent using the portal at the same time.
6. **Persistence.** Sign out and in again: which student is selected by
   default -- the last one chosen, or always the first?
7. **A single-student account, if one is available.** Does the page have a
   `#students-list` at all, and is its one `li` marked `selected`? The
   library treats a page with no switcher as unreadable.
8. **The scores page.** On `scores.html` for one course of each student:
   the `studentFRN` value in `data-ng-init` (so the two students' values
   can be told apart), and whether the page carries the same
   `#students-list` marker as the home page.

## Also unconfirmed against a live instance

- That `table#tblgrades` is present in the raw `home.html` response without
  JavaScript (HTTP page mode depends on it).
- That the sign-in page is recognisable by `form#LoginForm` /
  `input#fieldPassword` / `input[name="pw"]` when it is served with a 200
  in place of a signed-in page, and that no signed-in page without a
  student switcher contains one of those.
- That a `scores.html` request with a valid session and complete parameters
  never answers with the sign-in page. The library reports the sign-in page
  as a session-expired error wherever it appears; an incomplete scores URL
  produces the same answer and cannot be told apart.
