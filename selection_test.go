package powerschool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Everything in this file is synthetic: ids, names, numbers and markup were
// written by hand for the tests and describe no real student or school.

// homePageHTML builds a stand-in /guardian/home.html with two students in
// the switcher. selected lists the nav ids that carry class="selected";
// the student-demo table and the grades row belong to "whoever the session
// shows", exactly as on the portal.
func homePageHTML(selected ...string) string {
	isSel := func(id string) string {
		for _, s := range selected {
			if s == id {
				return ` class="selected"`
			}
		}
		return ""
	}
	return fmt.Sprintf(`<!doctype html><html><body>
<ul id="students-list">
  <li%s><a href="javascript:switchStudent(111111);">Student One</a></li>
  <li%s><a href="javascript:switchStudent(111112);">Student Two</a></li>
</ul>
<table class="student-demo">
  <tr><td class="lbl">Student ID #:</td><td>9000002</td></tr>
  <tr><td class="lbl">Grade Level:</td><td>7</td></tr>
</table>
<div id="print-school">Example District<br><span>Example Middle School</span></div>
<table id="tblgrades">
<tr class="center">
  <td>1(A)</td>
  <td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td>
  <td>Example Science 7&nbsp;<br/><a href="mailto:teacher@example.org">Email Teacher, Example</a></td>
  <td><a href="scores.html?frn=00111222333&begdate=09/03/2025&enddate=11/05/2025&fg=Q1&schoolid=1">B+<br/>88</a></td>
  <td></td><td></td><td></td><td></td><td></td>
  <td>2</td>
  <td>1</td>
</tr>
</table>
</body></html>`, isSel("111111"), isSel("111112"))
}

// syntheticLoginPage is the shape of the sign-in page: the form the browser
// login drives, and no student switcher.
const syntheticLoginPage = `<!doctype html><html><head><title>Sign In</title></head><body>
<form id="LoginForm" action="/guardian/home.html" method="post">
  <input type="text" id="fieldAccount" name="account">
  <input type="password" id="fieldPassword" name="pw">
  <button type="submit" id="btn-enter-sign-in">Sign In</button>
</form>
</body></html>`

