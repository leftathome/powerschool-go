package powerschool

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// fakePortal is a stateful stand-in for the portal's home page and student
// switcher. Like the real one, "which student is selected" is state on the
// server side of the session, changed by a form post to home.html. All ids
// and names are synthetic.
type fakePortal struct {
	mu sync.Mutex

	selected     string // nav id currently selected
	noMarker     bool   // render the switcher with nobody marked
	ignoreSwitch bool   // accept the form post but change nothing
	postStatus   int    // status for the form post; 0 = 302 to home.html
	postLocation string // Location for a redirecting post; "" = home.html

	requests []string // "METHOD path" in order
	posts    []postedForm
}

type postedForm struct {
	contentType string
	body        string
	cookie      string
}

func (p *fakePortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, r.Method+" "+r.URL.Path)

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/guardian/home.html":
		if p.noMarker {
			_, _ = w.Write([]byte(homePageHTML()))
			return
		}
		_, _ = w.Write([]byte(homePageHTML(p.selected)))
	case r.Method == http.MethodPost && r.URL.Path == SwitchStudentPath:
		body, _ := io.ReadAll(r.Body)
		form := postedForm{contentType: r.Header.Get("Content-Type"), body: string(body)}
		if c, err := r.Cookie("JSESSIONID"); err == nil {
			form.cookie = c.Value
		}
		p.posts = append(p.posts, form)
		if !p.ignoreSwitch {
			p.selected = strings.TrimPrefix(string(body), SwitchStudentField+"=")
		}
		if p.postStatus != 0 && p.postStatus < 300 || p.postStatus >= 400 {
			w.WriteHeader(p.postStatus)
			_, _ = w.Write([]byte("<html><body>switch response for Zephyrine Quillfeather</body></html>"))
			return
		}
		loc := p.postLocation
		if loc == "" {
			loc = "/guardian/home.html"
		}
		http.Redirect(w, r, loc, http.StatusFound)
	case strings.HasPrefix(r.URL.Path, "/public/"):
		_, _ = w.Write([]byte(syntheticLoginPage))
	default:
		http.NotFound(w, r)
	}
}

func (p *fakePortal) snapshot() (requests []string, posts []postedForm, selected string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requests...), append([]postedForm(nil), p.posts...), p.selected
}

func TestSelectStudent_AlreadySelectedPostsNothing(t *testing.T) {
	portal := &fakePortal{selected: "111112"}
	client := newHTTPPageClient(t, portal)

	page, err := client.SelectStudent(context.Background(), "111112")
	if err != nil {
		t.Fatalf("SelectStudent: %v", err)
	}
	if sel := page.SelectedStudent(); sel == nil || sel.ID != "111112" {
		t.Errorf("selected = %+v, want 111112", sel)
	}
	requests, posts, _ := portal.snapshot()
	if len(posts) != 0 || len(requests) != 1 || requests[0] != "GET /guardian/home.html" {
		t.Errorf("requests = %v (%d posts), want one GET and no post", requests, len(posts))
	}
}

