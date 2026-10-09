package powerschool

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

// The portal keeps "which student am I showing" as server-side state of the
// session. Its student switcher is
//
//	<form id="switch_student_form" action="home.html" method="post">
//	  <input type="hidden" name="selected_student_id"/></form>
//
// and switchStudent(id) sets that one field and submits the form. Nothing
// else is sent, and nothing but the session's selection changes.
const (
	// SwitchStudentPath is where the switcher form posts.
	SwitchStudentPath = "/guardian/home.html"
	// SwitchStudentField is the form's only field.
	SwitchStudentField = "selected_student_id"
)

// SwitchStudent makes studentID the student the session has selected, and
// confirms it. See SelectStudent.
func (c *Client) SwitchStudent(ctx context.Context, studentID string) error {
	_, err := c.SelectStudent(ctx, studentID)
	return err
}

// SelectStudent makes studentID the session's selected student and returns
// that student's home page.
//
// It reads the home page first. If the page already marks studentID as
// selected, that page is returned and nothing is posted. Otherwise it
// submits the portal's own switcher form -- POST SwitchStudentPath with the
// single field selected_student_id -- through the client's http.Client,
// reads the home page again, and requires that page to mark studentID as
// selected.
//
// It never guesses:
//   - ErrSelectionUnknown: the page marks no student as selected, so a
//     switch could not be confirmed. Nothing is posted.
//   - ErrNotFound: studentID is not in the account's switcher. Nothing is
//     posted.
//   - ErrSwitchNotConfirmed: the form was posted but the page read
//     afterwards does not mark studentID as selected.
//
// The session's selection is shared state: two SelectStudent calls (or a
// SelectStudent and a read) running concurrently on one session will read
// each other's student. Callers must serialise.
func (c *Client) SelectStudent(ctx context.Context, studentID string) (*HomePage, error) {
	if !isDigits(studentID) {
		return nil, NewError(CodeNotFound, "student id must be numeric", ErrNotFound)
	}

	page, err := c.GetHomePage(ctx)
	if err != nil {
		return nil, err
	}
	selected := page.SelectedStudent()
	if selected == nil {
		return nil, ErrSelectionUnknown
	}
	if selected.ID == studentID {
		return page, nil
	}
	known := false
	for _, s := range page.Students {
		if s.ID == studentID {
			known = true
			break
		}
	}
	if !known {
		return nil, ErrNotFound
	}

	c.logger.Info("Switching the selected student")
	if err := c.postSwitchStudent(ctx, studentID); err != nil {
		return nil, err
	}

	// Never trust the POST's own response: read the page again.
	page, err = c.GetHomePage(ctx)
	if err != nil {
		return nil, err
	}
	if now := page.SelectedStudent(); now == nil || now.ID != studentID {
		return nil, ErrSwitchNotConfirmed
	}
	return page, nil
}

// postSwitchStudent submits the switcher form. The response body is
// discarded; a redirect (the usual answer to a form post) is fine whether
// or not the http.Client followed it.
func (c *Client) postSwitchStudent(ctx context.Context, studentID string) error {
	body := url.Values{SwitchStudentField: {studentID}}.Encode()
	headers := map[string]string{
		"Referer": c.baseURL + SwitchStudentPath,
		"Origin":  c.baseURL,
	}
	resp, err := c.doRequestWithHeaders(ctx, http.MethodPost, SwitchStudentPath,
		[]byte(body), "application/x-www-form-urlencoded", headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, maxPageBytes))

	if resp.Request != nil && resp.Request.URL != nil && isPublicPath(resp.Request.URL.Path) {
		c.session = nil
		return errLoginPage("student switch")
	}
	if resp.StatusCode >= 400 {
		return WrapNetworkError(&HTTPStatusError{
			Op:         "POST " + SwitchStudentPath,
			StatusCode: resp.StatusCode,
			BodyLen:    int(n),
		})
	}
	return nil
}

func isDigits(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