func servePage(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

func TestParseHomePage_SelectionMarker(t *testing.T) {
	t.Run("marker on the first student", func(t *testing.T) {
		page, err := parseHomePage(homePageHTML("111111"))
		if err != nil {
			t.Fatalf("parseHomePage: %v", err)
		}
		sel := page.SelectedStudent()
		if sel == nil || sel.ID != "111111" {
			t.Fatalf("SelectedStudent = %+v, want 111111", sel)
		}
		if !page.Students[0].Selected || page.Students[1].Selected {
			t.Errorf("Selected flags = %v/%v, want true/false", page.Students[0].Selected, page.Students[1].Selected)
		}
		if sel.StudentNumber != "9000002" || sel.GradeLevel != 7 || sel.SchoolName != "Example Middle School" {
			t.Errorf("selected student not enriched: %+v", sel)
		}
		if page.Students[1].StudentNumber != "" || page.Students[1].SchoolName != "" {
			t.Errorf("unselected student was enriched: %+v", page.Students[1])
		}
		if len(page.Grades) != 1 || page.Grades[0].StudentID != "111111" {
			t.Errorf("grades = %+v, want one row attributed to 111111", page.Grades)
		}
	})

	// The defect this file exists for: the same page-level details, shown
	// while the SECOND student is selected, must land on the second student
	// and never on students[0].
	t.Run("marker on the second student", func(t *testing.T) {
		page, err := parseHomePage(homePageHTML("111112"))
		if err != nil {
			t.Fatalf("parseHomePage: %v", err)
		}
		sel := page.SelectedStudent()
		if sel == nil || sel.ID != "111112" || sel.StudentNumber != "9000002" {
			t.Fatalf("SelectedStudent = %+v, want 111112 enriched", sel)
		}
		if first := page.Students[0]; first.Selected || first.StudentNumber != "" || first.GradeLevel != 0 || first.SchoolName != "" {
			t.Errorf("first student carries the selected student's details: %+v", first)
		}
		if page.Grades[0].StudentID != "111112" {
			t.Errorf("grade attributed to %q, want 111112", page.Grades[0].StudentID)
		}
	})

	t.Run("no marker: nobody is selected and nobody is enriched", func(t *testing.T) {
		page, err := parseHomePage(homePageHTML())
		if err != nil {
			t.Fatalf("parseHomePage: %v", err)
		}
		if sel := page.SelectedStudent(); sel != nil {
			t.Fatalf("SelectedStudent = %+v, want nil", sel)
		}
		if len(page.Students) != 2 {
			t.Fatalf("students = %d, want 2", len(page.Students))
		}
		for _, s := range page.Students {
			if s.Selected || s.StudentNumber != "" || s.GradeLevel != 0 || s.SchoolName != "" {
				t.Errorf("student enriched without a selection marker: %+v", s)
			}
		}
		if len(page.Grades) != 1 || page.Grades[0].StudentID != "" {
			t.Errorf("grades = %+v, want one unattributed row", page.Grades)
		}
	})

	t.Run("two markers are ambiguous", func(t *testing.T) {
		page, err := parseHomePage(homePageHTML("111111", "111112"))
		if err != nil {
			t.Fatalf("parseHomePage: %v", err)
		}
		if sel := page.SelectedStudent(); sel != nil {
			t.Errorf("SelectedStudent = %+v, want nil", sel)
		}
		for _, s := range page.Students {
			if s.StudentNumber != "" {
				t.Errorf("student enriched under an ambiguous marker: %+v", s)
			}
		}
	})

	t.Run("a switcher rendered twice lists each student once", func(t *testing.T) {
		html := strings.Replace(homePageHTML("111112"), "<table class=\"student-demo\">",
			`<div class="mobile-nav"><a href="javascript:switchStudent(111111);">Student One</a>
<a href="javascript:switchStudent(111112);">Student Two</a></div><table class="student-demo">`, 1)
		page, err := parseHomePage(html)
		if err != nil {
			t.Fatalf("parseHomePage: %v", err)
		}
		if len(page.Students) != 2 {
			t.Fatalf("students = %d, want 2", len(page.Students))
		}
		if sel := page.SelectedStudent(); sel == nil || sel.ID != "111112" {
			t.Errorf("SelectedStudent = %+v, want 111112", sel)
		}
	})

	t.Run("no switcher at all is a parse error", func(t *testing.T) {
		_, err := parseHomePage("<html><body><table id=\"tblgrades\"></table></body></html>")
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeParseError {
			t.Fatalf("err = %v, want a parse error", err)
		}
	})
}

func TestGetGrades_ChecksTheSelectedStudent(t *testing.T) {
	ctx := context.Background()

	t.Run("asked-for student is selected", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage(homePageHTML("111112")))
		grades, err := client.GetGrades(ctx, "111112")
		if err != nil {
			t.Fatalf("GetGrades: %v", err)
		}
		if len(grades) != 1 || grades[0].StudentID != "111112" {
			t.Errorf("grades = %+v", grades)
		}
	})

	t.Run("another student is selected", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage(homePageHTML("111112")))
		grades, err := client.GetGrades(ctx, "111111")
		if !errors.Is(err, ErrStudentNotSelected) || grades != nil {
			t.Fatalf("GetGrades = (%v, %v), want (nil, ErrStudentNotSelected)", grades, err)
		}
	})

	t.Run("no marker", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage(homePageHTML()))
		grades, err := client.GetGrades(ctx, "111111")
		if !errors.Is(err, ErrSelectionUnknown) || grades != nil {
			t.Fatalf("GetGrades = (%v, %v), want (nil, ErrSelectionUnknown)", grades, err)
		}
	})

	t.Run("empty id opts out of the check", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage(homePageHTML()))
		grades, err := client.GetGrades(ctx, "")
		if err != nil || len(grades) != 1 || grades[0].StudentID != "" {
			t.Fatalf("GetGrades = (%+v, %v), want one unattributed row", grades, err)
		}
	})
}

