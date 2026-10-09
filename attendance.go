package powerschool

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// AttendancePath is the meeting attendance history page.
const AttendancePath = "/guardian/attendance.html"

// AttendancePage is /guardian/attendance.html: for each course, one cell
// per school day (Monday to Friday) of the term, holding an attendance code
// when the student was marked anything but present.
//
// The page has no totals; count the days yourself (AttendanceCourse.Count).
// Parsing fails with ErrSelectionUnknown when the page does not mark exactly
// one student as selected.
type AttendancePage struct {
	// Student is the page's own evidence of whose attendance this is. See
	// PageStudent.
	Student PageStudent
	// Courses are the page's rows, in page order.
	Courses []*AttendanceCourse
	// Legend maps the codes used in the cells to their descriptions, in
	// page order. Empty when the page carries no legend.
	Legend []AttendanceCode
}

// AttendanceCourse is one course's row.
type AttendanceCourse struct {
	// CourseName is the course title.
	CourseName string
	// Expression is the schedule slot, e.g. "3(A)".
	Expression string
	// Teacher is the teacher as the page prints it ("Last, First M").
	Teacher string
	// Room is the trailing token of the teacher line when it contains a
	// digit, which is how the page prints the room. Empty otherwise.
	Room string
	// EnrollDate and LeaveDate are the "E:" and "L:" dates of the row: the
	// student's enrolment span in the course. Zero when not present.
	EnrollDate time.Time
	LeaveDate  time.Time
	// Days has one entry per day column, in date order.
	Days []AttendanceDay
}

// AttendanceCellKind says what a day cell holds.
type AttendanceCellKind string

const (
	// CellPresent: an empty cell. The page's legend reads "Blank=Present".
	CellPresent AttendanceCellKind = "present"
	// CellNoMeeting: "-". Not in the page's legend. A non-event: no class
	// for this student that day. Never an attendance code.
	CellNoMeeting AttendanceCellKind = "no_meeting"
	// CellUnmarked: ".". Not in the page's legend. A non-event: nothing
	// marked. Never an attendance code.
	CellUnmarked AttendanceCellKind = "unmarked"
	// CellCode: an attendance code; see AttendanceDay.Code and the legend.
	CellCode AttendanceCellKind = "code"
)

// AttendanceDay is one day cell of a course row.
type AttendanceDay struct {
	// Date is the school day, at midnight UTC.
	Date time.Time
	// Kind classifies the cell.
	Kind AttendanceCellKind
	// Code is the attendance code when Kind is CellCode, else "".
	Code string
	// Raw is the cell text as printed.
	Raw string
}

// AttendanceCode is one legend entry. The legend prints each as
// "CODE=CATEGORY-Description", e.g. "ME=EA-Medical/Sick": several codes
// share a category. Codes, categories and descriptions are the district's.
type AttendanceCode struct {
	// Code is what appears in the grid's cells, e.g. "ME".
	Code string
	// Category is the part of the legend text before the first hyphen,
	// e.g. "EA". Empty when the text has no hyphen.
	Category string
	// Description is the rest, e.g. "Medical/Sick".
	Description string
}

// Describe returns the legend entry for code.
func (p *AttendancePage) Describe(code string) (AttendanceCode, bool) {
	if p == nil {
		return AttendanceCode{}, false
	}
	for _, entry := range p.Legend {
		if entry.Code == code {
			return entry, true
		}
	}
	return AttendanceCode{}, false
}

// Count returns how many of the course's days carry a code whose legend
// category is category. A code missing from the legend has category "".
func (c *AttendanceCourse) Count(page *AttendancePage, category string) int {
	n := 0
	for _, d := range c.Days {
		if d.Kind != CellCode {
			continue
		}
		entry, _ := page.Describe(d.Code)
		if entry.Category == category {
			n++
		}
	}
	return n
}

// GetAttendance fetches and parses /guardian/attendance.html for the
// student the session has selected. Check AttendancePage.Student before
// attributing the result.
//
// The page prints its week headers as month/day without a year. The year is
// the one that puts the header's days on school days and, where the row has
// enrolment dates, inside the enrolment (see resolveWeek); a row without
// them falls back to the school year containing the current date. Use
// ParseAttendancePage to supply that reference date yourself. A header that
// cannot be placed is a parse error, never a guessed date.
func (c *Client) GetAttendance(ctx context.Context) (*AttendancePage, error) {
	c.logger.Info("Fetching attendance")
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}
	html, err := c.getPageHTML(ctx, AttendancePath)
	if err != nil {
		return nil, err
	}
	return ParseAttendancePage(html, time.Now())
}

