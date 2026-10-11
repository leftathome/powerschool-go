package powerschool

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// MaxTeacherCommentBytes bounds Assignment.TeacherComment and
// Assignment.TeacherCommentHTML, each on its own. A longer value is cut at
// a UTF-8 boundary and Assignment.TeacherCommentTruncated is set.
const MaxTeacherCommentBytes = 16 << 10

// APIAssignmentScoreComment is the "_assignmentscorecomment" member of a
// score in the assignment-lookup response: the note a teacher left on one
// student's score. The portal's class score page shows its notes icon when
// this object and its "commentvalue" are both non-null, and renders the
// value as HTML (white-space: pre-wrap) in the assignment detail dialog.
//
// PowerSchool omits null members, so a score without a note has no such
// key at all.
type APIAssignmentScoreComment struct {
	// CommentValue is "commentvalue" as sent: HTML, or plain text with
	// line breaks. Empty when the member is absent, null, or not a string.
	CommentValue string
}

// UnmarshalJSON never fails: a comment in a shape this library does not
// know must not cost the caller the whole assignment list. Anything other
// than an object with a string "commentvalue" decodes as no comment; other
// members are ignored.
func (c *APIAssignmentScoreComment) UnmarshalJSON(data []byte) error {
	c.CommentValue = ""
	var obj struct {
		CommentValue json.RawMessage `json:"commentvalue"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	var value string
	if err := json.Unmarshal(obj.CommentValue, &value); err == nil {
		c.CommentValue = value
	}
	return nil
}

// setTeacherComment fills the four teacher-comment fields of a from the
// raw "commentvalue" of the score the assignment was built from.
func (a *Assignment) setTeacherComment(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	text, textCut := truncateUTF8(htmlToText(raw), MaxTeacherCommentBytes)
	rawKept, rawCut := truncateUTF8(raw, MaxTeacherCommentBytes)
	a.TeacherComment = strings.TrimRight(text, " \n")
	a.TeacherCommentHTML = rawKept
	a.TeacherCommentTruncated = textCut || rawCut
	a.HasTeacherComment = a.TeacherComment != ""
}

// truncateUTF8 cuts s to at most max bytes without splitting a character.
func truncateUTF8(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// textBlockTags are the elements whose start and end put what follows on a
// new line.
var textBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"dd": true, "div": true, "dl": true, "dt": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "li": true, "ol": true, "p": true,
	"pre": true, "section": true, "table": true, "tr": true, "ul": true,
}

// htmlToText reduces a fragment of HTML (or plain text) to readable plain
// text:
//
//   - tags are removed and character references decoded;
//   - <br> is a line break; the start and the end of a block element (<p>,
//     <div>, <li>, <tr>, headings, ...) end the current line; a table cell
//     ends with a space;
//   - <script> and <style> content and HTML comments are dropped;
//   - line breaks already in the text are kept (the portal shows the value
//     with white-space: pre-wrap, so they are the teacher's own);
//   - within a line, runs of whitespace (non-breaking spaces included)
//     become one space and the ends are trimmed; control characters and
//     direction marks are dropped; more than one blank line in a row
//     becomes one, and leading and trailing blank lines are removed.
//
// It walks the tokens and builds no tree, so its cost is linear in the
// input whatever the nesting.
func htmlToText(s string) string {
	var b strings.Builder
	atLineStart := true
	write := func(t string) {
		if t == "" {
			return
		}
		b.WriteString(t)
		atLineStart = strings.HasSuffix(t, "\n")
	}
	endLine := func() {
		if !atLineStart {
			write("\n")
		}
	}
	skip := "" // "script" or "style" while inside one
	z := html.NewTokenizer(strings.NewReader(s))
loop:
	for {
		switch z.Next() {
		case html.ErrorToken:
			break loop
		case html.TextToken:
			if skip == "" {
				write(html.UnescapeString(string(z.Raw())))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch {
			case skip != "":
			case tag == "script" || tag == "style":
				skip = tag
			case tag == "br":
				write("\n")
			case textBlockTags[tag]:
				endLine()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch {
			case skip != "":
				if tag == skip {
					skip = ""
				}
			case tag == "td" || tag == "th":
				write(" ")
			case textBlockTags[tag]:
				endLine()
			}
		}
	}

	text := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(b.String())
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == 0x200b || r == 0x200e || r == 0x200f || r == 0xfeff: // zero-width space, direction marks, BOM
			return -1
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, text)

	var lines []string
	blank := true // drops leading blank lines
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		blank = false
		lines = append(lines, line)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