// TestGetHomePage_OneFetch: students and grades come from one response.
func TestGetHomePage_OneFetch(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/guardian/home.html" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(homePageHTML("111111")))
	}))

	page, err := client.GetHomePage(context.Background())
	if err != nil {
		t.Fatalf("GetHomePage: %v", err)
	}
	if len(page.Students) != 2 || len(page.Grades) != 1 || page.SelectedStudent() == nil {
		t.Errorf("page = %+v", page)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("home page fetched %d times, want 1", hits)
	}
}

// TestLoginPageWith200IsSessionExpired: a dead session that is answered
// with the sign-in form and a 200 -- no redirect -- must be an auth error on
// every page read, not an empty or unparseable page.
func TestLoginPageWith200IsSessionExpired(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *Client) error{
		"GetStudents": func(c *Client) error { _, err := c.GetStudents(ctx); return err },
		"GetHomePage": func(c *Client) error { _, err := c.GetHomePage(ctx); return err },
		"GetGrades":   func(c *Client) error { _, err := c.GetGrades(ctx, "111111"); return err },
		"FetchScoresMetadata": func(c *Client) error {
			_, err := c.FetchScoresMetadata(ctx, "/guardian/scores.html?frn=00111222333&fg=Q1")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			client := newHTTPPageClient(t, servePage(syntheticLoginPage))
			err := call(client)
			if !IsSessionExpired(err) || !errors.Is(err, ErrSessionExpired) {
				t.Fatalf("err = %v, want a session-expired error", err)
			}
			if strings.Contains(err.Error(), "frn=") {
				t.Errorf("error carries the query string: %v", err)
			}
			if client.IsAuthenticated() {
				t.Error("session still marked valid after the login page came back")
			}
		})
	}
}

func TestIsLoginPage(t *testing.T) {
	cases := []struct {
		name string
		html string
		want bool
	}{
		{"sign-in form", syntheticLoginPage, true},
		{"sign-in heading only", "<html><body><h1>" + loginPageTitle + "</h1></body></html>", true},
		{"password input by name", `<html><body><input type="password" name="pw"></body></html>`, true},
		{"signed-in home page", homePageHTML("111111"), false},
		{"signed-in page that mentions a password", homePageHTML("111111") + `<a href="/guardian/account.html">Change password</a>`, false},
		{"signed-in page with a password field keeps its switcher", homePageHTML("111111") + `<input type="password" name="pw">`, false},
		{"unrelated page", "<html><body>Class Score Detail</body></html>", false},
		{"word password without the form", "<html><body>Forgot your password?</body></html>", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLoginPage(tc.html); got != tc.want {
				t.Errorf("isLoginPage = %v, want %v", got, tc.want)
			}
		})
	}
}

// fakeChild is a made-up name the error-hygiene tests plant in responses.
const fakeChild = "Zephyrine Quillfeather"

