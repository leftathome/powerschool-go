package powerschool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// syntheticHomePage is a hand-written stand-in for /guardian/home.html. It
// carries the two structures the parsers read: the switchStudent() nav and
// one table#tblgrades row. Nothing in it comes from a real instance.
const syntheticHomePage = `<!doctype html><html><body>
<ul id="students-list">
  <li class="selected"><a href="javascript:switchStudent(111111);">Student One</a></li>
  <li><a href="javascript:switchStudent(111112);">Student Two</a></li>
</ul>
<table class="student-demo"><tr><td class="lbl">Student ID #:</td><td>9000001</td></tr></table>
<table id="tblgrades">
<tr class="center">
  <td>1(A)</td>
  <td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td>
  <td>Example Science 7&nbsp;<br/><a href="mailto:teacher@example.org">Email Teacher, Example</a><span class="display-flex">- Rm: 12</span></td>
  <td><a href="scores.html?frn=00111222333&begdate=09/03/2025&enddate=11/05/2025&fg=Q1&schoolid=1">B+<br/>88</a></td>
  <td></td><td></td><td></td><td></td><td></td>
  <td><a href="#">2</a></td>
  <td>1</td>
</tr>
</table>
</body></html>`

func newHTTPPageClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	client, err := NewClient(
		srv.URL,
		WithHTTPClient(srv.Client()),
		WithHTTPPageFetch(),
		WithSession([]*http.Cookie{{Name: "JSESSIONID", Value: "test-session"}}, time.Now().Add(time.Hour)),
		WithLogLevel(LogLevelNone),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestHTTPPageFetch_ReadsPagesWithGETOnly drives GetStudents and GetGrades
// in HTTP page mode and asserts that every request is a GET carrying the
// session cookie -- no browser, no other method.
func TestHTTPPageFetch_ReadsPagesWithGETOnly(t *testing.T) {
	var mu sync.Mutex
	var methods []string

	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "test-session" {
			http.Error(w, "no session cookie", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/guardian/home.html" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(syntheticHomePage))
	}))

	if !client.UsesHTTPPageFetch() {
		t.Fatal("UsesHTTPPageFetch() = false after WithHTTPPageFetch")
	}

	ctx := context.Background()
	students, err := client.GetStudents(ctx)
	if err != nil {
		t.Fatalf("GetStudents: %v", err)
	}
	if len(students) != 2 || students[0].ID != "111111" || students[0].StudentNumber != "9000001" {
		t.Errorf("students = %+v, want two with the selected one enriched", students)
	}
	if students[1].StudentNumber != "" {
		t.Errorf("unselected student was enriched: %+v", students[1])
	}

	grades, err := client.GetGrades(ctx, "111111")
	if err != nil {
		t.Fatalf("GetGrades: %v", err)
	}
	if len(grades) != 1 {
		t.Fatalf("len(grades) = %d, want 1", len(grades))
	}
	g := grades[0]
	if g.CourseName != "Example Science 7" || g.CourseID != "00111222333" || g.Absences != 2 || g.Tardies != 1 {
		t.Errorf("grade = %+v", g)
	}
	if !strings.HasPrefix(g.ScoresURL, "/guardian/scores.html?frn=00111222333") {
		t.Errorf("ScoresURL = %q", g.ScoresURL)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(methods) != 2 {
		t.Fatalf("requests = %v, want 2", methods)
	}
	for _, m := range methods {
		if m != "GET /guardian/home.html" {
			t.Errorf("unexpected request %q", m)
		}
	}
}

// TestHTTPPageFetch_LoginRedirectIsSessionExpired checks that a bounce to
// the public login page surfaces as a session-expired error rather than an
// empty, successful parse.
func TestHTTPPageFetch_LoginRedirectIsSessionExpired(t *testing.T) {
	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public/") {
			_, _ = w.Write([]byte("<html><body>Parent, Guardian, and Student Login</body></html>"))
			return
		}
		http.Redirect(w, r, "/public/home.html", http.StatusFound)
	}))

	_, err := client.GetGrades(context.Background(), "111111")
	if !IsSessionExpired(err) {
		t.Fatalf("GetGrades error = %v, want session expired", err)
	}
	if client.IsAuthenticated() {
		t.Error("session still marked valid after login redirect")
	}
}

// TestHTTPPageFetch_BrowserOnlyCallsRefuse checks that the calls which can
// only work through a browser fail instead of starting one.
func TestHTTPPageFetch_BrowserOnlyCallsRefuse(t *testing.T) {
	client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	ctx := context.Background()

	if err := client.SwitchStudent(ctx, "111112"); err == nil {
		t.Error("SwitchStudent succeeded in HTTP page mode")
	}
	if _, err := client.GetProgressReports(ctx, "111111"); err == nil {
		t.Error("GetProgressReports succeeded in HTTP page mode")
	}
}
