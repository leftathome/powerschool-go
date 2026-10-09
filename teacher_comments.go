package powerschool

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// TeacherCommentsPath is the teacher comments page.
const TeacherCommentsPath = "/guardian/teachercomments.html"

// TeacherCommentsTermParam is the query parameter that selects the
// reporting term (verified live). The page's selector is named "storecode",
// but that name has no effect as a parameter: the selector's option values
// are URLs of the form teachercomments.html?fg=<term>.
const TeacherCommentsTermParam = "fg"

// ErrTermMismatch is returned by GetTeacherComments when the page that came
// back does not mark the requested reporting term as the selected one.
var ErrTermMismatch = errors.New("the page shows a different reporting term than the one requested")

// TeacherCommentsPage is /guardian/teachercomments.html: one row per
// course with the teacher's comment for one reporting term.
type TeacherCommentsPage struct {
	// Student is the page's own evidence of whose comments these are. See
	// PageStudent.
	Student PageStudent
	// Term is the reporting term the page shows: the term of the selector
	// option the page marks as selected. The bare page marks the current
	// term. Empty when the page marks no term.
	Term string
	// TermConfirmed is true when the page itself marked Term. It is false
	// only for a page that marks no term option, in which case Term is
	// empty too.
	TermConfirmed bool
	// Terms lists the reporting terms the selector offers (Q1, Q2, ...),
	// in page order, without the placeholder entry.
	Terms []string
	// Comments has one entry per course row, in page order, including
	// rows whose comment is empty.
	Comments []*TeacherComment
}

// TeacherComment is one course row.
type TeacherComment struct {
	// Expression is the schedule slot, e.g. "3(A)".
	Expression string
	// CourseNumber is the district's course number, e.g. "SCI7001".
	CourseNumber string
	// CourseName is the course title.
	CourseName string
	// Teacher is the teacher's name ("Last, First M").
	Teacher string
	// Comment is the comment text with surrounding whitespace removed and
	// line breaks kept. Empty when the teacher left none.
	Comment string
}

var termRE = regexp.MustCompile(`^[A-Za-z0-9]{1,8}$`)

// GetTeacherComments fetches /guardian/teachercomments.html for the student
// the session has selected. Check TeacherCommentsPage.Student before
// attributing the result.
//
// term == "" fetches the bare page, which shows the current reporting term
// (TeacherCommentsPage.Term says which). Otherwise the page is requested
// with ?fg=<term> (a GET) and must mark that term as selected: if it marks
// another term, or none, the call fails with ErrTermMismatch rather than
// return comments under a term the page did not confirm.
func (c *Client) GetTeacherComments(ctx context.Context, term string) (*TeacherCommentsPage, error) {
	c.logger.Info("Fetching teacher comments")
	if term != "" && !termRE.MatchString(term) {
		return nil, NewError(CodeNotFound, "reporting term must be 1-8 letters or digits", ErrNotFound)
	}
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}
	path := TeacherCommentsPath
	if term != "" {
		path += "?" + url.Values{TeacherCommentsTermParam: {term}}.Encode()
	}
	html, err := c.getPageHTML(ctx, path)
	if err != nil {
		return nil, err
	}
	page, err := ParseTeacherCommentsPage(html)
	if err != nil {
		return nil, err
	}
	if term != "" && (!page.TermConfirmed || !strings.EqualFold(page.Term, term)) {
		return nil, ErrTermMismatch
	}
	return page, nil
}

// optionTerm returns the reporting term a selector option stands for.
//
// The option values are relative URLs: "teachercomments.html?fg=Q1" for a
// term, and for the first, blank option "teachercomments.html?fg=&
// showdropped=" -- or, when a term was requested, "teachercomments.html?
// fg=S1&showdropped=", which carries a term without being one. So an
// option is a term only if its value has a non-empty fg and no showdropped
// parameter. A value that is not such a URL falls back to the option's
// label when the label looks like a term.
func optionTerm(value, label string) string {
	value = strings.TrimSpace(value)
	if _, query, isURL := strings.Cut(value, "?"); isURL {
		q, err := url.ParseQuery(query)
		if err != nil {
			return ""
		}
		if _, dropped := q["showdropped"]; dropped {
			return ""
		}
		if term := q.Get(TeacherCommentsTermParam); termRE.MatchString(term) {
			return term
		}
		return ""
	}
	if termRE.MatchString(value) {
		return value
	}
	if value == "" && termRE.MatchString(label) {
		return label
	}
	return ""
}

