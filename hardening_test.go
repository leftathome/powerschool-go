package powerschool

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Review hardening for the attendance, grade-history and teacher-comment
// parsers. All markup and values are synthetic.

// attendanceGrid builds a one-course attendance page: `course` is the
// content of the course cell after the name (enrolment line etc.), weeks
// are the header cells, cells are the day cells (5 per week; blank when
// nil).
func attendanceGrid(course string, weeks []string, cells []string) string {
	var b strings.Builder
	b.WriteString(switcher("111111") + `<div id="content-main"><h1>Meeting Attendance History: Rivera, Ana</h1><table class="grid"><tr><th rowspan="2">Course</th><th rowspan="2">Expression</th>`)
	for _, w := range weeks {
		b.WriteString(`<th colspan="5">` + w + `</th>`)
	}
	b.WriteString(`</tr><tr>` + strings.Repeat(`<th>M</th><th>T</th><th>W</th><th>H</th><th>F</th>`, len(weeks)) + `</tr>`)
	b.WriteString(`<tr><td>EXAMPLE ART<br>Nakamura, Lena 3` + course + `</td><td>4(A)</td>`)
	for i := 0; i < 5*len(weeks); i++ {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		b.WriteString(`<td>` + cell + `</td>`)
	}
	b.WriteString(`</tr></table></div>`)
	return b.String()
}

func dayStrings(t *testing.T, html string, ref time.Time) []string {
	t.Helper()
	page, err := ParseAttendancePage(html, ref)
	if err != nil {
		t.Fatalf("ParseAttendancePage: %v", err)
	}
	var out []string
	for _, d := range page.Courses[0].Days {
		out = append(out, d.Date.Format("Mon 2006-01-02"))
	}
	return out
}

func wantParseError(t *testing.T, err error) {
	t.Helper()
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != CodeParseError {
		t.Fatalf("err = %v, want a typed parse error", err)
	}
}

