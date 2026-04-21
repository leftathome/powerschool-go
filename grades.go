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

// GetGrades returns grades for the currently-selected student
// This parses the grades table from the home page
// Note: This returns grades for whichever student is currently displayed on the home page
func (c *Client) GetGrades(ctx context.Context, studentID string) ([]*Grade, error) {
	c.logger.Info("Fetching grades for student ID: %s", studentID)

	// Ensure we're authenticated
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// Grades are on the home page
	// The home page shows the currently-selected student
	// TODO: In future, implement student switching to get specific student's grades
	html, err := c.getPageHTML(ctx, "/guardian/home.html")
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Parsing grades from home page HTML")

	grades, err := parseGradesFromHomePage(html)
	if err != nil {
		return nil, WrapParseError(err, "grades")
	}

	c.logger.Info("Found %d grade(s)", len(grades))
	return grades, nil
}

// GetGPA returns GPA information for a student
func (c *Client) GetGPA(ctx context.Context, studentID string) (*GPA, error) {
	// GPA is often shown on the main grades page or a separate GPA page
	path := fmt.Sprintf("/guardian/home.html?frn=%s", studentID)

	html, err := c.getPageHTML(ctx, path)
	if err != nil {
		return nil, err
	}

	gpa, err := parseGPA(html)
	if err != nil {
		return nil, WrapParseError(err, "GPA")
	}

	return gpa, nil
}

// parseGradesFromHomePage parses grade information from the reference instance home page
// Table structure: table#tblgrades with columns:
// Exp | Last Week (M-F) | This Week (M-F) | Course | Q1 | Q2 | S1 | Q3 | Q4 | S2 | Absences | Tardies
func parseGradesFromHomePage(html string) ([]*Grade, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var grades []*Grade

	// the reference instance uses table#tblgrades with tr.center for data rows
	doc.Find("table#tblgrades tr.center").Each(func(i int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 13 {
			// Not enough columns, skip
			return
		}

		grade := &Grade{}

		// Column 0: Exp - Period/Expression (e.g., "1(A)", "HR(A)")
		period := strings.TrimSpace(cells.Eq(0).Text())
		grade.Period = period

		// Columns 1-10: Attendance (skip for now)

		// Column 11: Course info (contains course name, teacher, email, room)
		courseCell := cells.Eq(11)

		// Parse course name (first text before <br>)
		courseHTML, _ := courseCell.Html()
		courseLines := strings.Split(courseHTML, "<br/>")
		if len(courseLines) > 0 {
			// Extract course name (remove nbsp and extra markup)
			courseName := strings.TrimSpace(stripHTMLTags(courseLines[0]))
			courseName = strings.ReplaceAll(courseName, "\u00a0", " ") // Replace &nbsp;
			courseName = strings.TrimSpace(courseName)
			grade.CourseName = courseName
		}

		// Parse teacher name and email
		courseCell.Find("a").Each(func(j int, link *goquery.Selection) {
			href, exists := link.Attr("href")
			if !exists {
				return
			}

			linkText := strings.TrimSpace(link.Text())

			// Teacher info link (teacherinfo.html)
			if strings.Contains(href, "teacherinfo.html") {
				// Link text is teacher name
				grade.Teacher = strings.TrimPrefix(linkText, "Email ")
			}

			// Email link
			if strings.HasPrefix(href, "mailto:") {
				email := strings.TrimPrefix(href, "mailto:")
				grade.TeacherEmail = email
				// If we haven't found teacher name yet, extract from email link text
				if grade.Teacher == "" {
					teacherName := strings.TrimPrefix(linkText, "Email ")
					grade.Teacher = teacherName
				}
			}

			// Course details link (scores.html). Capture both the frn (for
			// API calls) and the full href (needed to fetch the section
			// detail page, which requires begdate/enddate/fg/schoolid).
			if strings.Contains(href, "scores.html") {
				grade.CourseID = extractParameter(href, "frn")
				if grade.ScoresURL == "" {
					grade.ScoresURL = normalizeScoresURL(href)
				}
			}
		})

		// Parse room number from courseCell
		courseCell.Find("span.display-flex").Each(func(j int, span *goquery.Selection) {
			roomText := strings.TrimSpace(span.Text())
			// Format: "- Rm: 35"
			if strings.Contains(roomText, "Rm:") {
				parts := strings.Split(roomText, "Rm:")
				if len(parts) > 1 {
					grade.RoomNumber = strings.TrimSpace(parts[1])
				}
			}
		})

		// Columns 12-17: Q1, Q2, S1, Q3, Q4, S2 grade cells. Each contains a
		// scores.html link with the full param set we need for ScoresURL,
		// so we pass the grade through and let parseGradeCell backfill.
		parseGradeCell(cells.Eq(12), &grade.Q1Grade, grade)
		parseGradeCell(cells.Eq(13), &grade.Q2Grade, grade)
		parseGradeCell(cells.Eq(14), &grade.S1Grade, grade)
		parseGradeCell(cells.Eq(15), &grade.Q3Grade, grade)
		parseGradeCell(cells.Eq(16), &grade.Q4Grade, grade)
		parseGradeCell(cells.Eq(17), &grade.S2Grade, grade)

		// Set current grade to the most recent non-empty quarter/semester
		grade.CurrentGrade = getMostRecentGrade(grade)

		// Column 18: Absences
		absencesText := strings.TrimSpace(cells.Eq(18).Text())
		if absences, err := strconv.Atoi(absencesText); err == nil {
			grade.Absences = absences
		} else {
			// Might be a link with the count
			absencesLink := cells.Eq(18).Find("a")
			if absencesLink.Length() > 0 {
				absencesText = strings.TrimSpace(absencesLink.Text())
				if absences, err := strconv.Atoi(absencesText); err == nil {
					grade.Absences = absences
				}
			}
		}

		// Column 19: Tardies
		tardiesText := strings.TrimSpace(cells.Eq(19).Text())
		if tardies, err := strconv.Atoi(tardiesText); err == nil {
			grade.Tardies = tardies
		} else {
			// Might be a link with the count
			tardiesLink := cells.Eq(19).Find("a")
			if tardiesLink.Length() > 0 {
				tardiesText = strings.TrimSpace(tardiesLink.Text())
				if tardies, err := strconv.Atoi(tardiesText); err == nil {
					grade.Tardies = tardies
				}
			}
		}

		// Only add if we have a course name
		if grade.CourseName != "" {
			grades = append(grades, grade)
		}
	})

	return grades, nil
}

