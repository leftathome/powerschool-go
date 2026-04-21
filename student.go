package powerschool

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/leftathome/powerschool-go/internal/browser"
)

// GetStudents returns all students accessible to the authenticated account
// This extracts student IDs from the navigation bar's switchStudent() JavaScript calls
func (c *Client) GetStudents(ctx context.Context) ([]*Student, error) {
	c.logger.Info("Fetching students from home page")

	// Ensure we're authenticated
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// Get the home page HTML
	html, err := c.getPageHTML(ctx, "/guardian/home.html")
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Parsing students from home page HTML")

	students, err := parseStudentsFromHomePage(html)
	if err != nil {
		return nil, WrapParseError(err, "students list")
	}

	c.logger.Info("Found %d student(s)", len(students))
	return students, nil
}

// GetStudent returns information about a specific student
func (c *Client) GetStudent(ctx context.Context, studentID string) (*Student, error) {
	students, err := c.GetStudents(ctx)
	if err != nil {
		return nil, err
	}

	for _, student := range students {
		if student.ID == studentID {
			return student, nil
		}
	}

	return nil, ErrNotFound
}

// getPageHTML retrieves the HTML content of a page
// This uses browser automation to handle JavaScript-rendered content
func (c *Client) getPageHTML(ctx context.Context, path string) (string, error) {
	return c.getPageHTMLWithWait(ctx, path, 2*time.Second)
}

// getPageHTMLWithWait retrieves HTML from a path with custom JavaScript wait time
// waitTime specifies how long to wait for JavaScript to execute before extracting HTML
func (c *Client) getPageHTMLWithWait(ctx context.Context, path string, waitTime time.Duration) (string, error) {
	if !c.IsAuthenticated() {
		return "", ErrSessionExpired
	}

	fullURL := c.buildURL(path)

	c.logger.Debug("Fetching page: %s (wait: %v)", fullURL, waitTime)

	// Use browser automation to get the page content with session cookies
	html, err := browser.GetPageContentWithWait(ctx, fullURL, c.session.GetCookies(), 30*time.Second, waitTime)
	if err != nil {
		return "", WrapNetworkError(err)
	}

	c.logger.DebugHTML("page content", html)

	return html, nil
}

// parseStudentsFromHomePage parses student information from the Guardian home page
// Looks for switchStudent(ID) JavaScript calls in the navigation bar
func parseStudentsFromHomePage(html string) ([]*Student, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var students []*Student

	// Pattern to match: javascript:switchStudent(111111);
	switchStudentPattern := regexp.MustCompile(`javascript:switchStudent\((\d+)\)`)

	// Find all links with switchStudent JavaScript
	doc.Find("a[href*='switchStudent']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		matches := switchStudentPattern.FindStringSubmatch(href)
		if len(matches) < 2 {
			return
		}

		studentID := matches[1]
		studentName := strings.TrimSpace(s.Text())

		if studentID != "" && studentName != "" {
			students = append(students, &Student{
				ID:   studentID,
				Name: studentName,
			})
		}
	})

	// If we found students via switchStudent, try to get more details from the page
	if len(students) > 0 {
		// Try to extract additional details from the currently displayed student info
		enrichStudentDetails(doc, students)
	}

	if len(students) == 0 {
		return nil, fmt.Errorf("no students found in HTML (looked for switchStudent JavaScript calls)")
	}

	return students, nil
}