var (
	weekRangeRE  = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})-(\d{1,2})/(\d{1,2})$`)
	enrollLineRE = regexp.MustCompile(`E:\s*(\d{1,2}/\d{1,2}/\d{4})\s+L:\s*(\d{1,2}/\d{1,2}/\d{4})`)
)

// attendanceWeek is one "M/D-M/D" header cell: the first and last day the
// header names. The five day columns under it are Monday to Friday.
type attendanceWeek struct {
	startMonth, startDay int
	endMonth, endDay     int
}

// weekMonday places a header on the calendar in year and returns the
// Monday of its week. ok is false when the header cannot be that week in
// that year: a day that does not exist (Feb 29), a start or end on a
// weekend, an end before the start, or a span longer than Monday to
// Friday. A header may start or end mid-week (a short school week).
func weekMonday(week attendanceWeek, year int) (monday time.Time, ok bool) {
	start := time.Date(year, time.Month(week.startMonth), week.startDay, 0, 0, 0, 0, time.UTC)
	if int(start.Month()) != week.startMonth || start.Day() != week.startDay {
		return time.Time{}, false
	}
	endYear := year
	if week.endMonth < week.startMonth {
		endYear++ // the week runs over New Year
	}
	end := time.Date(endYear, time.Month(week.endMonth), week.endDay, 0, 0, 0, 0, time.UTC)
	if int(end.Month()) != week.endMonth || end.Day() != week.endDay {
		return time.Time{}, false
	}
	if start.Weekday() < time.Monday || start.Weekday() > time.Friday {
		return time.Time{}, false
	}
	span := int(end.Sub(start).Hours() / 24)
	if span < 0 || int(start.Weekday())+span > int(time.Friday) {
		return time.Time{}, false
	}
	return start.AddDate(0, 0, -(int(start.Weekday()) - int(time.Monday))), true
}

// resolveWeek turns a header's month/day range into the Monday of its
// week. The page does not print the year, so the year is whichever one
// puts the header's days on school days:
//
//  1. Candidate years are those in which the header is a valid Monday-to-
//     Friday span (weekMonday) -- most years are ruled out by the weekday
//     alone -- and which lie within a year of the enrolment or reference.
//  2. If the row has both enrolment dates and exactly one candidate puts
//     the week inside [enrol - 7 days, leave + 7 days], that is the year.
//  3. Otherwise the school-year rule: a school year runs July to June, so
//     July to December belong to the year it starts in and January to June
//     to the next; the starting year comes from the row's enrolment date,
//     or from reference when the row has none. It is used only if it is a
//     candidate.
//  4. Otherwise, if exactly one candidate remains, that is the year.
//  5. Otherwise it is an error. A wrong date is never returned.
func resolveWeek(week attendanceWeek, course *AttendanceCourse, reference time.Time) (time.Time, error) {
	label := fmt.Sprintf("%d/%d-%d/%d", week.startMonth, week.startDay, week.endMonth, week.endDay)
	anchor := course.EnrollDate
	if anchor.IsZero() {
		anchor = reference
	}
	if anchor.IsZero() {
		return time.Time{}, fmt.Errorf("cannot place week %s in a year: the row has no enrolment dates and no reference date was given", label)
	}
	lastYear := anchor.Year() + 1
	if !course.LeaveDate.IsZero() && course.LeaveDate.Year()+1 > lastYear {
		lastYear = course.LeaveDate.Year() + 1
	}
	// The page shows one school year. A week more than a year before the
	// enrolment (or reference) or more than a year after its end is not on
	// it, whatever the weekdays say.
	earliest, latest := anchor.AddDate(-1, 0, -1), anchor.AddDate(1, 0, 1)
	if course.EnrollDate.IsZero() {
		// A reference date can be up to a summer past the year shown.
		earliest = anchor.AddDate(-1, -6, 0)
	} else if !course.LeaveDate.IsZero() {
		latest = course.LeaveDate.AddDate(1, 0, 1)
	}
	candidates := map[int]time.Time{}
	for year := anchor.Year() - 2; year <= lastYear; year++ {
		monday, ok := weekMonday(week, year)
		if !ok || monday.Before(earliest) || monday.After(latest) {
			continue
		}
		candidates[year] = monday
	}
	if len(candidates) == 0 {
		return time.Time{}, fmt.Errorf("attendance week header %s is not a Monday-to-Friday span in any year near %d", label, anchor.Year())
	}

	if !course.EnrollDate.IsZero() && !course.LeaveDate.IsZero() {
		from, to := course.EnrollDate.AddDate(0, 0, -7), course.LeaveDate.AddDate(0, 0, 7)
		var inside []time.Time
		for year, monday := range candidates {
			first := time.Date(year, time.Month(week.startMonth), week.startDay, 0, 0, 0, 0, time.UTC)
			if !first.Before(from) && !first.After(to) {
				inside = append(inside, monday)
			}
		}
		if len(inside) == 1 {
			return inside[0], nil
		}
	}

	startYear := anchor.Year()
	if anchor.Month() < time.July {
		startYear--
	}
	year := startYear
	if time.Month(week.startMonth) < time.July {
		year++
	}
	if monday, ok := candidates[year]; ok {
		return monday, nil
	}
	if len(candidates) == 1 {
		for _, monday := range candidates {
			return monday, nil
		}
	}
	return time.Time{}, fmt.Errorf("attendance week header %s fits more than one year and none of them is the expected school year", label)
}

// ParseAttendancePage parses the HTML of /guardian/attendance.html.
//
// reference is used only to choose a year for rows that carry no enrolment
// dates. Every returned day is on the weekday of its column (the grid's
// columns are Monday to Friday); a week header that does not fit, a header
// column that is not a week range, or course rows with no week columns at
// all are parse errors.
func ParseAttendancePage(html string, reference time.Time) (*AttendancePage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, WrapParseError(err, "attendance page")
	}
	page := &AttendancePage{Student: pageStudent(doc), Legend: parseAttendanceLegend(doc)}
	if err := requireSelected(page.Student); err != nil {
		return nil, err
	}

	var table *goquery.Selection
	doc.Find("table.grid").EachWithBreak(func(i int, t *goquery.Selection) bool {
		header := cleanText(t.Find("tr").First().Text())
		if strings.Contains(header, "Course") && strings.Contains(header, "Expression") {
			table = t
			return false
		}
		return true
	})
	if table == nil {
		return nil, WrapParseError(fmt.Errorf("no attendance grid (table.grid with Course / Expression headers)"), "attendance page")
	}

	byKey := map[string]*AttendanceCourse{}
	var weeks []attendanceWeek
	var parseErr error
	// The grid repeats its two header rows every few weeks, each followed
	// by one row per course for those weeks.
	table.Find("tr").EachWithBreak(func(i int, row *goquery.Selection) bool {
		ths := row.Find("th")
		if ths.Length() > 0 {
			if cleanText(ths.First().Text()) != "Course" {
				return true // the day-letter row
			}
			weeks = weeks[:0]
			ths.EachWithBreak(func(j int, th *goquery.Selection) bool {
				text := cleanText(th.Text())
				if text == "Course" || text == "Expression" {
					return true
				}
				m := weekRangeRE.FindStringSubmatch(strings.ReplaceAll(text, " ", ""))
				if m == nil {
					// A column this parser cannot date. Reading on would
					// silently drop its days, or shift the ones after it.
					parseErr = fmt.Errorf("attendance header column %d is not a week range (month/day-month/day)", j+1)
					return false
				}
				var w attendanceWeek
				w.startMonth, _ = strconv.Atoi(m[1])
				w.startDay, _ = strconv.Atoi(m[2])
				w.endMonth, _ = strconv.Atoi(m[3])
				w.endDay, _ = strconv.Atoi(m[4])
				weeks = append(weeks, w)
				return true
			})
			return parseErr == nil
		}

		cells := row.Find("td")
		if cells.Length() < 2 {
			return true
		}
		if len(weeks) == 0 {
			parseErr = fmt.Errorf("attendance grid has course rows but no recognised week columns")
			return false
		}
		if cells.Length() != 2+5*len(weeks) {
			parseErr = fmt.Errorf("attendance row has %d cells, want %d for %d week(s)", cells.Length(), 2+5*len(weeks), len(weeks))
			return false
		}
		course := parseAttendanceCourseCell(cells.Eq(0))
		course.Expression = cleanText(cells.Eq(1).Text())
		key := strings.Join([]string{course.CourseName, course.Expression, course.Teacher, course.Room,
			course.EnrollDate.Format("2006-01-02"), course.LeaveDate.Format("2006-01-02")}, "\x1f")
		existing, seen := byKey[key]
		if !seen {
			existing = course
			byKey[key] = course
			page.Courses = append(page.Courses, course)
		}
		for w, week := range weeks {
			monday, err := resolveWeek(week, existing, reference)
			if err != nil {
				parseErr = err
				return false
			}
			for d := 0; d < 5; d++ {
				date := monday.AddDate(0, 0, d)
				// The columns are Monday to Friday; a date that is not on
				// its column's weekday must never leave this function.
				if date.Weekday() != time.Monday+time.Weekday(d) {
					parseErr = fmt.Errorf("attendance column %d of week %d/%d resolved to a %s", d+1, week.startMonth, week.startDay, date.Weekday())
					return false
				}
				raw := cleanText(cells.Eq(2 + w*5 + d).Text())
				existing.Days = append(existing.Days, attendanceDay(date, raw))
			}
		}
		return true
	})
	if parseErr != nil {
		return nil, WrapParseError(parseErr, "attendance page")
	}
	return page, nil
}

// attendanceDay classifies one cell.
func attendanceDay(date time.Time, raw string) AttendanceDay {
	day := AttendanceDay{Date: date, Raw: raw}
	switch raw {
	case "":
		day.Kind = CellPresent
	case "-":
		day.Kind = CellNoMeeting
	case ".":
		day.Kind = CellUnmarked
	default:
		day.Kind = CellCode
		day.Code = raw
	}
	return day
}

// parseAttendanceCourseCell reads the first cell of a row: the course
// name, the teacher line and the enrolment dates, separated by <br>.
func parseAttendanceCourseCell(cell *goquery.Selection) *AttendanceCourse {
	course := &AttendanceCourse{}
	html, _ := cell.Html()
	var lines []string
	for _, part := range regexp.MustCompile(`(?i)<br\s*/?>`).Split(html, -1) {
		if text := cleanText(stripHTMLTags(part)); text != "" {
			lines = append(lines, text)
		}
	}
	for i, line := range lines {
		if m := enrollLineRE.FindStringSubmatch(line); m != nil {
			course.EnrollDate, _ = time.Parse("1/2/2006", m[1])
			course.LeaveDate, _ = time.Parse("1/2/2006", m[2])
			continue
		}
		switch {
		case i == 0:
			course.CourseName = line
		case course.Teacher == "":
			course.Teacher = line
			// The page prints the room after the teacher's name.
			if idx := strings.LastIndex(line, " "); idx > 0 && strings.ContainsAny(line[idx+1:], "0123456789") {
				course.Teacher, course.Room = strings.TrimSpace(line[:idx]), line[idx+1:]
			}
		}
	}
	return course
}

// legendSeparator stands in for the legend's separator elements while the
// entries are split.
const legendSeparator = "[[powerschool-go-legend-separator]]"

// parseAttendanceLegend reads "#legend": "Attendance Codes: Blank=Present |
// CODE=CATEGORY-Description | ...". The legend is read from the page, never
// assumed: the vocabulary is district policy.
func parseAttendanceLegend(doc *goquery.Document) []AttendanceCode {
	var legend []AttendanceCode
	doc.Find("#legend p").EachWithBreak(func(i int, p *goquery.Selection) bool {
		label := cleanText(p.Find("strong").First().Text())
		if !strings.Contains(strings.ToLower(label), "attendance") {
			return true
		}
		// Entries are separated by <span class="disclaimer">|</span>
		// elements. Split there, not on the "|" character, which a
		// description may contain.
		entries := p.Clone()
		entries.Find("strong").First().Remove()
		separator := legendSeparator
		if seps := entries.Find("span.disclaimer"); seps.Length() > 0 {
			seps.ReplaceWithHtml(legendSeparator)
		} else {
			// No separator elements at all: fall back to the character.
			separator = "|"
		}
		for _, part := range strings.Split(entries.Text(), separator) {
			part = cleanText(part)
			code, desc, ok := strings.Cut(part, "=")
			code, desc = strings.TrimSpace(code), strings.TrimSpace(desc)
			if !ok || code == "" || strings.EqualFold(code, "blank") {
				continue
			}
			entry := AttendanceCode{Code: code, Description: desc}
			if category, rest, hyphen := strings.Cut(desc, "-"); hyphen {
				entry.Category, entry.Description = strings.TrimSpace(category), strings.TrimSpace(rest)
			}
			legend = append(legend, entry)
		}
		return false
	})
	return legend
}
