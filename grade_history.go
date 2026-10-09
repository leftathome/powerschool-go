package powerschool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// GradeHistoryPath is the grade history page. Its file name says "term
// grades"; what it holds is the transcript-style list of stored grades.
const GradeHistoryPath = "/guardian/termgrades.html"

// GradeHistoryPage is /guardian/termgrades.html: the stored (final) grade
// of each course the student has completed, with its credit, one row per
// course and completion date. It is not a per-quarter view of the current
// year -- those grades are the Q1..S2 columns of the home page (GetGrades).
type GradeHistoryPage struct {
	// Student is the page's own evidence of whose history this is: the
	// switcher's selected id. (This page's heading names nobody.)
	Student PageStudent
	// Grades are the table's rows, in page order.
	Grades []*HistoricalGrade
}

// GetGradeHistoryPage fetches and parses /guardian/termgrades.html for the
// student the session has selected. Check GradeHistoryPage.Student (and
// read its caveat) before attributing the result.
func (c *Client) GetGradeHistoryPage(ctx context.Context) (*GradeHistoryPage, error) {
	c.logger.Info("Fetching grade history")
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}
	html, err := c.getPageHTML(ctx, GradeHistoryPath)
	if err != nil {
		return nil, err
	}
	page, err := ParseGradeHistoryPage(html)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Found %d historical grade(s)", len(page.Grades))
	return page, nil
}

// GetGradeHistory returns the grade history of a student.
//
// studentID == "" reads whoever the session has selected. Otherwise the
// student is selected first (SelectStudent, which may post the portal's
// switcher form), and the history page's own switcher must mark that
// student; if it marks another the call fails with ErrStudentNotSelected.
func (c *Client) GetGradeHistory(ctx context.Context, studentID string) ([]*HistoricalGrade, error) {
	if studentID != "" {
		if _, err := c.SelectStudent(ctx, studentID); err != nil {
			return nil, err
		}
	}
	page, err := c.GetGradeHistoryPage(ctx)
	if err != nil {
		return nil, err
	}
	if studentID != "" && page.Student.SelectedID != studentID {
		return nil, ErrStudentNotSelected
	}
	return page.Grades, nil
}

// CalculateGPAFromHistory calculates GPA from historical grades
// Only includes grades that have IncludedInGPA = true
func (c *Client) CalculateGPAFromHistory(ctx context.Context, studentID string) (*GPA, error) {
	c.logger.Info("Calculating GPA from grade history")

	history, err := c.GetGradeHistory(ctx, studentID)
	if err != nil {
		return nil, err
	}

	// Calculate GPA
	var totalPoints float64
	var totalCredits float64

	for _, grade := range history {
		if !grade.IncludedInGPA {
			continue
		}

		// Convert letter grade to GPA points
		points := letterGradeToGPA(grade.LetterGrade)
		if points < 0 {
			// Skip grades that don't convert (P, F, etc.)
			continue
		}

		totalPoints += points * grade.CreditAttempted
		totalCredits += grade.CreditAttempted
	}

	gpa := &GPA{
		Cumulative: 0,
		Weighted:   false, // the reference instance may not use weighted GPA
	}

	if totalCredits > 0 {
		gpa.Cumulative = totalPoints / totalCredits
		gpa.Current = gpa.Cumulative // For historical data, current = cumulative
	}

	c.logger.Info("Calculated GPA: %.2f", gpa.Cumulative)
	return gpa, nil
}

