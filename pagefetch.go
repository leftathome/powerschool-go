package powerschool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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
// started outside Authenticate: the calls that can only work through a
// browser (SwitchStudent, GetProgressReports) return an error instead.
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

	c.logger.Debug("Fetching page over HTTP: %s", c.baseURL+path)

	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if isLoginRedirect(resp) {
		c.session = nil
		return "", ErrSessionExpired
	}
	if resp.StatusCode != http.StatusOK {
		return "", WrapNetworkError(fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return "", WrapNetworkError(err)
	}

	html := string(body)
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
		return strings.HasPrefix(resp.Request.URL.Path, "/public/")
	}
	return false
}
