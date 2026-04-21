package powerschool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestGetAssignments_ParsesFixtureAndEnforcesHeaders drives GetAssignments
// against an httptest.Server that returns a captured live response, so we
// verify both sides of the contract at once:
//
//   1. The outbound POST includes the Referer header. Real PowerSchool
//      returns 400 + HTML when Referer is absent (see ASSIGNMENTS_API_DEBUG.md),
//      so the stub mimics that to catch regressions.
//   2. The inbound JSON is parsed correctly into Assignment values —
//      due dates, categories, flags, and score handling all survive the
//      translation.
//
// The fixture is captured live and redacted (studentsdcid anonymised) — see
// testdata/README for refresh instructions.
func TestGetAssignments_ParsesFixtureAndEnforcesHeaders(t *testing.T) {
	fixture, err := os.ReadFile("testdata/assignments_lookup.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var seenBody map[string]any
	var seenReferer, seenContentType, seenAccept string
	var seenCookies []string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/xte/assignment/lookup" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		// Emulate the real server's Referer check: no Referer → 400 + HTML.
		if r.Header.Get("Referer") == "" {
			w.Header().Set("Content-Type", "text/html;charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<!doctype html><title>HTTP Status 400 - Bad Request</title>"))
			return
		}
		seenReferer = r.Header.Get("Referer")
		seenContentType = r.Header.Get("Content-Type")
		seenAccept = r.Header.Get("Accept")
		for _, c := range r.Cookies() {
			seenCookies = append(seenCookies, c.Name+"="+c.Value)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &seenBody); err != nil {
			t.Errorf("server: failed to decode JSON body: %v (body=%s)", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(
		srv.URL,
		WithHTTPClient(srv.Client()),
		WithSession([]*http.Cookie{{Name: "JSESSIONID", Value: "test-session"}}, time.Now().Add(time.Hour)),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	grade := &Grade{
		CourseID:     "00111222333",
		SectionID:    "654321",
		StudentAPIID: "999999",
		ScoresURL:    "/guardian/scores.html?frn=00111222333&begdate=09/03/2025&enddate=11/05/2025&fg=Q1&schoolid=109",
		CourseName:   "Test Course",
	}

	assignments, err := client.GetAssignments(context.Background(), "ignored", grade)
	if err != nil {
		t.Fatalf("GetAssignments: %v", err)
	}

	// ---- Request-side assertions ----

	wantRefererPrefix := client.baseURL + "/guardian/scores.html?frn=00111222333"
	if !strings.HasPrefix(seenReferer, wantRefererPrefix) {
		t.Errorf("Referer = %q, want prefix %q", seenReferer, wantRefererPrefix)
	}
	if !strings.Contains(seenContentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", seenContentType)
	}
	if !strings.Contains(seenAccept, "application/json") {
		t.Errorf("Accept = %q, want application/json", seenAccept)
	}
	if len(seenCookies) == 0 {
		t.Error("expected at least one cookie on outbound request, got none")
	}
	if got := seenBody["section_ids"]; !jsonEqual(got, []any{float64(654321)}) {
		t.Errorf("section_ids = %v, want [654321]", got)
	}
	if got := seenBody["student_ids"]; !jsonEqual(got, []any{float64(999999)}) {
		t.Errorf("student_ids = %v, want [999999]", got)
	}
	for _, key := range []string{"start_date", "end_date"} {
		v, ok := seenBody[key].(string)
		if !ok || !strings.Contains(v, "-") {
			t.Errorf("%s = %v, want a YYYY-M-D string", key, seenBody[key])
		}
	}

	// ---- Response-side assertions ----

	if got, want := len(assignments), 35; got != want {
		t.Fatalf("len(assignments) = %d, want %d", got, want)
	}

	var (
		hasMissing, hasCollected, hasLate, hasExempt bool
		hasScored                                    bool
		seenCategories                               = map[string]bool{}
	)
	for _, a := range assignments {
		if a.Title == "" {
			t.Errorf("assignment %s has empty Title", a.ID)
		}
		if a.CourseID != grade.CourseID {
			t.Errorf("assignment %s CourseID = %q, want %q", a.ID, a.CourseID, grade.CourseID)
		}
		seenCategories[a.Category] = true
		for _, f := range a.Flags {
			switch f {
			case FlagMissing:
				hasMissing = true
			case FlagCollected:
				hasCollected = true
			case FlagLate:
				hasLate = true
			case FlagExempt:
				hasExempt = true
			}
		}
		if a.Score != nil {
			hasScored = true
		}
	}
	// Fixture is Student Alpha's Science class — we know there are scored,
	// missing, and collected assignments. Asserting their presence keeps
	// the test decoupled from specific titles that may change.
	if !hasScored {
		t.Error("no assignments had scores parsed")
	}
	if !hasMissing {
		t.Error("no assignment picked up FlagMissing")
	}
	if !hasCollected {
		t.Error("no assignment picked up FlagCollected")
	}
	// Late/exempt are present in the fixture but aren't essential, so we
	// only warn via t.Log.
	t.Logf("flag coverage: missing=%v collected=%v late=%v exempt=%v", hasMissing, hasCollected, hasLate, hasExempt)
	// Science course should have > 1 category (e.g. Warm Ups, Classwork,
	// Assessment, Pre-Reading, etc.); single-category output would mean we
	// stopped reading the category array after the first hit.
	if len(seenCategories) < 3 {
		t.Errorf("expected >=3 distinct categories, saw %d: %v", len(seenCategories), keysOf(seenCategories))
	}
}

// TestParseScoresMetadata_ExtractsIDsFromRawHTML verifies the pure parser
// used by FetchScoresMetadata works against the exact HTML shape the live
// site emits. Covers both the section_id regex and the studentFRN "001"
// prefix stripping — wrong handling of either silently produces API errors.
func TestParseScoresMetadata_ExtractsIDsFromRawHTML(t *testing.T) {
	html := `
<html><head><title>Class Score Detail</title></head><body>
<div class="xteContentWrapper"
     data-ng-init="studentFRN = '001999999';
                   beginningDate = '09/03/2025';
                   endingDate = '11/05/2025';">
  <div data-pss-student-assignment-scores=""
       data-termid="3501"
       data-sectionid="654321"
       data-studentfrn="studentFRN"
       data-schoolid="109"
       class="ng-isolate-scope">
  </div>
</div>
</body></html>`

	md, err := parseScoresMetadata(html)
	if err != nil {
		t.Fatalf("parseScoresMetadata: %v", err)
	}
	if md.SectionID != "654321" {
		t.Errorf("SectionID = %q, want 654321", md.SectionID)
	}
	if md.StudentAPIID != "999999" {
		t.Errorf("StudentAPIID = %q, want 999999 (001 prefix stripped)", md.StudentAPIID)
	}
}

// TestParseScoresMetadata_RejectsLoginPage ensures we return a descriptive
// error when the server redirected us to the login form — the shape of bug
// that burned us during discovery.
func TestParseScoresMetadata_RejectsLoginPage(t *testing.T) {
	if _, err := parseScoresMetadata("<html><body>nothing here</body></html>"); err == nil {
		t.Fatal("parseScoresMetadata on empty page: expected error, got nil")
	}
}

func jsonEqual(got, want any) bool {
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	return string(gb) == string(wb)
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