// TestSelectStudent_SwitchesAndConfirms: the form post carries exactly the
// one field the portal's own switchStudent() sends, with the session cookie
// attached explicitly, and the result is the page that confirmed it.
func TestSelectStudent_SwitchesAndConfirms(t *testing.T) {
	for _, tc := range []struct {
		name       string
		postStatus int
	}{
		{"post answered with a redirect to the home page", 0},
		{"post answered with a 200", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			portal := &fakePortal{selected: "111111", postStatus: tc.postStatus}
			client := newHTTPPageClient(t, portal)

			page, err := client.SelectStudent(context.Background(), "111112")
			if err != nil {
				t.Fatalf("SelectStudent: %v", err)
			}
			sel := page.SelectedStudent()
			if sel == nil || sel.ID != "111112" || sel.StudentNumber != "9000002" {
				t.Fatalf("selected = %+v, want 111112 enriched", sel)
			}
			if page.Students[0].Selected || page.Students[0].StudentNumber != "" {
				t.Errorf("previously selected student still carries the selection: %+v", page.Students[0])
			}
			if len(page.Grades) != 1 || page.Grades[0].StudentID != "111112" {
				t.Errorf("grades = %+v, want one row attributed to 111112", page.Grades)
			}

			requests, posts, selected := portal.snapshot()
			if selected != "111112" {
				t.Errorf("portal selection = %q, want 111112", selected)
			}
			if len(posts) != 1 {
				t.Fatalf("form posted %d times, want 1 (requests %v)", len(posts), requests)
			}
			if posts[0].body != "selected_student_id=111112" {
				t.Errorf("form body = %q, want exactly selected_student_id=111112", posts[0].body)
			}
			if posts[0].contentType != "application/x-www-form-urlencoded" {
				t.Errorf("Content-Type = %q", posts[0].contentType)
			}
			if posts[0].cookie != "test-session" {
				t.Errorf("session cookie on the post = %q, want it attached", posts[0].cookie)
			}
			for _, r := range requests {
				if r != "GET /guardian/home.html" && r != "POST "+SwitchStudentPath {
					t.Errorf("unexpected request %q", r)
				}
			}
			if last := requests[len(requests)-1]; last != "GET /guardian/home.html" {
				t.Errorf("last request = %q, want the confirming read", last)
			}
		})
	}
}

func TestSelectStudent_FailsClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("portal ignores the switch", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", ignoreSwitch: true}
		client := newHTTPPageClient(t, portal)
		page, err := client.SelectStudent(ctx, "111112")
		if !errors.Is(err, ErrSwitchNotConfirmed) || page != nil {
			t.Fatalf("SelectStudent = (%v, %v), want (nil, ErrSwitchNotConfirmed)", page, err)
		}
		if err := client.SwitchStudent(ctx, "111112"); !errors.Is(err, ErrSwitchNotConfirmed) {
			t.Errorf("SwitchStudent = %v, want ErrSwitchNotConfirmed", err)
		}
	})

	t.Run("no selection marker: nothing is posted", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", noMarker: true}
		client := newHTTPPageClient(t, portal)
		_, err := client.SelectStudent(ctx, "111112")
		if !errors.Is(err, ErrSelectionUnknown) {
			t.Fatalf("err = %v, want ErrSelectionUnknown", err)
		}
		if _, posts, _ := portal.snapshot(); len(posts) != 0 {
			t.Errorf("form posted %d times without a marker to confirm against", len(posts))
		}
	})

	t.Run("marker disappears after the switch", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			portal.ServeHTTP(w, r)
			if r.Method == http.MethodPost {
				portal.mu.Lock()
				portal.noMarker = true
				portal.mu.Unlock()
			}
		}))
		if _, err := client.SelectStudent(ctx, "111112"); !errors.Is(err, ErrSwitchNotConfirmed) {
			t.Fatalf("err = %v, want ErrSwitchNotConfirmed", err)
		}
	})

	t.Run("student not on the account: nothing is posted", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, portal)
		_, err := client.SelectStudent(ctx, "222222")
		if !IsNotFound(err) {
			t.Fatalf("err = %v, want not found", err)
		}
		if _, posts, _ := portal.snapshot(); len(posts) != 0 {
			t.Errorf("form posted for a student that is not in the switcher")
		}
	})

	t.Run("non-numeric id: no request at all", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, portal)
		for _, id := range []string{"", "11a", "111112&x=1", "111112 ", "-1"} {
			if _, err := client.SelectStudent(ctx, id); !IsNotFound(err) {
				t.Errorf("SelectStudent(%q) = %v, want not found", id, err)
			}
		}
		if requests, _, _ := portal.snapshot(); len(requests) != 0 {
			t.Errorf("requests made for a malformed id: %v", requests)
		}
	})

	t.Run("post answered with an error status", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", postStatus: http.StatusInternalServerError}
		client := newHTTPPageClient(t, portal)
		_, err := client.SelectStudent(ctx, "111112")
		var se *HTTPStatusError
		if !errors.As(err, &se) || se.StatusCode != http.StatusInternalServerError || se.Op != "POST "+SwitchStudentPath {
			t.Fatalf("err = %v, want an HTTPStatusError for the post", err)
		}
		if strings.Contains(err.Error(), "Quillfeather") || strings.Contains(err.Error(), "111112") {
			t.Errorf("error carries the response or the student id: %v", err)
		}
	})

	t.Run("post bounced to the login page", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", ignoreSwitch: true, postLocation: "/public/home.html"}
		client := newHTTPPageClient(t, portal)
		_, err := client.SelectStudent(ctx, "111112")
		if !IsSessionExpired(err) {
			t.Fatalf("err = %v, want session expired", err)
		}
	})
}