// parseGPA parses GPA information from HTML
func parseGPA(html string) (*GPA, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	gpa := &GPA{}

	// Look for GPA information
	// Common patterns: "GPA: 3.75", "Cumulative GPA: 3.80"
	doc.Find(".gpa, .GPA, [class*='gpa']").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		// Try to extract GPA value
		if strings.Contains(strings.ToLower(text), "cumulative") {
			gpa.Cumulative = extractGPAValue(text)
		} else if strings.Contains(strings.ToLower(text), "current") {
			gpa.Current = extractGPAValue(text)
		} else if gpa.Current == 0 {
			gpa.Current = extractGPAValue(text)
		}

		// Check for weighted GPA indicator
		if strings.Contains(strings.ToLower(text), "weighted") {
			gpa.Weighted = true
		}
	})

	// If no specific GPA elements found, search in text
	if gpa.Current == 0 && gpa.Cumulative == 0 {
		text := doc.Text()
		if strings.Contains(strings.ToLower(text), "gpa") {
			// Try to find GPA value near "GPA" text
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), "gpa") {
					value := extractGPAValue(line)
					if value > 0 {
						gpa.Current = value
						break
					}
				}
			}
		}
	}

	return gpa, nil
}

// Helper functions

// parseGradeCell parses a grade cell which may contain a link with grade info
// Format: <a href="scores.html?frn=...&begdate=...&enddate=...&fg=Q1&schoolid=...">A-<br>90</a>
// or "[ i ]" for no grade yet. If grade is non-nil, backfills CourseID and
// ScoresURL from the first cell whose href points at scores.html.
func parseGradeCell(cell *goquery.Selection, gradeStr *string, grade *Grade) {
	link := cell.Find("a")
	if link.Length() == 0 {
		text := strings.TrimSpace(cell.Text())
		if text != "" && !strings.Contains(text, "Not available") {
			*gradeStr = text
		}
		return
	}

	linkText := strings.TrimSpace(link.Text())
	// "[ i ]" means no grade posted yet for this term; the href is still a
	// valid scores URL so we fall through to capture it if needed.
	if !strings.Contains(linkText, "[ i ]") {
		if lines := strings.Split(linkText, "\n"); len(lines) > 0 {
			*gradeStr = strings.TrimSpace(lines[0])
		}
	}

	if grade == nil {
		return
	}
	href, exists := link.Attr("href")
	if !exists || !strings.Contains(href, "scores.html") {
		return
	}
	if grade.CourseID == "" {
		grade.CourseID = extractParameter(href, "frn")
	}
	if grade.ScoresURL == "" {
		grade.ScoresURL = normalizeScoresURL(href)
	}
}

