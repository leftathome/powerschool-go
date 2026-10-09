package powerschool

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// maxPageBytes caps how much of a page body getPageHTMLOverHTTP reads.
const maxPageBytes = 8 << 20

// WithHTTPPageFetch makes every post-login page read (GetStudents,
// GetGrades, GetGPA, GetGradeHistory, FetchScoresMetadata) a plain GET
// through the client's http.Client instead of a headless browser.
//
// Use it together with WithHTTPClient when the caller needs every byte of
// post-login traffic to pass through one transport -- for example a
// transport that refuses non-read-only methods. In this mode no browser is
// started outside Authenticate: the call that can only work through a
// browser (GetProgressReports) returns an error instead.
//
// The default (browser) mode is unchanged. Pages whose content is rendered
// client-side will parse as empty in HTTP mode; the home-page grades table
// and the scores-page metadata are server-rendered on the reference
// instance.
func WithHTTPPageFetch() Option {
	return func(c *Client) error {
		c.httpPages = true
		return nil
	}
}

// SetHTTPPageFetch toggles HTTP page fetching after construction. See
// WithHTTPPageFetch.
func (c *Client) SetHTTPPageFetch(on bool) {
	c.httpPages = on
}

// UsesHTTPPageFetch reports whether post-login page reads go through the
// client's http.Client (true) or a headless browser (false).
func (c *Client) UsesHTTPPageFetch() bool {
	return c.httpPages
}

// errBrowserDisabled is returned by browser-only calls in HTTP page mode.
func errBrowserDisabled(op string) error {
	return NewError(CodeUnknown, op+" needs a browser and is disabled by WithHTTPPageFetch", nil)
}

// getPageHTMLOverHTTP fetches a server-relative page with a GET through
// c.httpClient, carrying the session cookies. A redirect to the public
// login page, or any 401/403, is reported as ErrSessionExpired so a caller
// never parses the login form as an empty result.
func (c *Client) getPageHTMLOverHTTP(ctx context.Context, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	c.logger.Debug("Fetching page over HTTP: %s", pathOnly(path))

	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if isLoginRedirect(resp) {
		c.session = nil
		return "", errLoginPage("requested page")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return "", WrapNetworkError(err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", WrapNetworkError(&HTTPStatusError{
			Op:         "GET " + pathOnly(path),
			StatusCode: resp.StatusCode,
			BodyLen:    len(body),
		})
	}

	html := string(body)
	// A dead session is not always a redirect: the portal can answer the
	// original URL with 200 and the login form. Parsing that as a page
	// would read as "no students" / "no grades".
	if isLoginPage(html) {
		c.session = nil
		return "", errLoginPage("requested page")
	}

	c.logger.DebugHTML("page content", html)
	return html, nil
}

// isLoginRedirect reports whether resp is, or was redirected to, the
// public login page. PowerSchool answers an expired session with a 302 to
// /public/..., which an http.Client follows to a 200.
func isLoginRedirect(resp *http.Response) bool {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return true
	}
	if resp.Request != nil && resp.Request.URL != nil {
		return isPublicPath(resp.Request.URL.Path)
	}
	return false
}

// isPublicPath reports whether path is under the portal's signed-out area.
func isPublicPath(path string) bool {
	return strings.HasPrefix(path, "/public/")
}

// loginPageSelector matches the sign-in form's own fields: the form the
// browser login drives (internal/browser) and its password input.
const loginPageSelector = `form#LoginForm, input#fieldPassword, input[type="password"][name="pw"]`

// loginPageTitle is the heading of the reference instance's sign-in page.
const loginPageTitle = "Parent, Guardian, and Student Login"

// isLoginPage reports whether html is the portal's sign-in page rather
// than a signed-in page. It looks for the sign-in form's fields (or the
// sign-in heading) and requires that the page has no student switcher,
// which every signed-in guardian page carries and the sign-in page never
// does.
func isLoginPage(html string) bool {
	if strings.Contains(html, "switchStudent(") {
		return false
	}
	if strings.Contains(html, loginPageTitle) {
		return true
	}
	// Cheap pre-check before parsing a large page.
	lower := strings.ToLower(html)
	if !strings.Contains(lower, "loginform") && !strings.Contains(lower, "password") {
		return false
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return false
	}
	return doc.Find(loginPageSelector).Length() > 0
}

// pathOnly strips the query and fragment from a URL or server-relative
// path. Queries on this portal carry enrolment and school identifiers
// (frn, schoolid) and must not reach an error string or a log line.
func pathOnly(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

// redactURLError rewrites a *url.Error (what http.Client.Do returns) so its
// URL carries no query string, keeping the wrapped cause intact for
// errors.Is / errors.As.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: pathOnly(ue.URL), Err: ue.Err}
	}
	return err
}
