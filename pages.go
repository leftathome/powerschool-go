package powerschool

import (
	"errors"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ErrNotImplemented is returned by calls whose page structure has never
// been captured. They return this error instead of empty data, so "nothing
// there" cannot be confused with "not read".
var ErrNotImplemented = errors.New("not implemented")

// PageStudent is the evidence a page itself carries of which student it
// describes.
//
// Every guardian page describes the student the SESSION has selected,
// whatever the caller had in mind, and every guardian page carries the
// portal's student switcher with exactly one entry marked selected
// (verified live on home.html, attendance.html, termgrades.html and
// teachercomments.html). A caller that attributes a page to a student must
// compare SelectedID with that student's id. The library never fills these
// fields from anything but the page they came from.
type PageStudent struct {
	// SelectedID is the switchStudent() id of the student the page's own
	// switcher marks as selected (#students-list li.selected). The parsers
	// of attendance.html, termgrades.html and teachercomments.html fail
	// with ErrSelectionUnknown rather than return a page without it.
	SelectedID string
	// HeadingName is the student name the page's main heading ends with,
	// e.g. "Rivera, Ana" from "Teacher Comments: Rivera, Ana", or "" when
	// the heading names nobody (termgrades.html). It is a display name,
	// for people; do not use it to decide whose page this is.
	HeadingName string
}

// requireSelected is the fail-closed check of the three page parsers: a
// page that does not mark exactly one student as selected is not returned.
func requireSelected(ps PageStudent) error {
	if ps.SelectedID == "" {
		return ErrSelectionUnknown
	}
	return nil
}

// pageStudent reads the selected-student evidence out of a parsed page.
func pageStudent(doc *goquery.Document) PageStudent {
	return PageStudent{
		SelectedID:  selectedStudentID(doc),
		HeadingName: headingName(doc),
	}
}

// headingName returns the name after the colon in the first main heading
// that has one ("<page title>: <Last, First>").
func headingName(doc *goquery.Document) string {
	name := ""
	scope := doc.Find("#content-main h1")
	if scope.Length() == 0 {
		scope = doc.Find("h1")
	}
	scope.EachWithBreak(func(i int, h *goquery.Selection) bool {
		text := cleanText(h.Text())
		if _, after, ok := strings.Cut(text, ":"); ok {
			if after = strings.TrimSpace(after); after != "" {
				name = after
				return false
			}
		}
		return true
	})
	return name
}

// cleanText trims s and collapses internal whitespace (including
// non-breaking spaces) to single spaces, and drops the Unicode direction
// marks the portal puts inside date ranges.
func cleanText(s string) string {
	s = strings.NewReplacer("\u00a0", " ", "\u200e", "", "\u200f", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
