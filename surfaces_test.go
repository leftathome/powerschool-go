package powerschool

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Everything these tests read is synthetic: the fixtures under testdata/
// were written by hand to the shape of the live pages and contain no real
// name, id, course or comment. The attendance legend uses the code scheme
// observed on the live page, which is district policy vocabulary.

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// switcher renders the page frame's student switcher with the given ids
// marked selected (the live pages mark exactly one).
func switcher(selected ...string) string {
	li := func(id, name string) string {
		attr := ""
		for _, s := range selected {
			if s == id {
				attr = `class="selected"`
			}
		}
		return `<li ` + attr + `><a href="javascript:switchStudent(` + id + `);">` + name + `</a></li>`
	}
	return `<ul id="students-list">` + li("111111", "Ana") + li("111112", "Ben") + `</ul>`
}

// fixtureSwitcher is the switcher as the fixtures carry it.
const fixtureSwitcher = `<ul id="students-list"><li class="selected"><a href="javascript:switchStudent(111111);">Ana</a></li><li ><a href="javascript:switchStudent(111112);">Ben</a></li></ul>`

// reselect returns a fixture with its switcher replaced.
func reselect(t *testing.T, html string, selected ...string) string {
	t.Helper()
	if !strings.Contains(html, fixtureSwitcher) {
		t.Fatal("fixture does not carry the expected switcher")
	}
	return strings.Replace(html, fixtureSwitcher, switcher(selected...), 1)
}

// withoutSwitcher returns a fixture with no switcher at all.
func withoutSwitcher(t *testing.T, html string) string {
	t.Helper()
	if !strings.Contains(html, fixtureSwitcher) {
		t.Fatal("fixture does not carry the expected switcher")
	}
	return strings.Replace(html, fixtureSwitcher, "", 1)
}

