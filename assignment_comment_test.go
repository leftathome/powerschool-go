package powerschool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Everything in this file is synthetic: the JSON follows the shape of the
// assignment-lookup response (see testdata/assignments_lookup.json and
// docs/API_ENDPOINTS.md) but every name, id, score and comment is invented.

// lookupAssignments serves body as the assignment-lookup response and
// returns what GetAssignments makes of it.
func lookupAssignments(t *testing.T, body string) ([]*Assignment, error) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != AssignmentLookupPath || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(
		srv.URL,
		WithHTTPClient(srv.Client()),
		WithSession([]*http.Cookie{{Name: "JSESSIONID", Value: "test-session"}}, time.Now().Add(time.Hour)),
		WithLogLevel(LogLevelNone),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	grade := &Grade{
		CourseID:     "00111222333",
		SectionID:    "654321",
		StudentAPIID: "999999",
		ScoresURL:    "/guardian/scores.html?frn=00111222333&begdate=09/03/2025&enddate=11/05/2025&fg=Q1&schoolid=109",
	}
	return client.GetAssignments(context.Background(), "ignored", grade)
}

// syntheticScore is one "_assignmentscores" entry; extra is spliced in as
// further members (with its own leading comma) so a test controls exactly
// how the comment member appears, or that it does not.
func syntheticScore(points int, extra string) string {
	return fmt.Sprintf(`{
  "_name": "assignmentscore",
  "scorepoints": %d,
  "scorepercent": %d,
  "scorelettergrade": "B",
  "actualscoreentered": "%d",
  "actualscorekind": "REAL_SCORE",
  "scoreentrydate": "2025-09-21 17:16:37",
  "whenmodified": "2025-09-21",
  "islate": false, "ismissing": false, "isincomplete": false,
  "isabsent": false, "isexempt": false, "iscollected": false,
  "authoredbyuc": false,
  "studentsdcid": 999999%s
}`, points, points*10, points*10, extra)
}

// syntheticSection is one "_assignmentsections" entry.
func syntheticSection(name, description, category, scores string) string {
	nameJSON, _ := json.Marshal(name)
	descJSON, _ := json.Marshal(description)
	return fmt.Sprintf(`{
  "_name": "assignmentsection",
  "_id": 2000001,
  "assignmentsectionid": 2000001,
  "name": %s,
  "description": %s,
  "duedate": "2025-09-18",
  "scoretype": "POINTS",
  "scoreentrypoints": 10,
  "totalpointvalue": 10,
  "weight": 1,
  "iscountedinfinalgrade": true,
  "isscorespublish": true,
  "isscoringneeded": true,
  "sectionsdcid": 654000,
  "_assignmentcategoryassociations": [%s],
  "_assignmentscores": [%s]
}`, nameJSON, descJSON, category, scores)
}

func syntheticCategory(name, descriptionMember string, primary bool) string {
	return fmt.Sprintf(`{
  "_name": "assignmentcategoryassoc",
  "_id": 2000101,
  "assignmentcategoryassocid": 2000101,
  "isprimary": %t,
  "_teachercategory": {"_name": "teachercategory", "color": "4", "name": %q%s}
}`, primary, name, descriptionMember)
}

func syntheticAssignment(id int, sections ...string) string {
	return fmt.Sprintf(`{
  "_name": "assignment",
  "_id": %d,
  "assignmentid": %d,
  "hasstandards": false,
  "standardscoringmethod": "GradeScale",
  "_assignmentsections": [%s]
}`, id, id, strings.Join(sections, ","))
}

// oneAssignment is a lookup response of one assignment with one section and
// one score whose extra members are scoreExtra.
func oneAssignment(scoreExtra string) string {
	return "[" + syntheticAssignment(1000001,
		syntheticSection("Practice Set 1", "", syntheticCategory("Practice", "", true),
			syntheticScore(8, scoreExtra))) + "]"
}

func commentMember(value string) string {
	v, _ := json.Marshal(value)
	return `, "_assignmentscorecomment": {"_name": "assignmentscorecomment", "_id": 3000001, "commentvalue": ` + string(v) + `}`
}