// TestGetGrades_SwitchesFirst: GetGrades for a student who is not selected
// switches to that student, and returns grades attributed -- by their own
// page -- to that student.
func TestGetGrades_SwitchesFirst(t *testing.T) {
	ctx := context.Background()

	t.Run("switches when another student is selected", func(t *testing.T) {
		portal := &fakePortal{selected: "111111"}
		client := newHTTPPageClient(t, portal)
		grades, err := client.GetGrades(ctx, "111112")
		if err != nil {
			t.Fatalf("GetGrades: %v", err)
		}
		if len(grades) != 1 || grades[0].StudentID != "111112" {
			t.Errorf("grades = %+v, want one row attributed to 111112", grades)
		}
		if _, posts, _ := portal.snapshot(); len(posts) != 1 {
			t.Errorf("form posted %d times, want 1", len(posts))
		}
	})

	t.Run("does not post when the student is already selected", func(t *testing.T) {
		portal := &fakePortal{selected: "111112"}
		client := newHTTPPageClient(t, portal)
		grades, err := client.GetGrades(ctx, "111112")
		if err != nil || len(grades) != 1 || grades[0].StudentID != "111112" {
			t.Fatalf("GetGrades = (%+v, %v)", grades, err)
		}
		if _, posts, _ := portal.snapshot(); len(posts) != 0 {
			t.Errorf("form posted %d times, want 0", len(posts))
		}
	})

	t.Run("the switch is ignored: no grades under the wrong id", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", ignoreSwitch: true}
		client := newHTTPPageClient(t, portal)
		grades, err := client.GetGrades(ctx, "111112")
		if !errors.Is(err, ErrSwitchNotConfirmed) || grades != nil {
			t.Fatalf("GetGrades = (%v, %v), want (nil, ErrSwitchNotConfirmed)", grades, err)
		}
	})

	t.Run("no marker", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", noMarker: true}
		client := newHTTPPageClient(t, portal)
		grades, err := client.GetGrades(ctx, "111111")
		if !errors.Is(err, ErrSelectionUnknown) || grades != nil {
			t.Fatalf("GetGrades = (%v, %v), want (nil, ErrSelectionUnknown)", grades, err)
		}
	})

	t.Run("empty id reads whoever is selected and never posts", func(t *testing.T) {
		portal := &fakePortal{selected: "111111", noMarker: true}
		client := newHTTPPageClient(t, portal)
		grades, err := client.GetGrades(ctx, "")
		if err != nil || len(grades) != 1 || grades[0].StudentID != "" {
			t.Fatalf("GetGrades = (%+v, %v), want one unattributed row", grades, err)
		}
		if _, posts, _ := portal.snapshot(); len(posts) != 0 {
			t.Errorf("form posted %d times, want 0", len(posts))
		}
	})
}

func TestSwitchConstantsMatchTheForm(t *testing.T) {
	if SwitchStudentPath != "/guardian/home.html" || SwitchStudentField != "selected_student_id" {
		t.Errorf("switch form = POST %s field %s", SwitchStudentPath, SwitchStudentField)
	}
	if !strings.Contains(homePageHTML("111111"), `name="`+SwitchStudentField+`"`) {
		t.Error("fixture home page does not carry the switcher form")
	}
}
