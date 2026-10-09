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

// HomePage is everything the library reads from one fetch of
// /guardian/home.html: the account's students and the grades table of the
// student the session has selected.
type HomePage struct {
	// Students lists every student in the switcher, in page order. At most
	// one has Selected set.
	Students []*Student
	// Grades is the grades table. It belongs to SelectedStudent(); when
	// that is nil the page did not say whose grades these are.
	Grades []*Grade
	// Student is the same evidence in the form the other guardian pages
	// report it (PageStudent): the selected switcher id and the name the
	// page heading ends with. A caller can compare HeadingName with the
	// HeadingName of an attendance or teacher-comments page read in the
	// same pass.
	Student PageStudent
}

// SelectedStudent returns the student the page explicitly marked as
// selected, or nil when it marked none.
func (h *HomePage) SelectedStudent() *Student {
	if h == nil {
		return nil
	}
	for _, s := range h.Students {
		if s != nil && s.Selected {
			return s
		}
	}
	return nil
}

// GetHomePage fetches /guardian/home.html once and parses both the student
// switcher and the grades table from that single response, so the grades
// and the "which student is selected" answer cannot come from two different
// states of the session.
//
// Every returned Grade carries the selected student's id in Grade.StudentID
// ("" when the page has no explicit marker).
func (c *Client) GetHomePage(ctx context.Context) (*HomePage, error) {
	c.logger.Info("Fetching home page")

	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	html, err := c.getPageHTML(ctx, "/guardian/home.html")
	if err != nil {
		return nil, err
	}
	return parseHomePage(html)
}

// parseHomePage parses the students and the grades out of one home page.
func parseHomePage(html string) (*HomePage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, WrapParseError(err, "home page")
	}
	students, err := parseStudentsFromDoc(doc)
	if err != nil {
		return nil, WrapParseError(err, "students list")
	}
	page := &HomePage{Students: students, Grades: parseGradesFromDoc(doc), Student: pageStudent(doc)}
	if sel := page.SelectedStudent(); sel != nil {
		for _, g := range page.Grades {
			g.StudentID = sel.ID
		}
	}
	return page, nil
}

// GetStudents returns all students accessible to the authenticated account.
// This extracts student IDs from the navigation bar's switchStudent()
// JavaScript calls. The student the page explicitly marks as selected has
// Selected set and carries the page-level details (StudentNumber, ...); if
// the page marks nobody, no student is enriched.
func (c *Client) GetStudents(ctx context.Context) ([]*Student, error) {
	page, err := c.GetHomePage(ctx)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Found %d student(s)", len(page.Students))
	return page.Students, nil
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

	if c.httpPages {
		return c.getPageHTMLOverHTTP(ctx, path)
	}

	fullURL := c.buildURL(path)

	c.logger.Debug("Fetching page: %s (wait: %v)", pathOnly(path), waitTime)

	// Use browser automation to get the page content with session cookies
	html, err := browser.GetPageContentWithWait(ctx, fullURL, c.session.GetCookies(), 30*time.Second, waitTime)
	if err != nil {
		return "", WrapNetworkError(err)
	}

	c.logger.DebugHTML("page content", html)

	return html, nil
}

// switchStudentRE matches the student switcher's href, e.g.
// javascript:switchStudent(111111);
var switchStudentRE = regexp.MustCompile(`javascript:switchStudent\((\d+)\)`)

// parseStudentsFromHomePage parses student information from the Guardian home page
// Looks for switchStudent(ID) JavaScript calls in the navigation bar
func parseStudentsFromHomePage(html string) ([]*Student, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	return parseStudentsFromDoc(doc)
}

// parseStudentsFromDoc is parseStudentsFromHomePage on a parsed document.
//
// Each student appears once, in page order, even when the switcher is
// rendered more than once (desktop and mobile navigation).
func parseStudentsFromDoc(doc *goquery.Document) ([]*Student, error) {
	var students []*Student
	seen := make(map[string]bool)

	doc.Find("a[href*='switchStudent']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		matches := switchStudentRE.FindStringSubmatch(href)
		if len(matches) < 2 {
			return
		}
		studentID := matches[1]
		studentName := strings.TrimSpace(s.Text())
		if studentID == "" || studentName == "" || seen[studentID] {
			return
		}
		seen[studentID] = true
		students = append(students, &Student{ID: studentID, Name: studentName})
	})

	if len(students) == 0 {
		return nil, fmt.Errorf("no students found in HTML (looked for switchStudent JavaScript calls)")
	}

	enrichStudentDetails(doc, students)
	return students, nil
}

// selectedStudentID returns the id of the student the page explicitly marks
// as selected (#students-list li.selected), or "" when the page carries no
// such marker or marks more than one student.
//
// There is deliberately no fallback. The page-level details (the
// student-demo table, the grades table) describe whichever student the
// session has selected; guessing "the first one" when the marker is absent
// attaches one child's records to another child's id.
func selectedStudentID(doc *goquery.Document) string {
	ids := make(map[string]bool)
	doc.Find("#students-list li.selected a[href*='switchStudent']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if m := switchStudentRE.FindStringSubmatch(href); len(m) > 1 {
			ids[m[1]] = true
		}
	})
	if len(ids) != 1 {
		return ""
	}
	for id := range ids {
		return id
	}
	return ""
}

// enrichStudentDetails marks the explicitly selected student (Selected) and
// copies the page-level details -- the table.student-demo rows and the
// school name -- onto that student. When the page does not say which
// student is selected, nobody is marked and nobody is enriched.
func enrichStudentDetails(doc *goquery.Document, students []*Student) {
	selectedID := selectedStudentID(doc)
	if selectedID == "" {
		return
	}

	var currentStudent *Student
	for _, student := range students {
		if student.ID == selectedID {
			currentStudent = student
			break
		}
	}
	if currentStudent == nil {
		return
	}
	currentStudent.Selected = true

	// Reference structure: <table class="student-demo" role="presentation">
	// with rows like: <tr><td class="lbl">Student ID #:</td><td>9000001</td></tr>
	doc.Find("table.student-demo tr").Each(func(i int, row *goquery.Selection) {
		label := strings.ToLower(strings.TrimSpace(row.Find("td.lbl").Text()))
		value := strings.TrimSpace(row.Find("td").Eq(1).Text())
		if value == "" {
			return
		}

		switch {
		case strings.Contains(label, "student id #"):
			currentStudent.StudentNumber = value
		case strings.Contains(label, "state id #"):
			currentStudent.StateID = value
		case strings.Contains(label, "grade level"):
			if gradeLevel, err := strconv.Atoi(value); err == nil {
				currentStudent.GradeLevel = gradeLevel
			}
		case strings.Contains(label, "student portal username") ||
			strings.Contains(label, "email"):
			currentStudent.PortalUsername = value
		case strings.Contains(label, "source username"):
			currentStudent.SourceUsername = value
		}
	})

	// Reference structure: <div id="print-school">District<br><span>Example Middle School</span></div>
	doc.Find("#print-school span").Each(func(i int, s *goquery.Selection) {
		if schoolName := strings.TrimSpace(s.Text()); schoolName != "" {
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