func TestGetAssignments_TeacherComment(t *testing.T) {
	tests := []struct {
		name       string
		scoreExtra string
		wantText   string
		wantHTML   string
	}{
		{
			name:       "comment present",
			scoreExtra: commentMember("Careful work on every step."),
			wantText:   "Careful work on every step.",
			wantHTML:   "Careful work on every step.",
		},
		{
			name: "comment object with members this library does not know",
			scoreExtra: `, "_assignmentscorecomment": {"commentvalue": "See me after class.",
				"whenmodified": "2025-09-22", "authoredbyuc": false, "_extra": {"list": [1, 2, {"deep": null}]}}`,
			wantText: "See me after class.",
			wantHTML: "See me after class.",
		},
		{name: "key absent (how PowerSchool sends a score without a note)", scoreExtra: ""},
		{name: "null object", scoreExtra: `, "_assignmentscorecomment": null`},
		{name: "null value", scoreExtra: `, "_assignmentscorecomment": {"_name": "assignmentscorecomment", "commentvalue": null}`},
		{name: "object without the value member", scoreExtra: `, "_assignmentscorecomment": {"_name": "assignmentscorecomment"}`},
		{name: "empty string", scoreExtra: commentMember("")},
		{name: "whitespace only", scoreExtra: commentMember(" \n\t ")},
		{name: "value is not a string", scoreExtra: `, "_assignmentscorecomment": {"commentvalue": 42}`},
		{name: "member is not an object", scoreExtra: `, "_assignmentscorecomment": ["unexpected"]`},
		{name: "member is a bare string", scoreExtra: `, "_assignmentscorecomment": "unexpected"`},
		{
			name:       "HTML with entities, <br> and <p>",
			scoreExtra: commentMember(`<p>Strong start, Student Alpha &amp; lab partner!</p><p>Redo question&nbsp;3:<br>show the steps for 2 &lt; x &lt; 5.</p>`),
			wantText:   "Strong start, Student Alpha & lab partner!\nRedo question 3:\nshow the steps for 2 < x < 5.",
			wantHTML:   `<p>Strong start, Student Alpha &amp; lab partner!</p><p>Redo question&nbsp;3:<br>show the steps for 2 &lt; x &lt; 5.</p>`,
		},
		{
			name:       "plain text keeps the teacher's line breaks and collapses the rest",
			scoreExtra: commentMember("  First point.\r\n\r\n\r\n\r\nSecond   point,\twith  gaps.  \nThird point.\n"),
			wantText:   "First point.\n\nSecond point, with gaps.\nThird point.",
			wantHTML:   "First point.\r\n\r\n\r\n\r\nSecond   point,\twith  gaps.  \nThird point.",
		},
		{
			name:       "markup with no text is not a readable comment",
			scoreExtra: commentMember(`<p><br></p>`),
			wantText:   "",
			wantHTML:   `<p><br></p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookupAssignments(t, oneAssignment(tt.scoreExtra))
			if err != nil {
				t.Fatalf("GetAssignments: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("len(assignments) = %d, want 1", len(got))
			}
			a := got[0]
			if a.TeacherComment != tt.wantText {
				t.Errorf("TeacherComment = %q, want %q", a.TeacherComment, tt.wantText)
			}
			if a.TeacherCommentHTML != tt.wantHTML {
				t.Errorf("TeacherCommentHTML = %q, want %q", a.TeacherCommentHTML, tt.wantHTML)
			}
			if want := tt.wantText != ""; a.HasTeacherComment != want {
				t.Errorf("HasTeacherComment = %v, want %v", a.HasTeacherComment, want)
			}
			if a.TeacherCommentTruncated {
				t.Error("TeacherCommentTruncated = true for a short comment")
			}
			// The comment must never disturb the rest of the assignment.
			if a.Title != "Practice Set 1" || a.Category != "Practice" || a.Status != StatusGraded {
				t.Errorf("assignment = {Title:%q Category:%q Status:%q}, want Practice Set 1 / Practice / graded", a.Title, a.Category, a.Status)
			}
			if a.Score == nil || *a.Score != 8 || a.LetterGrade != "B" {
				t.Errorf("score = %v / %q, want 8 / B", a.Score, a.LetterGrade)
			}
		})
	}
}

func TestGetAssignments_TeacherCommentIsBounded(t *testing.T) {
	e := string(rune(0xe9)) // a two-byte character, to land the cut mid-character

	t.Run("oversized plain text", func(t *testing.T) {
		value := strings.Repeat("abcdefghij ", 2000) // 22000 bytes
		got, err := lookupAssignments(t, oneAssignment(commentMember(value)))
		if err != nil {
			t.Fatalf("GetAssignments: %v", err)
		}
		a := got[0]
		if !a.TeacherCommentTruncated || !a.HasTeacherComment {
			t.Errorf("Truncated = %v, Has = %v, want both true", a.TeacherCommentTruncated, a.HasTeacherComment)
		}
		if n := len(a.TeacherComment); n > MaxTeacherCommentBytes || n < MaxTeacherCommentBytes-16 {
			t.Errorf("len(TeacherComment) = %d, want just under %d", n, MaxTeacherCommentBytes)
		}
		if n := len(a.TeacherCommentHTML); n > MaxTeacherCommentBytes || n < MaxTeacherCommentBytes-16 {
			t.Errorf("len(TeacherCommentHTML) = %d, want just under %d", n, MaxTeacherCommentBytes)
		}
		if !strings.HasPrefix(value, a.TeacherComment) {
			t.Error("TeacherComment is not a prefix of the comment")
		}
	})

	t.Run("the cut never splits a character", func(t *testing.T) {
		value := "x" + strings.Repeat(e, MaxTeacherCommentBytes) // odd offset: the bound falls inside a character
		got, err := lookupAssignments(t, oneAssignment(commentMember(value)))
		if err != nil {
			t.Fatalf("GetAssignments: %v", err)
		}
		a := got[0]
		if !a.TeacherCommentTruncated {
			t.Error("TeacherCommentTruncated = false, want true")
		}
		for name, s := range map[string]string{"TeacherComment": a.TeacherComment, "TeacherCommentHTML": a.TeacherCommentHTML} {
			if !utf8.ValidString(s) {
				t.Errorf("%s is not valid UTF-8 after the cut", name)
			}
			if len(s) != MaxTeacherCommentBytes-1 {
				t.Errorf("len(%s) = %d, want %d", name, len(s), MaxTeacherCommentBytes-1)
			}
		}
	})

	t.Run("markup over the bound, text under it", func(t *testing.T) {
		value := strings.Repeat("<p><span>ok</span></p>", 1000) // 22000 bytes of markup, 2999 of text
		got, err := lookupAssignments(t, oneAssignment(commentMember(value)))
		if err != nil {
			t.Fatalf("GetAssignments: %v", err)
		}
		a := got[0]
		if !a.TeacherCommentTruncated {
			t.Error("TeacherCommentTruncated = false, want true (the HTML was cut)")
		}
		if len(a.TeacherCommentHTML) != MaxTeacherCommentBytes {
			t.Errorf("len(TeacherCommentHTML) = %d, want %d", len(a.TeacherCommentHTML), MaxTeacherCommentBytes)
		}
		// The text is taken from the whole value, so it is complete.
		if want := strings.TrimSuffix(strings.Repeat("ok\n", 1000), "\n"); a.TeacherComment != want {
			t.Errorf("TeacherComment has %d bytes, want the complete %d", len(a.TeacherComment), len(want))
		}
	})

	t.Run("exactly at the bound is not truncated", func(t *testing.T) {
		value := strings.Repeat("y", MaxTeacherCommentBytes)
		got, err := lookupAssignments(t, oneAssignment(commentMember(value)))
		if err != nil {
			t.Fatalf("GetAssignments: %v", err)
		}
		if a := got[0]; a.TeacherCommentTruncated || a.TeacherComment != value || a.TeacherCommentHTML != value {
			t.Errorf("Truncated = %v, len(text) = %d, len(html) = %d; want the value untouched",
				a.TeacherCommentTruncated, len(a.TeacherComment), len(a.TeacherCommentHTML))
		}
	})
}

// TestGetAssignments_TeacherCommentPerSectionFirstScore pins which score a
// comment is read from. GetAssignments returns one Assignment per section
// and has always read a section's first score; the comment follows the same
// rule, so it can never be attached to a different score than the points
// and flags beside it.
func TestGetAssignments_TeacherCommentPerSectionFirstScore(t *testing.T) {
	body := "[" + strings.Join([]string{
		syntheticAssignment(1000001,
			syntheticSection("Shared Project (section one)", "", syntheticCategory("Projects", "", true),
				syntheticScore(9, commentMember("First score of section one."))+","+
					syntheticScore(4, commentMember("Second score of section one."))),
			syntheticSection("Shared Project (section two)", "", syntheticCategory("Projects", "", true),
				syntheticScore(6, "")+","+
					syntheticScore(7, commentMember("Second score of section two.")))),
		syntheticAssignment(1000002,
			syntheticSection("Not scored yet", "", syntheticCategory("Projects", "", true), "")),
	}, ",") + "]"

	got, err := lookupAssignments(t, body)
	if err != nil {
		t.Fatalf("GetAssignments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(assignments) = %d, want 3 (one per section)", len(got))
	}

	one, two, unscored := got[0], got[1], got[2]
	if one.ID != "1000001" || two.ID != "1000001" {
		t.Errorf("IDs = %q, %q; both sections belong to assignment 1000001", one.ID, two.ID)
	}
	if one.Score == nil || *one.Score != 9 || one.TeacherComment != "First score of section one." {
		t.Errorf("section one: score = %v, comment = %q; want the first score (9) and its comment", one.Score, one.TeacherComment)
	}
	// Section two's first score has no comment: the second score's comment
	// must not be borrowed.
	if two.Score == nil || *two.Score != 6 || two.HasTeacherComment || two.TeacherComment != "" || two.TeacherCommentHTML != "" {
		t.Errorf("section two: score = %v, has = %v, comment = %q; want the first score (6) and no comment",
			two.Score, two.HasTeacherComment, two.TeacherComment)
	}
	if unscored.Score != nil || unscored.HasTeacherComment || unscored.Status != StatusPending {
		t.Errorf("unscored: score = %v, has = %v, status = %q; want none / false / pending",
			unscored.Score, unscored.HasTeacherComment, unscored.Status)
	}
}

func TestGetAssignments_CategoryDescription(t *testing.T) {
	body := "[" + strings.Join([]string{
		// The primary category is second; the first one's description must
		// not be used.
		syntheticAssignment(1000001, syntheticSection("Unit Quiz", "<p>Covers chapters 1 &amp; 2</p>",
			syntheticCategory("Extra", `, "description": "Not the primary category"`, false)+","+
				syntheticCategory("Quizzes", `, "description": "<p>Short checks,&nbsp;one per unit</p>"`, true),
			syntheticScore(7, ""))),
		// PowerSchool omits a null description.
		syntheticAssignment(1000002, syntheticSection("Unit Test", "", syntheticCategory("Tests", "", true), "")),
		syntheticAssignment(1000003, syntheticSection("Reading Log", "", syntheticCategory("Homework", `, "description": null`, true), "")),
	}, ",") + "]"

	got, err := lookupAssignments(t, body)
	if err != nil {
		t.Fatalf("GetAssignments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(assignments) = %d, want 3", len(got))
	}
	if got[0].Category != "Quizzes" || got[0].CategoryDescription != "Short checks, one per unit" {
		t.Errorf("assignment 0: Category = %q, CategoryDescription = %q; want Quizzes / Short checks, one per unit",
			got[0].Category, got[0].CategoryDescription)
	}
	// Description is unchanged by this release: tags stripped, nothing else.
	if got[0].Description != "Covers chapters 1 &amp; 2" {
		t.Errorf("assignment 0: Description = %q, want the existing tag-stripped form", got[0].Description)
	}
	for _, a := range got[1:] {
		if a.CategoryDescription != "" {
			t.Errorf("%s: CategoryDescription = %q, want empty", a.Title, a.CategoryDescription)
		}
	}
}

// TestCapturedFixture_HasNoScoreComments records what the captured response
// looks like: no score carries a comment member. It guards the claim in the
// docs that the key is absent rather than null.
func TestCapturedFixture_HasNoScoreComments(t *testing.T) {
	fixture, err := os.ReadFile("testdata/assignments_lookup.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if strings.Contains(string(fixture), "_assignmentscorecomment") {
		t.Fatal("the fixture now has a comment member; cover it in TestGetAssignments_ParsesFixtureAndEnforcesHeaders")
	}
	got, err := lookupAssignments(t, string(fixture))
	if err != nil {
		t.Fatalf("GetAssignments: %v", err)
	}
	for _, a := range got {
		if a.HasTeacherComment || a.TeacherComment != "" || a.TeacherCommentHTML != "" || a.TeacherCommentTruncated {
			t.Errorf("assignment %s has teacher-comment fields set without a comment in the response", a.ID)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	nbsp := string(rune(0xa0))
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "Nice job.", "Nice job."},
		{"entities", "5 &gt; 3 &amp;&amp; 3 &lt; 5 &quot;ok&quot; &#39;yes&#39; &#x41;", `5 > 3 && 3 < 5 "ok" 'yes' A`},
		{"escaped entity stays text", "write &amp;lt; for less-than", "write &lt; for less-than"},
		{"br variants", "one<br>two<br/>three<BR />four", "one\ntwo\nthree\nfour"},
		{"double br is one blank line", "one<br><br>two<br><br><br><br>three", "one\n\ntwo\n\nthree"},
		{"paragraphs", "<p>one</p><p>two</p>", "one\ntwo"},
		// A line break written between two blocks is kept, as it is on
		// the portal (white-space: pre-wrap), but never more than one
		// blank line.
		{"paragraphs with source newlines", "<p>one</p>\n<p>two</p>\n\n\n<p>three</p>\n", "one\n\ntwo\n\nthree"},
		{"divs and inline tags", "<div>Well <b>done</b>, <i>really</i>.</div><div>Next: <a href=\"https://example.invalid/x\">the rubric</a></div>", "Well done, really.\nNext: the rubric"},
		{"list", "Fix:<ul><li>units</li><li>labels</li></ul>then resubmit", "Fix:\nunits\nlabels\nthen resubmit"},
		{"heading", "<h3>Feedback</h3>Good", "Feedback\nGood"},
		{"table cells are spaced", "<table><tr><td>Part A</td><td>4/5</td></tr><tr><td>Part B</td><td>3/5</td></tr></table>", "Part A 4/5\nPart B 3/5"},
		{"script and style dropped", "before<script>alert('<p>x</p>')</script><style>p{color:red}</style>after", "beforeafter"},
		{"unclosed script drops the rest", "before<script>alert(1)", "before"},
		{"comment dropped", "one<!-- hidden -->two", "onetwo"},
		{"nbsp and whitespace", "a&nbsp;&nbsp;b" + nbsp + nbsp + "c \t d", "a b c d"},
		{"line ends trimmed", "  one  \n   two\t\n", "one\ntwo"},
		{"crlf", "one\r\ntwo\rthree", "one\ntwo\nthree"},
		{"control characters dropped", "one\x00\x07two\x1b[0m", "onetwo[0m"},
		{"less-than that is not a tag", "score < 5 and > 3", "score < 5 and > 3"},
		{"unterminated tag at the end", "kept <b", "kept"},
		{"attributes are not text", `<span style="color:red" title="hidden">shown</span>`, "shown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := htmlToText(tt.in); got != tt.want {
				t.Errorf("htmlToText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHTMLToText_DeepNestingIsLinear(t *testing.T) {
	in := strings.Repeat("<div>", 200000) + "deep" + strings.Repeat("</div>", 200000)
	if got := htmlToText(in); got != "deep" {
		t.Errorf("htmlToText(deeply nested) = %q, want %q", got, "deep")
	}
}

func TestAPIAssignmentScoreComment_UnmarshalNeverFails(t *testing.T) {
	for _, in := range []string{`{}`, `{"commentvalue":null}`, `{"commentvalue":{"a":1}}`, `[]`, `"text"`, `12`, `true`} {
		var c APIAssignmentScoreComment
		c.CommentValue = "stale"
		if err := json.Unmarshal([]byte(in), &c); err != nil {
			t.Errorf("Unmarshal(%s): %v", in, err)
		}
		if c.CommentValue != "" {
			t.Errorf("Unmarshal(%s): CommentValue = %q, want empty", in, c.CommentValue)
		}
	}
	var c APIAssignmentScoreComment
	if err := json.Unmarshal([]byte(`{"COMMENTVALUE":"x","other":1}`), &c); err != nil || c.CommentValue != "x" {
		t.Errorf("Unmarshal with other members: %q, %v", c.CommentValue, err)
	}
}
