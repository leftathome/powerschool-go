package powerschool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// GetGradeHistory retrieves the grade history (transcript) for a student
// This shows all completed courses with final grades
// URL: https://ps.example.org/guardian/termgrades.html
func (c *Client) GetGradeHistory(ctx context.Context, studentID string) ([]*HistoricalGrade, error) {
	c.logger.Info("Fetching grade history for student: %s", studentID)

	// Ensure we're authenticated
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// Get the grade history page
	// Note: May need to pass studentID as parameter or switch student first
	html, err := c.getPageHTML(ctx, "/guardian/termgrades.html")
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Parsing grade history from HTML")

	grades, err := parseGradeHistory(html)
	if err != nil {
		return nil, WrapParseError(err, "grade history")
	}

	c.logger.Info("Found %d historical grade(s)", len(grades))
	return grades, nil
}

// CalculateGPAFromHistory calculates GPA from historical grades
// Only includes grades that have IncludedInGPA = true
func (c *Client) CalculateGPAFromHistory(ctx context.Context, studentID string) (*GPA, error) {
	c.logger.Info("Calculating GPA from grade history for student: %s", studentID)

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

// parseGradeHistory parses historical grades from the termgrades page
func parseGradeHistory(html string) ([]*HistoricalGrade, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var grades []*HistoricalGrade

	// TODO: Find the actual table with historical grades
	// Expected columns:
	// - Date Completed
	// - Grade Level
	// - School
	// - Course Number
	// - Course Name
	// - Credit Earned
	// - Credit Attempted
	// - Grade (with asterisk if not included in GPA)

	// Placeholder implementation - will need actual HTML structure
	doc.Find("table tr").Each(func(i int, s *goquery.Selection) {
		// Skip header row
		if s.Find("th").Length() > 0 {
			return
		}

		cells := s.Find("td")
		if cells.Length() < 8 {
			return
		}

		grade := &HistoricalGrade{}

		// Parse date completed
		dateStr := strings.TrimSpace(cells.Eq(0).Text())
		if date, err := parseDate(dateStr); err == nil {
			grade.DateCompleted = date
		}

		// Parse grade level
		gradeLevelStr := strings.TrimSpace(cells.Eq(1).Text())
		if gradeLevel, err := strconv.Atoi(gradeLevelStr); err == nil {
			grade.GradeLevel = gradeLevel
		}

		// School
		grade.School = strings.TrimSpace(cells.Eq(2).Text())

		// Course number
		grade.CourseNumber = strings.TrimSpace(cells.Eq(3).Text())

		// Course name
		grade.CourseName = strings.TrimSpace(cells.Eq(4).Text())

		// Credit earned
		creditEarnedStr := strings.TrimSpace(cells.Eq(5).Text())
		if credit, err := strconv.ParseFloat(creditEarnedStr, 64); err == nil {
			grade.CreditEarned = credit
		}

		// Credit attempted
		creditAttemptedStr := strings.TrimSpace(cells.Eq(6).Text())
		if credit, err := strconv.ParseFloat(creditAttemptedStr, 64); err == nil {
			grade.CreditAttempted = credit
		}

		// Letter grade (may have asterisk)
		gradeText := strings.TrimSpace(cells.Eq(7).Text())
		if strings.HasSuffix(gradeText, "*") {
			grade.LetterGrade = strings.TrimSuffix(gradeText, "*")
			grade.IncludedInGPA = false
		} else {
			grade.LetterGrade = gradeText
			grade.IncludedInGPA = true
		}

		if grade.CourseName != "" {
			grades = append(grades, grade)
		}
	})

	return grades, nil
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