// TestErrorsCarryNoResponseBodyOrQuery: an unexpected status is reported by
// code and size only.
func TestErrorsCarryNoResponseBodyOrQuery(t *testing.T) {
	ctx := context.Background()
	errorPage := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html><body>Error loading record for " + fakeChild + " frn=00111222333</body></html>"))
	})

	check := func(t *testing.T, err error, wantOp string) {
		t.Helper()
		if err == nil {
			t.Fatal("no error")
		}
		var se *HTTPStatusError
		if !errors.As(err, &se) {
			t.Fatalf("err = %v, want an *HTTPStatusError in the chain", err)
		}
		if se.StatusCode != http.StatusInternalServerError || se.BodyLen == 0 || se.Op != wantOp {
			t.Errorf("status error = %+v, want op %q status 500 and a body length", se, wantOp)
		}
		for _, leak := range []string{fakeChild, "Quillfeather", "frn=", "schoolid", "?"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("error text contains %q: %v", leak, err)
			}
		}
	}

	t.Run("page read", func(t *testing.T) {
		client := newHTTPPageClient(t, errorPage)
		_, err := client.FetchScoresMetadata(ctx, "/guardian/scores.html?frn=00111222333&schoolid=1")
		check(t, err, "GET /guardian/scores.html")
	})

	t.Run("assignment lookup", func(t *testing.T) {
		client := newHTTPPageClient(t, errorPage)
		grade := &Grade{
			CourseID:     "00111222333",
			ScoresURL:    "/guardian/scores.html?frn=00111222333&schoolid=1",
			SectionID:    "654321",
			StudentAPIID: "999999",
		}
		_, err := client.GetAssignments(ctx, "111111", grade)
		check(t, err, "POST "+AssignmentLookupPath)
	})

	t.Run("lookup that is not JSON", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage(`{"student":"`+fakeChild+`"`))
		grade := &Grade{ScoresURL: "/guardian/scores.html?frn=1", SectionID: "654321", StudentAPIID: "999999"}
		_, err := client.GetAssignments(ctx, "111111", grade)
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeParseError {
			t.Fatalf("err = %v, want a parse error", err)
		}
		if strings.Contains(err.Error(), "Quillfeather") {
			t.Errorf("error text contains the response: %v", err)
		}
	})

	t.Run("non-numeric ids are not echoed", func(t *testing.T) {
		client := newHTTPPageClient(t, servePage("[]"))
		grade := &Grade{ScoresURL: "/guardian/scores.html?frn=1", SectionID: "654321", StudentAPIID: "x9000002x"}
		_, err := client.GetAssignments(ctx, "111111", grade)
		if err == nil || strings.Contains(err.Error(), "9000002") {
			t.Errorf("err = %v, want an error that does not echo the id", err)
		}
	})
}

// failingTransport fails every request the way a dial error does.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("synthetic dial failure")
}

// TestTransportErrorsCarryNoQuery: http.Client reports the full URL in its
// *url.Error; the library strips the query before wrapping it.
func TestTransportErrorsCarryNoQuery(t *testing.T) {
	client := newHTTPPageClient(t, servePage(""))
	client.httpClient = &http.Client{Transport: failingTransport{}}

	_, err := client.FetchScoresMetadata(context.Background(), "/guardian/scores.html?frn=00111222333&schoolid=1")
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), "frn=") || strings.Contains(err.Error(), "schoolid") {
		t.Errorf("error carries the query string: %v", err)
	}
	if !strings.Contains(err.Error(), "/guardian/scores.html") || !strings.Contains(err.Error(), "synthetic dial failure") {
		t.Errorf("error lost the path or the cause: %v", err)
	}
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != CodeNetworkError {
		t.Errorf("err = %v, want a network error", err)
	}
}

func TestPathOnly(t *testing.T) {
	for in, want := range map[string]string{
		"/guardian/scores.html?frn=1&schoolid=2": "/guardian/scores.html",
		"https://ps.example.org/a/b?x=1#frag":    "https://ps.example.org/a/b",
		"/guardian/home.html":                    "/guardian/home.html",
		"/guardian/home.html#top":                "/guardian/home.html",
		"":                                       "",
	} {
		if got := pathOnly(in); got != want {
			t.Errorf("pathOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSwitchStudent_IsExplicitlyUnsupported: it must not report success, in
// either page mode, and must not touch the portal.
func TestSwitchStudent_IsExplicitlyUnsupported(t *testing.T) {
	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	for _, httpPages := range []bool{true, false} {
		client.SetHTTPPageFetch(httpPages)
		if err := client.SwitchStudent(context.Background(), "111112"); !errors.Is(err, ErrSwitchStudentUnsupported) {
			t.Errorf("http pages %v: SwitchStudent = %v, want ErrSwitchStudentUnsupported", httpPages, err)
		}
	}
}