func ymd(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// TestPages_SelectedStudentIsRequired: every one of these pages describes
// the session's selected student, and every one carries the switcher. The
// parsers return the id that page marks -- and fail closed, with
// ErrSelectionUnknown, when the page marks nobody or more than one.
func TestPages_SelectedStudentIsRequired(t *testing.T) {
	parsers := map[string]struct {
		fixture string
		parse   func(html string) (PageStudent, error)
	}{
		"attendance": {"attendance.html", func(html string) (PageStudent, error) {
			p, err := ParseAttendancePage(html, time.Time{})
			if err != nil {
				return PageStudent{}, err
			}
			return p.Student, nil
		}},
		"grade history": {"termgrades.html", func(html string) (PageStudent, error) {
			p, err := ParseGradeHistoryPage(html)
			if err != nil {
				return PageStudent{}, err
			}
			return p.Student, nil
		}},
		"teacher comments": {"teachercomments.html", func(html string) (PageStudent, error) {
			p, err := ParseTeacherCommentsPage(html)
			if err != nil {
				return PageStudent{}, err
			}
			return p.Student, nil
		}},
	}
	for name, tc := range parsers {
		t.Run(name, func(t *testing.T) {
			base := fixture(t, tc.fixture)

			got, err := tc.parse(base)
			if err != nil || got.SelectedID != "111111" {
				t.Fatalf("fixture: student = %+v err = %v, want selected 111111", got, err)
			}
			got, err = tc.parse(reselect(t, base, "111112"))
			if err != nil || got.SelectedID != "111112" {
				t.Fatalf("other student marked: student = %+v err = %v, want selected 111112", got, err)
			}
			for variant, html := range map[string]string{
				"nobody marked": reselect(t, base),
				"both marked":   reselect(t, base, "111111", "111112"),
				"no switcher":   withoutSwitcher(t, base),
			} {
				if _, err := tc.parse(html); !errors.Is(err, ErrSelectionUnknown) {
					t.Errorf("%s: err = %v, want ErrSelectionUnknown", variant, err)
				}
			}
		})
	}
}

// --- attendance ---------------------------------------------------------

func TestParseAttendancePage(t *testing.T) {
	page, err := ParseAttendancePage(fixture(t, "attendance.html"), time.Time{})
	if err != nil {
		t.Fatalf("ParseAttendancePage: %v", err)
	}

	if page.Student.SelectedID != "111111" || page.Student.HeadingName != "Rivera, Ana" {
		t.Errorf("student evidence = %+v", page.Student)
	}

	// The grid repeats its header every block; the three courses must come
	// out once each, with the days of both blocks.
	if len(page.Courses) != 3 {
		t.Fatalf("courses = %d, want 3", len(page.Courses))
	}
	wantCourses := []struct {
		name, exp, teacher, room string
		enroll, leave            time.Time
	}{
		{"ADVISORY", "HR(A)", "Okafor, Daniel R", "7", ymd(2025, 9, 3), ymd(2026, 6, 17)},
		{"EXAMPLE SCIENCE 7A", "1(A)", "Lindqvist, Marta", "12", ymd(2025, 9, 3), ymd(2026, 6, 17)},
		{"EXAMPLE MATH 7B", "2(B)", "Abara, Tomas J", "B4", ymd(2026, 1, 27), ymd(2026, 6, 17)},
	}
	for i, want := range wantCourses {
		c := page.Courses[i]
		if c.CourseName != want.name || c.Expression != want.exp || c.Teacher != want.teacher || c.Room != want.room ||
			!c.EnrollDate.Equal(want.enroll) || !c.LeaveDate.Equal(want.leave) {
			t.Errorf("course %d = %+v, want %+v", i, *c, want)
		}
		if len(c.Days) != 20 {
			t.Errorf("course %d has %d days, want 20 (4 weeks x 5)", i, len(c.Days))
		}
	}

	// Dates: the header prints month/day only. Four weeks, one of them
	// straddling the new year.
	wantDates := []time.Time{
		ymd(2025, 9, 1), ymd(2025, 9, 2), ymd(2025, 9, 3), ymd(2025, 9, 4), ymd(2025, 9, 5),
		ymd(2025, 12, 29), ymd(2025, 12, 30), ymd(2025, 12, 31), ymd(2026, 1, 1), ymd(2026, 1, 2),
		ymd(2026, 1, 5), ymd(2026, 1, 6), ymd(2026, 1, 7), ymd(2026, 1, 8), ymd(2026, 1, 9),
		ymd(2026, 6, 15), ymd(2026, 6, 16), ymd(2026, 6, 17), ymd(2026, 6, 18), ymd(2026, 6, 19),
	}
	for ci, c := range page.Courses {
		for i, want := range wantDates {
			if !c.Days[i].Date.Equal(want) {
				t.Errorf("course %d day %d = %s, want %s", ci, i, c.Days[i].Date.Format("2006-01-02"), want.Format("2006-01-02"))
			}
			if wd := c.Days[i].Date.Weekday(); wd == time.Saturday || wd == time.Sunday {
				t.Errorf("course %d day %d falls on a %s", ci, i, wd)
			}
		}
	}

	// Cells of the science row: blank, "-", "." and the codes seen live.
	science := page.Courses[1]
	wantCells := []struct {
		kind AttendanceCellKind
		code string
	}{
		{CellNoMeeting, ""}, {CellNoMeeting, ""}, {CellCode, "L"}, {CellCode, "A"}, {CellCode, "ME"},
		{CellNoMeeting, ""}, {CellNoMeeting, ""}, {CellNoMeeting, ""}, {CellNoMeeting, ""}, {CellPresent, ""},
		{CellPresent, ""}, {CellCode, "VU"}, {CellPresent, ""}, {CellCode, "FT"}, {CellCode, "LE"},
		{CellUnmarked, ""}, {CellUnmarked, ""}, {CellUnmarked, ""}, {CellNoMeeting, ""}, {CellNoMeeting, ""},
	}
	for i, want := range wantCells {
		if got := science.Days[i]; got.Kind != want.kind || got.Code != want.code {
			t.Errorf("science day %d (%s) = %s/%q, want %s/%q", i, got.Date.Format("01-02"), got.Kind, got.Code, want.kind, want.code)
		}
	}

	// Legend: CODE=CATEGORY-Description, read from the page, without the
	// "Blank=Present" entry.
	wantLegend := []AttendanceCode{
		{"FT", "AUTH", "Field Trip"}, {"IA", "AUTH", "On Campus"}, {"OA", "AUTH", "Off Campus"},
		{"AA", "EA", "Approved Activity"}, {"EA", "EA", "Excused Absence"}, {"FE", "EA", "Family Emergency"},
		{"ME", "EA", "Medical/Sick"}, {"QE", "EA", "Health Excused"}, {"SS", "EA", "Suspended"},
		{"EL", "EL", "Excused Late"}, {"LB", "EL", "Late Transport"}, {"LE", "LE", "Left Early Excused"},
		{"LU", "LU", "Left Early Unexcused"}, {"UA", "UA", "Unexcused Absence"}, {"VU", "UA", "Unexcused Travel"},
		{"UL", "UL", "Unexcused Late"}, {"A", "A", "Absent"}, {"L", "L", "Late"},
	}
	if len(page.Legend) != len(wantLegend) {
		t.Fatalf("legend has %d entries, want %d: %+v", len(page.Legend), len(wantLegend), page.Legend)
	}
	for i, want := range wantLegend {
		if page.Legend[i] != want {
			t.Errorf("legend %d = %+v, want %+v", i, page.Legend[i], want)
		}
	}
	if entry, ok := page.Describe("VU"); !ok || entry.Category != "UA" || entry.Description != "Unexcused Travel" {
		t.Errorf("Describe(VU) = (%+v, %v)", entry, ok)
	}
	if _, ok := page.Describe("ZZ"); ok {
		t.Error("Describe found a code that is not in the legend")
	}
	if _, ok := page.Describe("Blank"); ok {
		t.Error("Blank=Present was kept as a code")
	}

	// The page has no totals; they are counted, by the legend's category.
	for category, want := range map[string]int{"A": 1, "L": 1, "EA": 1, "UA": 1, "AUTH": 1, "LE": 1, "EL": 0, "": 0} {
		if got := science.Count(page, category); got != want {
			t.Errorf("science count of category %q = %d, want %d", category, got, want)
		}
	}
	// A code that is not in the legend has no category.
	if got := page.Courses[2].Count(page, ""); got != 1 {
		t.Errorf("math codes without a category = %d, want 1 (ZZ)", got)
	}
}

func TestParseAttendanceLegend_Shapes(t *testing.T) {
	parse := func(p string) []AttendanceCode {
		html := switcher("111111") + `<table class="grid"><tr><th>Course</th><th>Expression</th></tr></table><div id="legend">` + p + `</div>`
		page, err := ParseAttendancePage(html, ymd(2026, 10, 9))
		if err != nil {
			t.Fatalf("ParseAttendancePage: %v", err)
		}
		return page.Legend
	}
	cases := []struct {
		name string
		p    string
		want []AttendanceCode
	}{
		{"category and description", `<p><strong>Attendance Codes:</strong> Blank=Present | ME=EA-Medical/Sick |</p>`,
			[]AttendanceCode{{"ME", "EA", "Medical/Sick"}}},
		{"hyphen inside the description", `<p><strong>Attendance Codes:</strong> XX=CAT-Half-Day Leave</p>`,
			[]AttendanceCode{{"XX", "CAT", "Half-Day Leave"}}},
		{"no category", `<p><strong>Attendance Codes:</strong> P=Present Remote | Q=</p>`,
			[]AttendanceCode{{"P", "", "Present Remote"}, {"Q", "", ""}}},
		{"another legend paragraph is ignored", `<p><strong>Citizenship Codes:</strong> O=O | S=S</p><p><strong>Attendance Codes:</strong> A=A-Absent</p>`,
			[]AttendanceCode{{"A", "A", "Absent"}}},
		{"no attendance legend", `<p><strong>Citizenship Codes:</strong> O=O</p>`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parse(tc.p)
			if len(got) != len(tc.want) {
				t.Fatalf("legend = %+v, want %+v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParseAttendancePage_YearResolution: the header carries no year.
func TestParseAttendancePage_YearResolution(t *testing.T) {
	grid := func(enrollLine string, weeks ...string) string {
		var b strings.Builder
		b.WriteString(switcher("111111") + `<div id="content-main"><h1>Meeting Attendance History: Rivera, Ana</h1><table class="grid"><tr><th rowspan="2">Course</th><th rowspan="2">Expression</th>`)
		for _, w := range weeks {
			b.WriteString(`<th colspan="5">` + w + `</th>`)
		}
		b.WriteString(`</tr><tr>` + strings.Repeat(`<th>M</th><th>T</th><th>W</th><th>H</th><th>F</th>`, len(weeks)) + `</tr>`)
		b.WriteString(`<tr><td>EXAMPLE ART<br>Nakamura, Lena 3` + enrollLine + `</td><td>4(A)</td>` + strings.Repeat(`<td></td>`, 5*len(weeks)) + `</tr></table></div>`)
		return b.String()
	}
	firstDays := func(t *testing.T, html string, ref time.Time) []string {
		t.Helper()
		page, err := ParseAttendancePage(html, ref)
		if err != nil {
			t.Fatalf("ParseAttendancePage: %v", err)
		}
		var out []string
		for i := 0; i < len(page.Courses[0].Days); i += 5 {
			out = append(out, page.Courses[0].Days[i].Date.Format("2006-01-02"))
		}
		return out
	}
	cases := []struct {
		name   string
		enroll string
		ref    time.Time
		weeks  []string
		want   []string
	}{
		{
			name:   "year from the enrolment dates, autumn enrolment",
			enroll: "<br>E: 09/03/2025 L: 06/17/2026",
			weeks:  []string{"8/25-8/29", "12/22-12/26", "12/29-1/2", "1/5-1/9", "6/29-7/3"},
			want:   []string{"2025-08-25", "2025-12-22", "2025-12-29", "2026-01-05", "2026-06-29"},
		},
		{
			name:   "enrolment dates win over a reference date in another school year",
			enroll: "<br>E: 09/03/2025 L: 06/17/2026",
			ref:    ymd(2031, 3, 1),
			weeks:  []string{"9/1-9/5", "1/5-1/9"},
			want:   []string{"2025-09-01", "2026-01-05"},
		},
		{
			name:   "second-semester enrolment: autumn weeks belong to the previous calendar year",
			enroll: "<br>E: 01/27/2026 L: 06/17/2026",
			weeks:  []string{"9/1-9/5", "2/2-2/6"},
			want:   []string{"2025-09-01", "2026-02-02"},
		},
		{
			name:  "no enrolment dates: reference date in the autumn",
			ref:   ymd(2026, 10, 9),
			weeks: []string{"9/7-9/11", "12/28-1/1", "1/4-1/8"},
			want:  []string{"2026-09-07", "2026-12-28", "2027-01-04"},
		},
		{
			name:  "no enrolment dates: reference date in the spring",
			ref:   ymd(2027, 2, 1),
			weeks: []string{"9/7-9/11", "12/28-1/1", "1/4-1/8"},
			want:  []string{"2026-09-07", "2026-12-28", "2027-01-04"},
		},
		{
			name:  "no enrolment dates: reference on new year's day",
			ref:   ymd(2027, 1, 1),
			weeks: []string{"12/28-1/1"},
			want:  []string{"2026-12-28"},
		},
		{
			name:  "direction mark inside the range",
			ref:   ymd(2026, 10, 9),
			weeks: []string{"9/7-&rlm;9/11"},
			want:  []string{"2026-09-07"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstDays(t, grid(tc.enroll, tc.weeks...), tc.ref)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("week starts = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("the week across new year continues into January", func(t *testing.T) {
		page, err := ParseAttendancePage(grid("<br>E: 09/03/2025 L: 06/17/2026", "12/29-1/2"), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, d := range page.Courses[0].Days {
			got = append(got, d.Date.Format("2006-01-02"))
		}
		if want := "2025-12-29,2025-12-30,2025-12-31,2026-01-01,2026-01-02"; strings.Join(got, ",") != want {
			t.Errorf("days = %v, want %s", got, want)
		}
	})

	t.Run("no enrolment dates and no reference date is an error, not a guess", func(t *testing.T) {
		_, err := ParseAttendancePage(grid("", "9/7-9/11"), time.Time{})
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeParseError {
			t.Fatalf("err = %v, want a parse error", err)
		}
	})
}

func TestParseAttendancePage_Malformed(t *testing.T) {
	sw := switcher("111111")
	for name, html := range map[string]string{
		"no grid":            sw + `<div id="content-main"><h1>Meeting Attendance History: Rivera, Ana</h1></div>`,
		"another grid":       sw + `<table class="grid"><tr><th>Exp.</th><th>Teacher</th></tr></table>`,
		"row of wrong width": sw + `<table class="grid"><tr><th>Course</th><th>Expression</th><th colspan="5">9/1-9/5</th></tr><tr><td>X<br>E: 09/03/2025 L: 06/17/2026</td><td>1(A)</td><td></td></tr></table>`,
		"impossible date":    sw + `<table class="grid"><tr><th>Course</th><th>Expression</th><th colspan="5">2/30-3/5</th></tr><tr><td>X<br>E: 09/03/2025 L: 06/17/2026</td><td>1(A)</td><td></td><td></td><td></td><td></td><td></td></tr></table>`,
	} {
		t.Run(name, func(t *testing.T) {
			page, err := ParseAttendancePage(html, ymd(2026, 10, 9))
			var perr *Error
			if !errors.As(err, &perr) || perr.Code != CodeParseError || page != nil {
				t.Fatalf("ParseAttendancePage = (%v, %v), want a parse error", page, err)
			}
		})
	}

	t.Run("a page with no legend parses, with an empty legend", func(t *testing.T) {
		html := fixture(t, "attendance.html")
		html = html[:strings.Index(html, `<div aria-hidden="true" id="legend"`)] + "</div></body></html>"
		page, err := ParseAttendancePage(html, time.Time{})
		if err != nil || len(page.Legend) != 0 || len(page.Courses) != 3 {
			t.Fatalf("page = %+v, err %v", page, err)
		}
		// Without a legend no code has a category.
		if got := page.Courses[1].Count(page, ""); got != 6 {
			t.Errorf("codes without a category = %d, want all 6", got)
		}
	})
}

// --- grade history ------------------------------------------------------

func TestParseGradeHistoryPage(t *testing.T) {
	page, err := ParseGradeHistoryPage(fixture(t, "termgrades.html"))
	if err != nil {
		t.Fatalf("ParseGradeHistoryPage: %v", err)
	}
	// The heading names nobody; the switcher is the evidence.
	if page.Student.SelectedID != "111111" || page.Student.HeadingName != "" {
		t.Errorf("student evidence = %+v, want the switcher id and no heading name", page.Student)
	}
	want := []HistoricalGrade{
		{DateCompleted: ymd(2026, 1, 1), DateCompletedText: "01/2026", GradeLevel: 7, School: "Example MS", CourseNumber: "SCI7001", CourseName: "EXAMPLE SCIENCE 7A", CreditEarned: 0.5, CreditAttempted: 0.5, LetterGrade: "A-", IncludedInGPA: true},
		{DateCompleted: ymd(2026, 1, 1), DateCompletedText: "01/2026", GradeLevel: 7, School: "Example MS", CourseNumber: "MTH7002", CourseName: "EXAMPLE MATH 7A", CreditEarned: 0.5, CreditAttempted: 0.5, LetterGrade: "B+", IncludedInGPA: true},
		{DateCompleted: ymd(2026, 1, 1), DateCompletedText: "01/2026", GradeLevel: 7, School: "Example MS", CourseNumber: "ADV7000", CourseName: "ADVISORY", CreditEarned: 0, CreditAttempted: 0.25, LetterGrade: "P", IncludedInGPA: false},
		{DateCompleted: ymd(2025, 6, 1), DateCompletedText: "06/2025", GradeLevel: 6, School: "Example ES", CourseNumber: "SCI6001", CourseName: "EXAMPLE SCIENCE 6", CreditEarned: 1, CreditAttempted: 1, LetterGrade: "A", IncludedInGPA: true},
	}
	if len(page.Grades) != len(want) {
		t.Fatalf("grades = %d, want %d", len(page.Grades), len(want))
	}
	for i, w := range want {
		if got := *page.Grades[i]; got != w {
			t.Errorf("grade %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestParseGradeHistoryPage_Variants(t *testing.T) {
	sw := switcher("111111")
	t.Run("the live header names", func(t *testing.T) {
		// Verbatim: "Date Compl", "Grade Level", "School", "Course #",
		// "Course", "Credit Earned", "Credit Attempted", "Grade".
		html := fixture(t, "termgrades.html")
		for _, h := range []string{">Date Compl<", ">Grade Level<", ">School<", ">Course #<", ">Course<", ">Credit Earned<", ">Credit Attempted<", ">Grade<"} {
			if !strings.Contains(html, h) {
				t.Errorf("fixture lacks the header %s", h)
			}
		}
	})
	t.Run("no table is a parse error", func(t *testing.T) {
		_, err := ParseGradeHistoryPage(sw + `<div id="content-main"><h1>Grade History and GPA Report</h1></div>`)
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeParseError {
			t.Fatalf("err = %v, want a parse error", err)
		}
	})
	t.Run("an empty table is an empty history", func(t *testing.T) {
		html := fixture(t, "termgrades.html")
		a, b := strings.Index(html, "<tbody>"), strings.Index(html, "</tbody>")
		page, err := ParseGradeHistoryPage(html[:a] + "<tbody>" + html[b:])
		if err != nil || len(page.Grades) != 0 {
			t.Fatalf("page = %+v, err %v", page, err)
		}
	})
	t.Run("columns are found by header, not position", func(t *testing.T) {
		html := sw + `<table id="grades-tbl"><thead><tr><th>Grade</th><th>Course</th><th>Date Compl</th></tr></thead>
<tbody><tr><td>B <b>*</b></td><td>EXAMPLE ART</td><td>06/2026</td></tr></tbody></table>`
		page, err := ParseGradeHistoryPage(html)
		if err != nil || len(page.Grades) != 1 {
			t.Fatalf("page = %+v, err %v", page, err)
		}
		g := page.Grades[0]
		if g.LetterGrade != "B" || g.IncludedInGPA || g.CourseName != "EXAMPLE ART" || !g.DateCompleted.Equal(ymd(2026, 6, 1)) {
			t.Errorf("grade = %+v", *g)
		}
	})
}

// --- teacher comments ---------------------------------------------------

func TestParseTeacherCommentsPage(t *testing.T) {
	page, err := ParseTeacherCommentsPage(fixture(t, "teachercomments.html"))
	if err != nil {
		t.Fatalf("ParseTeacherCommentsPage: %v", err)
	}
	if page.Student.SelectedID != "111111" || page.Student.HeadingName != "Rivera, Ana" {
		t.Errorf("student evidence = %+v", page.Student)
	}
	// Terms come from the fg value of each option's URL; the blank first
	// option is not a term.
	if got := strings.Join(page.Terms, ","); got != "Q1,Q2,Q3,Q4,S1,S2" {
		t.Errorf("terms = %s", got)
	}
	// The bare page marks the current term.
	if page.Term != "S1" || !page.TermConfirmed {
		t.Errorf("term = %q confirmed=%v, want S1 confirmed", page.Term, page.TermConfirmed)
	}
	want := []TeacherComment{
		{Expression: "HR(A)", CourseNumber: "ADV7000", CourseName: "ADVISORY", Teacher: "Okafor, Daniel R", Comment: ""},
		{Expression: "1(A)", CourseNumber: "SCI7001", CourseName: "EXAMPLE SCIENCE 7A", Teacher: "Lindqvist, Marta",
			Comment: "Careful lab work this term.\nPlease review the unit 2 vocabulary before the quiz."},
		{Expression: "2(B)", CourseNumber: "MTH7002", CourseName: "EXAMPLE MATH 7B", Teacher: "Abara, Tomas J", Comment: ""},
	}
	if len(page.Comments) != len(want) {
		t.Fatalf("comments = %d, want %d", len(page.Comments), len(want))
	}
	for i, w := range want {
		if got := *page.Comments[i]; got != w {
			t.Errorf("comment %d = %+v, want %+v", i, got, w)
		}
	}
}

// commentsFor renders the fixture as the portal serves it for ?fg=<term>:
// that term's option is the selected one, and -- the live quirk -- the
// FIRST option's value becomes "...?fg=<term>&showdropped=". term == ""
// is the bare page (S1 selected, first option "fg=&showdropped=").
func commentsFor(html, term string) string {
	if term == "" {
		return html
	}
	html = strings.Replace(html, `fg=S1" selected>`, `fg=S1">`, 1)
	html = strings.Replace(html, `?fg=`+term+`">`, `?fg=`+term+`" selected>`, 1)
	return strings.Replace(html, `teachercomments.html?fg=&showdropped=`, `teachercomments.html?fg=`+term+`&showdropped=`, 1)
}

func TestParseTeacherCommentsPage_TermSelector(t *testing.T) {
	base := fixture(t, "teachercomments.html")

	for _, term := range []string{"Q1", "Q2", "Q3", "Q4", "S1", "S2"} {
		t.Run("requested "+term, func(t *testing.T) {
			page, err := ParseTeacherCommentsPage(commentsFor(base, term))
			if err != nil {
				t.Fatalf("ParseTeacherCommentsPage: %v", err)
			}
			if page.Term != term || !page.TermConfirmed {
				t.Errorf("term = %q confirmed=%v, want %s confirmed", page.Term, page.TermConfirmed, term)
			}
			// The first option now reads "?fg=<term>&showdropped=": it
			// must not be mistaken for a (second) term entry.
			if got := strings.Join(page.Terms, ","); got != "Q1,Q2,Q3,Q4,S1,S2" {
				t.Errorf("terms = %s, want each term exactly once", got)
			}
		})
	}

	t.Run("the first option marked selected is not a term", func(t *testing.T) {
		html := strings.Replace(base, `fg=S1" selected>`, `fg=S1">`, 1)
		html = strings.Replace(html, `<option value="teachercomments.html?fg=&showdropped=">`, `<option value="teachercomments.html?fg=S1&showdropped=" selected>`, 1)
		page, err := ParseTeacherCommentsPage(html)
		if err != nil {
			t.Fatal(err)
		}
		if page.Term != "" || page.TermConfirmed {
			t.Errorf("term = %q confirmed=%v, want none: the show-dropped option is not a term", page.Term, page.TermConfirmed)
		}
	})
	t.Run("no option marked", func(t *testing.T) {
		page, err := ParseTeacherCommentsPage(strings.Replace(base, `fg=S1" selected>`, `fg=S1">`, 1))
		if err != nil || page.Term != "" || page.TermConfirmed {
			t.Fatalf("term = %q confirmed=%v err=%v, want none", page.Term, page.TermConfirmed, err)
		}
	})
}

func TestOptionTerm(t *testing.T) {
	cases := []struct{ value, label, want string }{
		{"teachercomments.html?fg=Q1", "Q1", "Q1"},
		{"teachercomments.html?fg=S2", "", "S2"},
		{"/guardian/teachercomments.html?fg=Q3&x=1", "Q3", "Q3"},
		{"teachercomments.html?fg=&showdropped=", "", ""},
		{"teachercomments.html?fg=S1&showdropped=", "", ""},
		{"teachercomments.html?fg=S1&showdropped=true", "S1", ""},
		{"teachercomments.html?showdropped=&fg=S1", "S1", ""},
		{"teachercomments.html?fg=", "Q1", ""},
		{"teachercomments.html?storecode=Q1", "Q1", ""},
		{"teachercomments.html?fg=Q1%26x", "Q1", ""},
		{"Q1", "Q1", "Q1"},
		{"", "Q2", "Q2"},
		{"", "--", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := optionTerm(tc.value, tc.label); got != tc.want {
			t.Errorf("optionTerm(%q, %q) = %q, want %q", tc.value, tc.label, got, tc.want)
		}
	}
}

func TestParseTeacherCommentsPage_Variants(t *testing.T) {
	base := fixture(t, "teachercomments.html")
	t.Run("no table is a parse error", func(t *testing.T) {
		_, err := ParseTeacherCommentsPage(switcher("111111") + `<div id="content-main"><h1>Teacher Comments: Rivera, Ana</h1></div>`)
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeParseError {
			t.Fatalf("err = %v, want a parse error", err)
		}
	})
	t.Run("a comment outside a pre is still read", func(t *testing.T) {
		html := strings.Replace(base, "<td><pre></pre></td>", "<td>See me after class.</td>", 1)
		page, err := ParseTeacherCommentsPage(html)
		if err != nil || page.Comments[0].Comment != "See me after class." {
			t.Fatalf("comment = %q err=%v", page.Comments[0].Comment, err)
		}
	})
}

// --- the client methods -------------------------------------------------

// surfacesPortal serves the three pages for one selected student and
// records every request.
type surfacesPortal struct {
	t        *testing.T
	mu       sync.Mutex
	requests []string
	// selected is the student every page's switcher marks.
	selected string
	// ignoreTerm makes teachercomments.html ignore ?fg= and serve the bare
	// page (what the portal does with a parameter it does not know).
	ignoreTerm bool
	login      bool
}

func (p *surfacesPortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.requests = append(p.requests, r.Method+" "+r.URL.RequestURI())
	p.mu.Unlock()
	if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "test-session" {
		http.Error(w, "no session cookie", http.StatusForbidden)
		return
	}
	if p.login {
		_, _ = w.Write([]byte(syntheticLoginPage))
		return
	}
	selected := p.selected
	if selected == "" {
		selected = "111111"
	}
	serve := func(html string) {
		_, _ = w.Write([]byte(reselect(p.t, html, selected)))
	}
	switch r.URL.Path {
	case AttendancePath:
		serve(fixture(p.t, "attendance.html"))
	case GradeHistoryPath:
		serve(fixture(p.t, "termgrades.html"))
	case TeacherCommentsPath:
		html := fixture(p.t, "teachercomments.html")
		if !p.ignoreTerm {
			html = commentsFor(html, r.URL.Query().Get("fg"))
		}
		serve(html)
	default:
		http.NotFound(w, r)
	}
}

func (p *surfacesPortal) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requests...)
}

// TestSurfaces_FetchedWithGETThroughTheInjectedClient: each new read is
// one plain GET of its page, carrying the session cookie, through the
// client's http.Client -- no browser, no other method -- and each result
// carries the student that page marked.
func TestSurfaces_FetchedWithGETThroughTheInjectedClient(t *testing.T) {
	ctx := context.Background()
	portal := &surfacesPortal{t: t, selected: "111112"}
	client := newHTTPPageClient(t, portal)

	att, err := client.GetAttendance(ctx)
	if err != nil || len(att.Courses) != 3 || att.Student.SelectedID != "111112" {
		t.Fatalf("GetAttendance = (%+v, %v)", att, err)
	}
	hist, err := client.GetGradeHistoryPage(ctx)
	if err != nil || len(hist.Grades) != 4 || hist.Student.SelectedID != "111112" {
		t.Fatalf("GetGradeHistoryPage = (%+v, %v)", hist, err)
	}
	def, err := client.GetTeacherComments(ctx, "")
	if err != nil || len(def.Comments) != 3 || def.Term != "S1" || !def.TermConfirmed || def.Student.SelectedID != "111112" {
		t.Fatalf("GetTeacherComments(bare) = (%+v, %v), want the current term S1", def, err)
	}
	q2, err := client.GetTeacherComments(ctx, "Q2")
	if err != nil || q2.Term != "Q2" || !q2.TermConfirmed {
		t.Fatalf("GetTeacherComments(Q2) = (%+v, %v), want Q2 confirmed", q2, err)
	}

	want := []string{
		"GET /guardian/attendance.html",
		"GET /guardian/termgrades.html",
		"GET /guardian/teachercomments.html",
		"GET /guardian/teachercomments.html?fg=Q2",
	}
	if got := portal.seen(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests = %v, want %v", got, want)
	}
}

func TestGetTeacherComments_TermHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("the term is requested with fg, never storecode", func(t *testing.T) {
		if TeacherCommentsTermParam != "fg" {
			t.Fatalf("TeacherCommentsTermParam = %q, want fg", TeacherCommentsTermParam)
		}
		portal := &surfacesPortal{t: t}
		client := newHTTPPageClient(t, portal)
		if _, err := client.GetTeacherComments(ctx, "Q4"); err != nil {
			t.Fatalf("GetTeacherComments: %v", err)
		}
		if got := portal.seen(); len(got) != 1 || got[0] != "GET /guardian/teachercomments.html?fg=Q4" {
			t.Errorf("requests = %v", got)
		}
	})
	t.Run("page shows another term: an error, not mislabelled comments", func(t *testing.T) {
		// The portal ignores the parameter and serves the current term.
		client := newHTTPPageClient(t, &surfacesPortal{t: t, ignoreTerm: true})
		page, err := client.GetTeacherComments(ctx, "Q3")
		if !errors.Is(err, ErrTermMismatch) || page != nil {
			t.Fatalf("GetTeacherComments = (%v, %v), want ErrTermMismatch", page, err)
		}
	})
	t.Run("requesting the term the bare page shows anyway is confirmed", func(t *testing.T) {
		client := newHTTPPageClient(t, &surfacesPortal{t: t, ignoreTerm: true})
		page, err := client.GetTeacherComments(ctx, "S1")
		if err != nil || page.Term != "S1" || !page.TermConfirmed {
			t.Fatalf("GetTeacherComments(S1) = (%+v, %v)", page, err)
		}
	})
	t.Run("a malformed term is refused before any request", func(t *testing.T) {
		portal := &surfacesPortal{t: t}
		client := newHTTPPageClient(t, portal)
		for _, term := range []string{"Q1&x=1", "Q 1", "../x", "QUARTERONE", "Q1?"} {
			if _, err := client.GetTeacherComments(ctx, term); !IsNotFound(err) {
				t.Errorf("GetTeacherComments(%q) = %v, want not found", term, err)
			}
		}
		if got := portal.seen(); len(got) != 0 {
			t.Errorf("requests made for malformed terms: %v", got)
		}
	})
}

// TestSurfaces_LoginPageIsSessionExpired: a dead session answered with the
// sign-in form must not parse as "no attendance" / "no comments".
func TestSurfaces_LoginPageIsSessionExpired(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *Client) error{
		"GetAttendance":       func(c *Client) error { _, err := c.GetAttendance(ctx); return err },
		"GetGradeHistoryPage": func(c *Client) error { _, err := c.GetGradeHistoryPage(ctx); return err },
		"GetTeacherComments":  func(c *Client) error { _, err := c.GetTeacherComments(ctx, "Q1"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			client := newHTTPPageClient(t, &surfacesPortal{t: t, login: true})
			if err := call(client); !IsSessionExpired(err) {
				t.Fatalf("err = %v, want session expired", err)
			}
		})
	}
}

// TestGetGradeHistory_ChecksThePagesOwnStudent: asking for a student's
// history selects that student first, and the history page's own switcher
// must then mark that student.
func TestGetGradeHistory_ChecksThePagesOwnStudent(t *testing.T) {
	ctx := context.Background()
	// mount serves the history page marking whoever `who` returns.
	mount := func(portal *fakePortal, who func() string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == GradeHistoryPath {
				_, _ = w.Write([]byte(reselect(t, fixture(t, "termgrades.html"), who())))
				return
			}
			portal.ServeHTTP(w, r)
		})
	}
	current := func(portal *fakePortal) func() string {
		return func() string {
			portal.mu.Lock()
			defer portal.mu.Unlock()
			return portal.selected
		}
	}

	t.Run("switches, then reads that student's page", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, mount(portal, current(portal)))
		grades, err := client.GetGradeHistory(ctx, "111112")
		if err != nil || len(grades) != 4 {
			t.Fatalf("GetGradeHistory = (%d grades, %v)", len(grades), err)
		}
		if _, posts, selected := portal.snapshot(); len(posts) != 1 || selected != "111112" {
			t.Errorf("posts = %d, selected = %q", len(posts), selected)
		}
	})
	t.Run("the history page marks another student: no history under the wrong id", func(t *testing.T) {
		portal := &fakePortal{selected: "111112"}
		client := newHTTPPageClient(t, mount(portal, func() string { return "111111" }))
		grades, err := client.GetGradeHistory(ctx, "111112")
		if !errors.Is(err, ErrStudentNotSelected) || grades != nil {
			t.Fatalf("GetGradeHistory = (%v, %v), want ErrStudentNotSelected", grades, err)
		}
	})
	t.Run("empty id reads whoever is selected, one request", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, mount(portal, current(portal)))
		grades, err := client.GetGradeHistory(ctx, "")
		if err != nil || len(grades) != 4 {
			t.Fatalf("GetGradeHistory = (%d grades, %v)", len(grades), err)
		}
		if requests, _, _ := portal.snapshot(); len(requests) != 0 {
			t.Errorf("home page requested %v for an unchecked read", requests)
		}
	})
}

// TestProgressReports_NotImplemented: typed error, no request, no empty
// list that reads as "no reports".
func TestProgressReports_NotImplemented(t *testing.T) {
	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	for _, httpPages := range []bool{true, false} {
		client.SetHTTPPageFetch(httpPages)
		reports, err := client.GetProgressReports(context.Background(), "111111")
		if !errors.Is(err, ErrNotImplemented) || reports != nil {
			t.Errorf("GetProgressReports = (%v, %v), want ErrNotImplemented", reports, err)
		}
		if _, err := client.DownloadProgressReport(context.Background(), &ProgressReport{}); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("DownloadProgressReport = %v, want ErrNotImplemented", err)
		}
	}
}

// TestHomePage_ReportsTheSameEvidence: the home page reports its selected
// student in the same form as the other pages.
func TestHomePage_ReportsTheSameEvidence(t *testing.T) {
	html := strings.Replace(homePageHTML("111112"), `<table class="student-demo">`,
		`<div id="content-main"><h1>Grades and Attendance: Rivera, Ben</h1></div><table class="student-demo">`, 1)
	page, err := parseHomePage(html)
	if err != nil {
		t.Fatal(err)
	}
	if page.Student.SelectedID != "111112" || page.Student.HeadingName != "Rivera, Ben" {
		t.Errorf("home page evidence = %+v", page.Student)
	}
}
