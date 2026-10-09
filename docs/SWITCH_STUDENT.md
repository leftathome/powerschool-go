# Switching students

The parent portal shows one student at a time. Which one is state on the
server side of the session; every page read returns that student's data
whatever the caller had in mind. Reading a second child therefore means
changing that state first, and proving that it changed.

## What the portal does

Observed on a live instance (structure only; ids and names below are
placeholders):

```html
<ul id="students-list"><li class="selected"><a href="javascript:switchStudent(111111);">NAME</a></li><li ><a href="javascript:switchStudent(111112);">NAME</a></li></ul>
<form id="switch_student_form" action="home.html" method="post"><input type="hidden" name="selected_student_id"/></form>
```

```js
function switchStudent(studentId){ var f = document.getElementById('switch_student_form'); f.selected_student_id.value=studentId; f.submit(); }
```

- The selected student is marked explicitly: `li.selected`. The unselected
  item is written `<li >`.
- Switching is a form post to `/guardian/home.html` with exactly one field,
  `selected_student_id`, holding the id from `switchStudent(...)`. No token
  field accompanies it.
- Pages are server-rendered: a plain GET of `/guardian/home.html` with the
  session cookies attached returns 200 and contains `table#tblgrades`.
  The cookies must be attached to each request explicitly; a `net/http`
  cookie jar dropped them and the portal answered 302.

## What the library does

`SelectStudent(ctx, id)` (and `SwitchStudent`, `GetGrades(ctx, id)`, which
use it):

1. GET the home page. If it marks no student as selected, stop with
   `ErrSelectionUnknown` -- a switch could not be confirmed, so none is
   attempted. If `id` is already selected, return this page.
2. If `id` is not in the switcher, stop with `ErrNotFound`.
3. POST `selected_student_id=<id>` to `/guardian/home.html`
   (`application/x-www-form-urlencoded`), through the client's
   `http.Client`. The response is discarded.
4. GET the home page again and require `li.selected` to carry `id`;
   otherwise `ErrSwitchNotConfirmed`.

The post changes which student the session displays and nothing else: no
record is created, edited or submitted. A caller that restricts the client
to read-only traffic needs to let through exactly this request
(`SwitchStudentPath`, `SwitchStudentField`) and the assignment lookup
(`AssignmentLookupPath`).

Selection is shared by everything using the session. Do not run reads for
different students concurrently on one session; switch, read, confirm, then
move on.

## Not yet confirmed against a live instance

The markup above was observed; the switch itself has not been exercised by
this library against a real portal. To settle:

1. **The post's response.** Status and `Location` (a redirect to
   `home.html` is assumed and a 200 is also handled), and whether any
   `Set-Cookie` comes back. The library attaches the cookies it was given
   and ignores `Set-Cookie`; if the portal rotates the session cookie on a
   switch, the next read will fail as an expired session.
2. **Whether the selection is per session or per account.** If a parent
   switching students in a browser moves the selection of a second,
   separately signed-in session, a long read can have its student changed
   underneath it. Callers should verify the selection again after reading.
3. **Which student a fresh session starts on**, and whether the last
   selection persists across sign-ins.
4. **Single-student accounts.** Whether the page has a `#students-list` at
   all and marks its one item. The library treats a page with no switcher
   as unreadable.
5. **The scores page.** Whether `scores.html` carries the same
   `#students-list` marker, and that the `studentFRN` in its `data-ng-init`
   differs between the two students (callers use it to check that every
   scores page read in one pass belongs to one student).
6. **The sign-in page served with a 200.** It is recognised by
   `form#LoginForm` / `input#fieldPassword` / `input[name="pw"]` on a page
   with no student switcher. A `scores.html` request with incomplete
   parameters gets the same answer and cannot be told apart from a dead
   session.
7. **Session lifetime.** The expiry the library reports after login is a
   one-year placeholder, not the portal's real session lifetime; expiry is
   only discovered when a read is refused.