// TestAttendanceDates_MatchTheirWeekdayColumns (L1): the grid's five
// columns per week are Monday to Friday. Every date the parser returns must
// fall on its column's weekday; when that cannot be arranged the parser
// returns a parse error, never a wrong date.
func TestAttendanceDates_MatchTheirWeekdayColumns(t *testing.T) {
	const enrolled = "<br>E: 09/03/2025 L: 06/17/2026"

	t.Run("a normal Monday week", func(t *testing.T) {
		got := dayStrings(t, attendanceGrid(enrolled, []string{"9/8-9/12"}, nil), time.Time{})
		want := []string{"Mon 2025-09-08", "Tue 2025-09-09", "Wed 2025-09-10", "Thu 2025-09-11", "Fri 2025-09-12"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("days = %v, want %v", got, want)
		}
	})

	// (a) The first week of school: the header starts on a Wednesday. The
	// Wednesday cell is the third column, not the first.
	t.Run("a header that starts mid-week maps onto the matching columns", func(t *testing.T) {
		html := attendanceGrid(enrolled, []string{"9/3-9/5", "9/8-9/12"}, []string{"-", "-", "A", "", "L"})
		page, err := ParseAttendancePage(html, time.Time{})
		if err != nil {
			t.Fatalf("ParseAttendancePage: %v", err)
		}
		days := page.Courses[0].Days
		want := []string{"Mon 2025-09-01", "Tue 2025-09-02", "Wed 2025-09-03", "Thu 2025-09-04", "Fri 2025-09-05",
			"Mon 2025-09-08", "Tue 2025-09-09", "Wed 2025-09-10", "Thu 2025-09-11", "Fri 2025-09-12"}
		for i, w := range want {
			if got := days[i].Date.Format("Mon 2006-01-02"); got != w {
				t.Errorf("day %d = %s, want %s", i, got, w)
			}
		}
		// The absence marked in the Wednesday column is on Wednesday the 3rd.
		if days[2].Code != "A" || days[2].Date.Format("2006-01-02") != "2025-09-03" {
			t.Errorf("the Wednesday cell = %+v, want code A on 2025-09-03", days[2])
		}
		if days[4].Code != "L" || days[4].Date.Weekday() != time.Friday {
			t.Errorf("the Friday cell = %+v", days[4])
		}
	})
	t.Run("a header that ends mid-week", func(t *testing.T) {
		got := dayStrings(t, attendanceGrid(enrolled, []string{"11/24-11/26"}, nil), time.Time{})
		if got[0] != "Mon 2025-11-24" || got[4] != "Fri 2025-11-28" {
			t.Errorf("days = %v", got)
		}
	})

	// (b) No enrolment dates, read in the summer after the school year the
	// page shows. "9/1-9/5" is Monday to Friday in 2025 and Tuesday to
	// Saturday in 2026: only one year fits the columns.
	t.Run("no enrolment dates, reference date in July", func(t *testing.T) {
		for _, ref := range []time.Time{ymd(2026, 7, 20), ymd(2026, 8, 3), ymd(2026, 6, 20)} {
			got := dayStrings(t, attendanceGrid("", []string{"9/1-9/5", "1/5-1/9"}, nil), ref)
			if got[0] != "Mon 2025-09-01" || got[5] != "Mon 2026-01-05" {
				t.Errorf("reference %s: week starts = %s, %s; want 2025-09-01 and 2026-01-05", ref.Format("2006-01-02"), got[0], got[5])
			}
		}
	})

	// (c) Summer school: enrolled June to July of one calendar year.
	t.Run("a summer-school row", func(t *testing.T) {
		got := dayStrings(t, attendanceGrid("<br>E: 06/22/2026 L: 07/24/2026", []string{"6/22-6/26", "7/6-7/10", "7/20-7/24"}, nil), time.Time{})
		want := []string{"Mon 2026-06-22", "Mon 2026-07-06", "Mon 2026-07-20"}
		for i, w := range want {
			if got[i*5] != w {
				t.Errorf("week %d starts %s, want %s", i, got[i*5], w)
			}
		}
	})

	t.Run("a school year spanning New Year", func(t *testing.T) {
		got := dayStrings(t, attendanceGrid(enrolled, []string{"12/15-12/19", "12/29-1/2", "1/5-1/9"}, nil), time.Time{})
		want := []string{
			"Mon 2025-12-15", "Tue 2025-12-16", "Wed 2025-12-17", "Thu 2025-12-18", "Fri 2025-12-19",
			"Mon 2025-12-29", "Tue 2025-12-30", "Wed 2025-12-31", "Thu 2026-01-01", "Fri 2026-01-02",
			"Mon 2026-01-05", "Tue 2026-01-06", "Wed 2026-01-07", "Thu 2026-01-08", "Fri 2026-01-09",
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("days = %v, want %v", got, want)
		}
	})

	t.Run("Feb 29", func(t *testing.T) {
		leap := "<br>E: 09/01/2027 L: 06/15/2028"
		got := dayStrings(t, attendanceGrid(leap, []string{"2/28-3/3"}, nil), time.Time{})
		want := []string{"Mon 2028-02-28", "Tue 2028-02-29", "Wed 2028-03-01", "Thu 2028-03-02", "Fri 2028-03-03"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("days = %v, want %v", got, want)
		}
		// A header that starts on Feb 29 itself (a Tuesday in 2028).
		got = dayStrings(t, attendanceGrid(leap, []string{"2/29-3/3"}, nil), time.Time{})
		if got[1] != "Tue 2028-02-29" || got[0] != "Mon 2028-02-28" {
			t.Errorf("days = %v", got)
		}
		// Feb 29 in a school year that has none is an error, not Mar 1.
		_, err := ParseAttendancePage(attendanceGrid(enrolled, []string{"2/29-3/4"}, nil), time.Time{})
		wantParseError(t, err)
	})

	t.Run("every returned date is on its column's weekday", func(t *testing.T) {
		page, err := ParseAttendancePage(fixture(t, "attendance.html"), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range page.Courses {
			for i, d := range c.Days {
				if want := time.Weekday(i%5 + 1); d.Date.Weekday() != want {
					t.Errorf("%s day %d is a %s, column is %s", c.CourseName, i, d.Date.Weekday(), want)
				}
			}
		}
	})

	t.Run("headers that fit no year are errors", func(t *testing.T) {
		for name, weeks := range map[string][]string{
			"a weekend range":              {"9/6-9/7"},
			"longer than Monday to Friday": {"9/1-9/12"},
			"end before start":             {"9/5-9/1"},
			"starts on a Saturday":         {"9/6-9/10"},
		} {
			_, err := ParseAttendancePage(attendanceGrid(enrolled, weeks, nil), time.Time{})
			if err == nil {
				t.Errorf("%s: parsed without error", name)
				continue
			}
			var perr *Error
			if !errors.As(err, &perr) || perr.Code != CodeParseError {
				t.Errorf("%s: err = %v, want a typed parse error", name, err)
			}
		}
	})
}

// TestAttendance_UnrecognisedWeekHeader (L2): a header the week pattern
// does not match, followed by data rows, is an error -- not a course with
// zero days.
func TestAttendance_UnrecognisedWeekHeader(t *testing.T) {
	sw := switcher("111111")
	for name, html := range map[string]string{
		"a week header in another format": attendanceGrid("<br>E: 09/03/2025 L: 06/17/2026", []string{"Sep 8 - Sep 12"}, nil),
		"one of two headers unreadable":   attendanceGrid("<br>E: 09/03/2025 L: 06/17/2026", []string{"9/8-9/12", "week 3"}, nil),
		"no week headers, two-cell rows":  sw + `<table class="grid"><tr><th>Course</th><th>Expression</th></tr><tr><td>EXAMPLE ART<br>E: 09/03/2025 L: 06/17/2026</td><td>4(A)</td></tr></table>`,
	} {
		t.Run(name, func(t *testing.T) {
			page, err := ParseAttendancePage(html, ymd(2025, 10, 1))
			if err == nil {
				t.Fatalf("parsed without error: %d course(s), %d day(s)", len(page.Courses), len(page.Courses[0].Days))
			}
			wantParseError(t, err)
		})
	}
	t.Run("a grid with headers and no data rows is still an empty page", func(t *testing.T) {
		page, err := ParseAttendancePage(sw+`<table class="grid"><tr><th>Course</th><th>Expression</th><th colspan="5">9/8-9/12</th></tr></table>`, ymd(2025, 10, 1))
		if err != nil || len(page.Courses) != 0 {
			t.Fatalf("page = %+v err = %v", page, err)
		}
	})
}

// TestAttendanceLegend_SplitOnThePagesSeparators (L4): entries are
// separated by span.disclaimer elements; a "|" or "=" inside a description
// is part of the description.
func TestAttendanceLegend_SplitOnThePagesSeparators(t *testing.T) {
	sep := ` <span class="disclaimer">|</span> `
	legend := `<p><strong>Attendance Codes:</strong> Blank=Present` + sep +
		`XA=EA-Excused | Verified` + sep +
		`XB=UA-Late = Unexcused` + sep +
		`XC=AUTH-A|B=C` + sep +
		`A=A-Absent` + sep + `</p>`
	html := switcher("111111") + `<table class="grid"><tr><th>Course</th><th>Expression</th></tr></table><div id="legend">` + legend + `</div>`
	page, err := ParseAttendancePage(html, ymd(2026, 10, 9))
	if err != nil {
		t.Fatalf("ParseAttendancePage: %v", err)
	}
	want := []AttendanceCode{
		{"XA", "EA", "Excused | Verified"},
		{"XB", "UA", "Late = Unexcused"},
		{"XC", "AUTH", "A|B=C"},
		{"A", "A", "Absent"},
	}
	if len(page.Legend) != len(want) {
		t.Fatalf("legend = %+v, want %+v", page.Legend, want)
	}
	for i, w := range want {
		if page.Legend[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, page.Legend[i], w)
		}
	}
}