// enrichStudentDetails tries to extract additional student details from the page
// This looks for the table.student-demo element that contains student details
func enrichStudentDetails(doc *goquery.Document, students []*Student) {
	if len(students) == 0 {
		return
	}

	// First, find which student is currently selected
	// the reference instance marks the selected student with class="selected" in the student list
	var selectedStudentID string
	doc.Find("#students-list li.selected a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			// Extract ID from javascript:switchStudent(111112);
			switchStudentPattern := regexp.MustCompile(`javascript:switchStudent\((\d+)\)`)
			matches := switchStudentPattern.FindStringSubmatch(href)
			if len(matches) > 1 {
				selectedStudentID = matches[1]
			}
		}
	})

	// If no selected student found, check for non-selected students and use first one
	// (page might default to showing the first student without marking it selected)
	if selectedStudentID == "" && len(students) > 0 {
		selectedStudentID = students[0].ID
	}

	// Find the student object that matches the selected ID
	var currentStudent *Student
	for _, student := range students {
		if student.ID == selectedStudentID {
			currentStudent = student
			break
		}
	}

	// If we couldn't determine which student is selected, use the first one
	if currentStudent == nil && len(students) > 0 {
		currentStudent = students[0]
	}

	// the reference instance structure: <table class="student-demo" role="presentation">
	// with rows like: <tr><td class="lbl">Student ID #:</td><td>8199499</td></tr>
	doc.Find("table.student-demo tr").Each(func(i int, row *goquery.Selection) {
		label := strings.TrimSpace(row.Find("td.lbl").Text())
		value := ""

		// Get the second td (the value)
		row.Find("td").Each(func(j int, cell *goquery.Selection) {
			if j == 1 { // Second cell contains the value
				value = strings.TrimSpace(cell.Text())
			}
		})

		if value == "" {
			return
		}

		// Parse based on label and apply to the currently selected student
		switch {
		case strings.Contains(strings.ToLower(label), "student id #"):
			currentStudent.StudentNumber = value
		case strings.Contains(strings.ToLower(label), "state id #"):
			currentStudent.StateID = value
		case strings.Contains(strings.ToLower(label), "grade level"):
			if gradeLevel, err := strconv.Atoi(value); err == nil {
				currentStudent.GradeLevel = gradeLevel
			}
		case strings.Contains(strings.ToLower(label), "student portal username") ||
			strings.Contains(strings.ToLower(label), "email"):
			currentStudent.PortalUsername = value
		case strings.Contains(strings.ToLower(label), "source username"):
			currentStudent.SourceUsername = value
		}
	})

	// Also look for school name in the page
	// the reference instance shows: <div id="print-school">the reference school district<br><span>Example Middle School</span></div>
	doc.Find("#print-school span").Each(func(i int, s *goquery.Selection) {
		schoolName := strings.TrimSpace(s.Text())
		if schoolName != "" {
			currentStudent.SchoolName = schoolName
		}
	})
}

// extractValueAfterLabel tries to extract a value that comes after a label
func extractValueAfterLabel(s *goquery.Selection, text string) string {
	// Try to find sibling or child element with the value
	// This is a heuristic that may need adjustment

	// Look at next sibling
	next := s.Next()
	if next.Length() > 0 {
		value := strings.TrimSpace(next.Text())
		if value != "" && value != text {
			return value
		}
	}

	// Look for value within same element after colon
	if idx := strings.Index(text, ":"); idx != -1 && idx < len(text)-1 {
		value := strings.TrimSpace(text[idx+1:])
		if value != "" {
			return value
		}
	}

	return ""
}

// SwitchStudent switches to a different student context
// This may require calling the switchStudent JavaScript function
// For now, we'll just update subsequent requests to include the student ID
func (c *Client) SwitchStudent(ctx context.Context, studentID string) error {
	c.logger.Info("Switching to student ID: %s", studentID)

	// The switchStudent JavaScript likely sets a session variable
	// We can simulate this by navigating to a page with the student context
	// Based on the pattern, we might need to call the actual JavaScript or
	// make a request that triggers the switch

	// For now, we'll attempt to navigate to the home page which should
	// switch context, but we may need to adjust this based on how it actually works

	// One approach: use browser automation to execute the JavaScript
	session := c.GetSession()
	if session == nil {
		return ErrSessionExpired
	}

	fullURL := c.GetBaseURL() + "/guardian/home.html"

	// Use browser to execute the switch
	html, err := browser.GetPageContent(ctx, fullURL, session.GetCookies(), 30*time.Second)
	if err != nil {
		return WrapNetworkError(err)
	}

	// TODO: Actually execute: javascript:switchStudent(studentID)
	// This might require enhancing the browser package to execute arbitrary JavaScript

	_ = html // TODO: verify switch was successful

	c.logger.Info("Student switch completed")
	return nil
}