// getMostRecentGrade returns the most recent non-empty grade
// Priority: Q2 > S1 > Q1 for first semester, Q4 > S2 > Q3 for second semester
func getMostRecentGrade(grade *Grade) string {
	// Check second semester first (most recent)
	if grade.Q4Grade != "" {
		return grade.Q4Grade
	}
	if grade.S2Grade != "" {
		return grade.S2Grade
	}
	if grade.Q3Grade != "" {
		return grade.Q3Grade
	}

	// Check first semester
	if grade.Q2Grade != "" {
		return grade.Q2Grade
	}
	if grade.S1Grade != "" {
		return grade.S1Grade
	}
	if grade.Q1Grade != "" {
		return grade.Q1Grade
	}

	return ""
}

// stripHTMLTags removes HTML tags from a string
func stripHTMLTags(s string) string {
	// Simple regex-free approach: remove everything between < and >
	var result strings.Builder
	inTag := false

	for _, char := range s {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// normalizeScoresURL converts a scores.html href from the grades table into a
// server-relative path suitable for the HTTP client. Hrefs in the table are
// bare ("scores.html?frn=...") — prefix /guardian/ if absent, leave absolute
// paths alone.
func normalizeScoresURL(href string) string {
	if strings.HasPrefix(href, "/") || strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	return "/guardian/" + href
}

// extractParameter extracts a URL parameter value
func extractParameter(href string, paramName string) string {
	parts := strings.Split(href, "?")
	if len(parts) < 2 {
		return ""
	}

	params := strings.Split(parts[1], "&")
	for _, param := range params {
		if strings.HasPrefix(param, paramName+"=") {
			return strings.TrimPrefix(param, paramName+"=")
		}
	}

	return ""
}

// extractCourseID extracts course ID from a URL (legacy function)
func extractCourseID(href string) string {
	// Try frn parameter first (the reference instance)
	if id := extractParameter(href, "frn"); id != "" {
		return id
	}
	// Try fg parameter
	if id := extractParameter(href, "fg"); id != "" {
		return id
	}
	// Try course parameter
	if id := extractParameter(href, "course"); id != "" {
		return id
	}
	return ""
}

// isLetterGrade checks if a string is a letter grade
func isLetterGrade(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 1 || len(s) > 2 {
		return false
	}

	// Common letter grades: A, A-, A+, B, B-, B+, etc.
	validGrades := []string{"A", "B", "C", "D", "F", "A+", "A-", "B+", "B-", "C+", "C-", "D+", "D-"}
	for _, grade := range validGrades {
		if s == grade {
			return true
		}
	}

	return false
}

// extractGPAValue extracts a numeric GPA value from text
func extractGPAValue(text string) float64 {
	// Look for patterns like "3.75", "GPA: 3.80", etc.
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "gpa:", "")
	text = strings.ReplaceAll(text, "gpa", "")
	text = strings.TrimSpace(text)

	// Try to find a number between 0 and 5 (typical GPA range)
	parts := strings.Fields(text)
	for _, part := range parts {
		if value, err := strconv.ParseFloat(part, 64); err == nil {
			if value >= 0 && value <= 5.0 {
				return value
			}
		}
	}

	return 0
}

// ScoresMetadata contains the identifiers scraped from a scores.html page
// that are needed to call the assignment-lookup API.
type ScoresMetadata struct {
	// SectionID is the DCID pulled from div[data-pss-student-assignment-scores]'s
	// data-sectionid attribute. Used as section_ids[] in the lookup POST.
	SectionID string
	// StudentAPIID is the numeric student identifier used by the XTE API.
	// Derived from the studentFRN embedded in the scores page's data-ng-init
	// expression (e.g. "001999999" → "999999"). This is NOT the same value
	// as the switchStudent() ID from the home-page nav.
	StudentAPIID string
}

// FetchScoresMetadata loads a scores.html page and extracts the identifiers
// needed for the assignment-lookup API. scoresURL must be the full path
// captured from Grade.ScoresURL — partial URLs redirect to the login page.
func (c *Client) FetchScoresMetadata(ctx context.Context, scoresURL string) (*ScoresMetadata, error) {
	if scoresURL == "" {
		return nil, fmt.Errorf("scores URL is required (did you populate Grade.ScoresURL?)")
	}

	c.logger.Debug("Fetching scores metadata from: %s", scoresURL)

	html, err := c.getPageHTMLWithWait(ctx, scoresURL, 3*time.Second)
	if err != nil {
		return nil, err
	}

	// If the server bounced us back to the login page, scoresURL was
	// malformed (typically missing one of the required query params).
	if strings.Contains(html, "Parent, Guardian, and Student Login") {
		return nil, fmt.Errorf("scores page redirected to login — scoresURL %q is missing required query params (need frn+begdate+enddate+fg+schoolid)", scoresURL)
	}

	return parseScoresMetadata(html)
}

// FetchSectionID is a thin wrapper over FetchScoresMetadata retained for
// call sites that only care about the section ID.
func (c *Client) FetchSectionID(ctx context.Context, scoresURL string) (string, error) {
	md, err := c.FetchScoresMetadata(ctx, scoresURL)
	if err != nil {
		return "", err
	}
	return md.SectionID, nil
}

// parseScoresMetadata pulls the section_id and the API-facing student_id out
// of a scores.html body. Both live in the rendered-from-server HTML, so this
// does not need a browser context.
func parseScoresMetadata(html string) (*ScoresMetadata, error) {
	md := &ScoresMetadata{}

	if m := sectionIDRE.FindStringSubmatch(html); len(m) > 1 {
		md.SectionID = m[1]
	} else {
		// Fall back to goquery in case the regex misses an unusual quoting.
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			return nil, fmt.Errorf("failed to parse HTML: %w", err)
		}
		doc.Find("div[data-pss-student-assignment-scores]").Each(func(i int, s *goquery.Selection) {
			if id, exists := s.Attr("data-sectionid"); exists && md.SectionID == "" {
				md.SectionID = id
			}
		})
	}

	if md.SectionID == "" {
		return nil, fmt.Errorf("section ID not found in scores page")
	}

	// studentFRN lives in the Angular data-ng-init expression on the
	// xteContentWrapper element, e.g. studentFRN = '001999999'. The reference district
	// prefixes the real DCID with "001"; strip that prefix (not just leading
	// zeros, since IDs that happen to start with 1 would lose a digit).
	if m := studentFRNRE.FindStringSubmatch(html); len(m) > 1 {
		md.StudentAPIID = strings.TrimPrefix(m[1], "001")
	}
	if md.StudentAPIID == "" {
		return nil, fmt.Errorf("studentFRN not found in scores page (looked for data-ng-init studentFRN='…')")
	}

	return md, nil
}

var (
	sectionIDRE  = regexp.MustCompile(`data-sectionid="(\d+)"`)
	studentFRNRE = regexp.MustCompile(`studentFRN\s*=\s*'(\d+)'`)
)

// Helper functions for min/max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
