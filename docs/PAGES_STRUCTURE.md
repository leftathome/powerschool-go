# Attendance, grade history and teacher comments: page structure

What the parsers for `/guardian/attendance.html`, `/guardian/termgrades.html`
and `/guardian/teachercomments.html` rely on. The structure was first taken
from captures of the live pages in which every text and attribute value had
been reduced to its shape, and the open points were then checked against
the live pages (structure only). Nothing below, and nothing in `testdata/`,
is personal data: names, ids, courses and comments are invented. The
attendance legend is quoted as observed; it is district policy vocabulary.

All three are server-rendered and are read with a plain GET through the
client's `http.Client` (`WithHTTPPageFetch`), like the home page.

## Whose page is it (verified live)

Each of these pages describes the student the SESSION has selected, and
each carries the portal's full student switcher, as the home page does:
`ul#students-list` with one `li` per student and exactly one `li.selected`.

The three parsers read the selected id from that switcher
(`#students-list li.selected a[href^="javascript:switchStudent("]`), return
it as `PageStudent.SelectedID`, and **fail with `ErrSelectionUnknown`** when
the page marks nobody, marks more than one, or has no switcher. A caller
attributing a page to a student compares `SelectedID` with that student's
id -- for every page it reads.

`PageStudent.HeadingName` is the name the page's `h1` ends with
(`Meeting Attendance History: Last, First`, `Teacher Comments: Last, First`;
the grade history's heading names nobody). It is a display name for people.
Do not use it to decide whose page this is.

## attendance.html

`table.grid`. The header is two rows and is repeated every few weeks, each
time followed by one row per course:

```
tr   th[rowspan=2] "Course" | th[rowspan=2] "Expression" | th[colspan=5] "M/D-M/D" ...   (one per week)
tr   th "M" | th "T" | th "W" | th "H" | th "F" ...                                     (five per week)
tr   td  COURSE NAME <br> Teacher Last, First M ROOM <br> E: MM/DD/YYYY L: MM/DD/YYYY
     td  expression, e.g. 3(A)
     td x (5 per week)   one cell per school day
```

- The week header is `month/day-month/day` with a Unicode right-to-left mark
  (U+200F) after the hyphen. **It has no year.** The course cell's `E:` /
  `L:` enrolment dates do. The five columns under a header are Monday to
  Friday, and the parser chooses the year that puts the header's days on
  those weekdays and, where the row has enrolment dates, inside the
  enrolment; a row without them uses the school year (July to June) of a
  reference date. A header may start or end mid-week (a short school week)
  and is mapped onto the matching columns. Every date is checked against
  its column's weekday; a header that fits no year is a parse error.
- Day cells (verified live): blank, `-`, `.`, or an attendance code. Blank
  is present (`Blank=Present` in the legend). `-` and `.` are **not in the
  legend**; they are non-events -- no class for the student that day
  (`CellNoMeeting`) and nothing marked (`CellUnmarked`) -- and are never
  attendance codes. The codes seen in the grid were `A`, `L`, `LE`, `ME`,
  `VU`.
- There are no totals on this page. `AttendanceCourse.Count` counts cells
  by legend category.
- Legend (verified live): `div#legend p`, text
  `Attendance Codes: Blank=Present | CODE=CATEGORY-Description | ...`,
  entries separated by `<span class="disclaimer">|</span>`. Several codes
  share a category:

  ```
  FT=AUTH-Field Trip | IA=AUTH-On Campus | OA=AUTH-Off Campus |
  AA=EA-Approved Activity | EA=EA-Excused Absence | FE=EA-Family Emergency |
  ME=EA-Medical/Sick | QE=EA-Health Excused | SS=EA-Suspended |
  EL=EL-Excused Late | LB=EL-Late Transport | LE=LE-Left Early Excused |
  LU=LU-Left Early Unexcused | UA=UA-Unexcused Absence | VU=UA-Unexcused Travel |
  UL=UL-Unexcused Late | A=A-Absent | L=L-Late |
  ```

  The parser reads the legend from the page -- it is never hardcoded -- and
  exposes each entry as `AttendanceCode{Code, Category, Description}`. What
  a category means for a household (excused or not, an absence or not) is
  the caller's decision.

## termgrades.html

Despite its name this is the **grade history**: stored (final) grades of
completed courses, not the current year's quarter grades (those are the
Q1..S2 columns of the home page).

`table#grades-tbl`, `thead` columns (verbatim, verified live): `Date Compl`,
`Grade Level`, `School`, `Course #`, `Course`, `Credit Earned`,
`Credit Attempted`, `Grade`. The parser finds columns by header text.

- `Date Compl` is `MM/YYYY`.
- The Grade cell is the letter followed by `<b></b>`; `<b>*</b>` marks a
  grade the page says is not included in the GPA.
- The site map links the page as `termgrades.html?schoolid=..&termid=..`,
  one link per school year and school. The parser reads whichever the
  portal serves at the bare path; the two parameters are not used. Observed:
  one school year, two distinct completion months.

## teachercomments.html

- `select#reportingTermHeadingSelect[name=storecode]` inside
  `form[name=navigation]`. Despite the select's name, **the term parameter
  is `fg`** (verified live; `?storecode=Q1` has no effect). The option
  VALUES are relative URLs:

  ```
  teachercomments.html?fg=&showdropped=     (first option, blank label)
  teachercomments.html?fg=Q1
  ...  fg=Q2, fg=Q3, fg=Q4, fg=S1
  teachercomments.html?fg=S2
  ```

- The page marks the term it shows with `selected` on that option: the bare
  page marks the current term; `?fg=Q1` marks Q1.
- Quirk: when a term is requested, the FIRST option's value becomes
  `teachercomments.html?fg=<term>&showdropped=`. It is the "show dropped
  classes" entry, not a term: an option is a term only when its URL has a
  non-empty `fg` and no `showdropped`.
- `table.grid.linkDescList` with columns Exp., Course #, Course, Teacher,
  Comment. The teacher cell is an info button and an `Email Last, First M`
  link; the comment is the text of a `<pre>`, empty when there is none.

`GetTeacherComments(ctx, term)` requests `?fg=<term>` and requires the page
to mark that term as selected (`ErrTermMismatch` otherwise). `term == ""`
reads the bare page; `TeacherCommentsPage.Term` then says which term it is.

## Still not verified against a live instance

1. **A non-empty comment.** No comment was posted in any term when the page
   was checked (every `<pre>` empty), so reading comment text out of the
   `<pre>` is tested against fixtures only.
2. **Attendance:** that the teacher line ends with the room; that the
   weekly columns are always Monday to Friday; the year logic across a real
   new-year week.
3. **Grade history** for a student with several school years: whether the
   bare URL returns everything or one `termid` only.
4. **Single-student accounts:** whether the switcher is rendered at all.
   Without it these parsers return `ErrSelectionUnknown`.