// ParseTeacherCommentsPage parses the HTML of /guardian/teachercomments.html.
// Fails with ErrSelectionUnknown when the page does not mark exactly one
// student as selected.
func ParseTeacherCommentsPage(html string) (*TeacherCommentsPage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, WrapParseError(err, "teacher comments page")
	}
	page := &TeacherCommentsPage{Student: pageStudent(doc)}
	if err := requireSelected(page.Student); err != nil {
		return nil, err
	}

	doc.Find(`select[name="storecode"] option, select#reportingTermHeadingSelect option`).Each(func(i int, opt *goquery.Selection) {
		value, _ := opt.Attr("value")
		term := optionTerm(value, cleanText(opt.Text()))
		if term == "" {
			return // the blank first option
		}
		page.Terms = append(page.Terms, term)
		if _, selected := opt.Attr("selected"); selected {
			page.Term, page.TermConfirmed = term, true
		}
	})

	var table *goquery.Selection
	doc.Find("table.grid").EachWithBreak(func(i int, t *goquery.Selection) bool {
		header := cleanText(t.Find("tr").First().Text())
		if strings.Contains(header, "Teacher") && strings.Contains(header, "Comment") {
			table = t
			return false
		}
		return true
	})
	if table == nil {
		return nil, WrapParseError(fmt.Errorf("no comments table (table.grid with Teacher / Comment headers)"), "teacher comments page")
	}

	// Locate the columns by header text rather than by position.
	col := map[string]int{}
	table.Find("tr").First().Find("th").Each(func(i int, th *goquery.Selection) {
		switch h := strings.ToLower(cleanText(th.Text())); {
		case strings.HasPrefix(h, "exp"):
			col["exp"] = i
		case strings.HasPrefix(h, "course #"), strings.HasPrefix(h, "course number"):
			col["number"] = i
		case h == "course":
			col["course"] = i
		case h == "teacher":
			col["teacher"] = i
		case strings.HasPrefix(h, "comment"):
			col["comment"] = i
		}
	})
	for _, need := range []string{"course", "teacher", "comment"} {
		if _, ok := col[need]; !ok {
			return nil, WrapParseError(fmt.Errorf("comments table has no %q column", need), "teacher comments page")
		}
	}
	cell := func(cells *goquery.Selection, name string) *goquery.Selection {
		i, ok := col[name]
		if !ok || i >= cells.Length() {
			return nil
		}
		return cells.Eq(i)
	}
	text := func(s *goquery.Selection) string {
		if s == nil {
			return ""
		}
		return cleanText(s.Text())
	}

	table.Find("tr").Each(func(i int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() == 0 {
			return
		}
		comment := &TeacherComment{
			Expression:   text(cell(cells, "exp")),
			CourseNumber: text(cell(cells, "number")),
			CourseName:   text(cell(cells, "course")),
		}
		if t := cell(cells, "teacher"); t != nil {
			// The cell holds an info button and an "Email <name>" link.
			name := ""
			t.Find("a").Each(func(j int, a *goquery.Selection) {
				if label := cleanText(a.Text()); label != "" {
					name = label
				}
			})
			if name == "" {
				name = cleanText(t.Text())
			}
			comment.Teacher = strings.TrimPrefix(name, "Email ")
		}
		if cm := cell(cells, "comment"); cm != nil {
			comment.Comment = commentText(cm)
		}
		if comment.CourseName != "" {
			page.Comments = append(page.Comments, comment)
		}
	})
	return page, nil
}

// commentText extracts a comment from its table cell: the text of the
// cell's <pre> (or of the cell when it has none), with <br> as a line
// break, <script> and <style> content dropped, entities decoded and the
// teacher's own line breaks kept.
func commentText(cell *goquery.Selection) string {
	body := cell.Find("pre")
	if body.Length() == 0 {
		body = cell
	}
	body = body.First().Clone()
	body.Find("script, style").Remove()
	body.Find("br").ReplaceWithHtml("\n")
	raw := strings.ReplaceAll(body.Text(), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