// TestTeacherComment_PreContent (L3): line breaks written as <br>, script
// and style content, entities, and plain multi-line text.
func TestTeacherComment_PreContent(t *testing.T) {
	base := fixture(t, "teachercomments.html")
	comment := func(t *testing.T, pre string) string {
		t.Helper()
		html := strings.Replace(base, "<td><pre></pre></td>", "<td><pre>"+pre+"</pre></td>", 1)
		page, err := ParseTeacherCommentsPage(html)
		if err != nil {
			t.Fatalf("ParseTeacherCommentsPage: %v", err)
		}
		return page.Comments[0].Comment
	}
	cases := []struct {
		name, pre, want string
	}{
		{"br becomes a newline", "First line.<br>Second line.<br/>Third line.<BR />Fourth.", "First line.\nSecond line.\nThird line.\nFourth."},
		{"script and style content is dropped", "Good effort.<script>var x = 'not a comment';</script><style>.c{color:red}</style> Keep going.", "Good effort. Keep going."},
		{"entities are decoded", "Reads &amp; writes well; scored &gt; 90% &mdash; &quot;excellent&quot;.", "Reads & writes well; scored > 90% \u2014 \"excellent\"."},
		{"multi-line text keeps its line breaks", "Line one.\nLine two.\r\n\r\nLine four.", "Line one.\nLine two.\n\nLine four."},
		{"inline markup keeps its text", "Needs <b>more</b> practice with <i>fractions</i>.", "Needs more practice with fractions."},
		{"only markup and whitespace is empty", " <br> <script>x()</script> ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := comment(t, tc.pre); got != tc.want {
				t.Errorf("comment = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGradeHistory_NotInGPAMarker (L5): the "*" that marks a grade as not
// included in the GPA can come before the letter as well as after it.
func TestGradeHistory_NotInGPAMarker(t *testing.T) {
	cases := []struct {
		cell       string
		wantLetter string
		wantInGPA  bool
	}{
		{"A-<b></b>", "A-", true},
		{"P <b>*</b>", "P", false},
		{"P*", "P", false},
		{"<b>*</b>A", "A", false},
		{"*A", "A", false},
		{"* B+", "B+", false},
		{"*A*", "A", false},
		{"A", "A", true},
	}
	for _, tc := range cases {
		html := switcher("111111") + `<table id="grades-tbl"><thead><tr><th>Date Compl</th><th>Course</th><th>Grade</th></tr></thead>
<tbody><tr><td>06/2026</td><td>EXAMPLE ART</td><td>` + tc.cell + `</td></tr></tbody></table>`
		page, err := ParseGradeHistoryPage(html)
		if err != nil || len(page.Grades) != 1 {
			t.Fatalf("cell %q: page = %+v err = %v", tc.cell, page, err)
		}
		if g := page.Grades[0]; g.LetterGrade != tc.wantLetter || g.IncludedInGPA != tc.wantInGPA {
			t.Errorf("cell %q: letter %q in GPA %v, want %q / %v", tc.cell, g.LetterGrade, g.IncludedInGPA, tc.wantLetter, tc.wantInGPA)
		}
	}
}