// ParseGradeHistoryPage parses the HTML of /guardian/termgrades.html:
// table#grades-tbl with the columns "Date Compl" (MM/YYYY), "Grade Level",
// "School", "Course #", "Course", "Credit Earned", "Credit Attempted" and
// "Grade" (headers verified live). A grade followed by "*" is one the page
// says is not included in the GPA. Fails with ErrSelectionUnknown when the
// page does not mark exactly one student as selected.
func ParseGradeHistoryPage(html string) (*GradeHistoryPage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, WrapParseError(err, "grade history page")
	}
	page := &GradeHistoryPage{Student: pageStudent(doc)}
	if err := requireSelected(page.Student); err != nil {
		return nil, err
	}

	table := doc.Find("table#grades-tbl")
	if table.Length() == 0 {
		return nil, WrapParseError(fmt.Errorf("no grade history table (table#grades-tbl)"), "grade history page")
	}

	// Locate the columns by header text rather than by position.
	col := map[string]int{}
	table.Find("thead th").Each(func(i int, th *goquery.Selection) {
		switch h := strings.ToLower(cleanText(th.Text())); {
		case strings.HasPrefix(h, "date"):
			col["date"] = i
		case strings.HasPrefix(h, "grade level"):
			col["level"] = i
		case h == "school":
			col["school"] = i
		case strings.HasPrefix(h, "course #"), strings.HasPrefix(h, "course number"):
			col["number"] = i
		case h == "course":
			col["course"] = i
		case strings.Contains(h, "earned"):
			col["earned"] = i
		case strings.Contains(h, "attempted"):
			col["attempted"] = i
		case h == "grade":
			col["grade"] = i
		}
	})
	for _, need := range []string{"date", "course", "grade"} {
		if _, ok := col[need]; !ok {
			return nil, WrapParseError(fmt.Errorf("grade history table has no %q column", need), "grade history page")
		}
	}

	table.Find("tbody tr").Each(func(i int, row *goquery.Selection) {
		cells := row.Find("td")
		text := func(name string) string {
			idx, ok := col[name]
			if !ok || idx >= cells.Length() {
				return ""
			}
			return cleanText(cells.Eq(idx).Text())
		}
		grade := &HistoricalGrade{
			School:        text("school"),
			CourseNumber:  text("number"),
			CourseName:    text("course"),
			IncludedInGPA: true,
		}
		if grade.CourseName == "" {
			return
		}
		grade.DateCompletedText = text("date")
		if t, err := time.Parse("1/2006", grade.DateCompletedText); err == nil {
			grade.DateCompleted = t
		} else if t, err := parseDate(grade.DateCompletedText); err == nil {
			grade.DateCompleted = t
		}
		if level, err := strconv.Atoi(text("level")); err == nil {
			grade.GradeLevel = level
		}
		if credit, err := strconv.ParseFloat(text("earned"), 64); err == nil {
			grade.CreditEarned = credit
		}
		if credit, err := strconv.ParseFloat(text("attempted"), 64); err == nil {
			grade.CreditAttempted = credit
		}
		// "*" marks a grade the page says is not included in the GPA. It
		// is printed after the letter; accept it before the letter too.
		letter := strings.ReplaceAll(text("grade"), " ", "")
		if strings.HasPrefix(letter, "*") || strings.HasSuffix(letter, "*") {
			letter = strings.Trim(letter, "*")
			grade.IncludedInGPA = false
		}
		grade.LetterGrade = letter
		page.Grades = append(page.Grades, grade)
	})
	return page, nil
}

// letterGradeToGPA converts a letter grade to GPA points (4.0 scale)
// Returns -1 for grades that don't have GPA equivalents (P, F, etc.)
func letterGradeToGPA(letter string) float64 {
	letter = strings.ToUpper(strings.TrimSpace(letter))

	switch letter {
	case "A", "A+":
		return 4.0
	case "A-":
		return 3.7
	case "B+":
		return 3.3
	case "B":
		return 3.0
	case "B-":
		return 2.7
	case "C+":
		return 2.3
	case "C":
		return 2.0
	case "C-":
		return 1.7
	case "D+":
		return 1.3
	case "D":
		return 1.0
	case "D-":
		return 0.7
	case "F":
		return 0.0
	default:
		// P (pass), or other non-standard grades
		return -1
	}
}

// parseDate attempts to parse a date string in common formats
func parseDate(dateStr string) (time.Time, error) {
	formats := []string{
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"Jan 2, 2006",
		"January 2, 2006",
		"01/02/06",
		"1/2/06",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}
