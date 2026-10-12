package browser

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// This file holds the checks both sign-in flows (local and remote) share:
// what may be typed, where it may be typed, which cookies come back, and
// what counts as having signed in.

// validateCredentials rejects a username or password that cannot be typed
// as plain text. The fields are filled by key events: a newline is the
// Enter key and submits the form in the middle of filling it, a tab moves
// the focus, and some code points are how the automation spells keys such
// as the arrows. A credential read from a file with a trailing newline is
// the usual way to get one, so nothing is trimmed silently: the caller is
// told, before any browser work. The error never contains the value.
func validateCredentials(opts *AuthenticateOptions) error {
	check := func(name, value string) error {
		for _, r := range value {
			var what string
			switch {
			case r == '\n' || r == '\r':
				what = "a line break (trim the value: a trailing newline is common when it is read from a file)"
			case r == '\t':
				what = "a tab"
			case unicode.IsControl(r):
				what = "a control character"
			case !unicode.IsPrint(r):
				what = "a character that cannot be typed"
			default:
				if k, ok := kb.Keys[r]; ok && !k.Print {
					what = "a character the browser automation types as a key, not as text"
				}
			}
			if what != "" {
				return &SignInError{
					Stage: StageOptions,
					local: opts.BrowserURL == "",
					msg:   "the " + name + " contains " + what,
				}
			}
		}
		return nil
	}
	if err := check("username", opts.Username); err != nil {
		return err
	}
	return check("password", opts.Password)
}

// unreadableOrigin stands in for an origin that is opaque ("null") or not
// in a form safe to repeat.
const unreadableOrigin = "an opaque or unreadable origin"

// canonicalOrigin reduces a URL or an origin string to scheme://host[:port]
// in the form a browser reports as location.origin: lower case, the
// default port left out. It returns "" when raw has no such origin
// (about:blank, "null", data:, browser error pages) or when its host is
// not plain ASCII: an internationalised host has to be given in its ASCII
// ("xn--") form, which is the form a browser reports.
func canonicalOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ""
	}
	for _, r := range host {
		ok := r == '.' || r == '-' || r == ':' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok {
			return ""
		}
	}
	if len(host) > 253 {
		return ""
	}
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		for _, r := range port {
			if r < '0' || r > '9' {
				return ""
			}
		}
		return scheme + "://" + host + ":" + port
	}
	return scheme + "://" + host
}

// pageState is what the flows read from the page, in one evaluation.
type pageState struct {
	Origin      string  `json:"origin"`
	Href        string  `json:"href"`
	Path        string  `json:"path"`
	HasPassword bool    `json:"hasPassword"`
	Rejected    bool    `json:"rejected"`
	Ready       string  `json:"ready"`
	Document    float64 `json:"document"`
	// SelfSubmitted: the page started a submission or a navigation of its
	// own after watchSubmissionJS was installed in it.
	SelfSubmitted bool `json:"selfSubmitted"`
}

// pageStateJS reads pageState. document is performance.timeOrigin, which
// differs for every document: it tells a page that was navigated to from
// one that is still the same.
const pageStateJS = `(() => ({
  origin: String(location.origin),
  href: String(location.href),
  path: String(location.pathname),
  hasPassword: document.querySelector('input[type="password"]') !== null,
  rejected: document.querySelector('.feedback-alert') !== null,
  ready: String(document.readyState),
  document: Number(performance.timeOrigin) || 0,
  selfSubmitted: window.__psgoSignInLeft === true
}))()`

// watchSubmissionJS makes the page remember that a form was submitted or
// that it started to navigate away. It is installed before the password is
// typed, so that a page which submits itself while being filled is not
// submitted a second time by the click.
const watchSubmissionJS = `(() => {
  if (window.__psgoSignInWatch === true) { return true; }
  window.__psgoSignInWatch = true;
  const left = () => { window.__psgoSignInLeft = true; };
  window.addEventListener('submit', left, true);
  window.addEventListener('beforeunload', left, true);
  window.addEventListener('pagehide', left, true);
  return true;
})()`

// alreadySubmitted reports whether the page that was being filled has
// submitted itself or been replaced since before was read. A page that
// cannot be read at this point counts: a navigation in flight is the usual
// reason, and the cost of being wrong is a second submission.
func alreadySubmitted(now pageState, readErr error, before pageState) bool {
	return readErr != nil || now.SelfSubmitted || now.Document != before.Document
}

func readPageState(ctx context.Context) (pageState, error) {
	var st pageState
	err := chromedp.Evaluate(pageStateJS, &st).Do(ctx)
	return st, err
}

// originMismatch is returned by requirePortalOrigin. got is already reduced
// to an origin (or to unreadableOrigin): never a path or a query.
type originMismatch struct {
	got, want string
}

func (e *originMismatch) Error() string {
	return "the page to sign in on is at " + e.got + ", not at the portal's origin " + e.want
}

// checkOrigin compares the origin a page reports with the portal's.
func checkOrigin(reported, want string) error {
	got := canonicalOrigin(reported)
	if got != "" && got == want {
		return nil
	}
	if got == "" {
		got = unreadableOrigin
	}
	return &originMismatch{got: got, want: want}
}

// requirePortalOrigin fails unless the page the credentials are about to be
// typed into, or submitted from, is on the portal's own origin. A sign-in
// URL that redirects elsewhere, to a page that happens to show a text and a
// password field, must not be handed the credentials.
func requirePortalOrigin(ctx context.Context, want string) error {
	st, err := readPageState(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &originMismatch{got: unreadableOrigin, want: want}
	}
	return checkOrigin(st.Origin, want)
}

// signInOutcome is what the page says about a submitted sign-in.
type signInOutcome int

const (
	// outcomeUnknown: nothing conclusive yet. Never a success.
	outcomeUnknown signInOutcome = iota
	// outcomeAccepted: the browser has left the sign-in form.
	outcomeAccepted
	// outcomeRejected: the portal shows its sign-in error.
	outcomeRejected
)

// judgeOutcome decides what a page read after the submission says, given
// the page as it was read immediately before it.
//
// Accepted needs positive evidence, all of: the page is on the portal's
// origin; it is a different document from the one that was submitted; its
// path is not the sign-in page's; it has finished at least parsing; and it
// has no password field. Rejected is the portal's error banner on its own
// origin, in a new document or newly shown in the same one (a banner that
// was already there before the submission says nothing about it).
// Everything else is unknown.
func judgeOutcome(now, before pageState, want string) signInOutcome {
	if checkOrigin(now.Origin, want) != nil {
		return outcomeUnknown
	}
	newDocument := now.Document != 0 && now.Document != before.Document
	if now.Rejected && (newDocument || !before.Rejected) {
		return outcomeRejected
	}
	if newDocument && now.Path != before.Path && !now.HasPassword &&
		(now.Ready == "interactive" || now.Ready == "complete") {
		return outcomeAccepted
	}
	return outcomeUnknown
}

// OutcomeEvidence is what was last seen of the page when a submitted
// sign-in ended without a conclusive answer. It is there to tell, from one
// failed attempt, why a sign-in that may well have worked was not
// recognised as one. It holds only yes/no observations and a duration:
// never a URL, a path, a title, page text or anything about cookies.
type OutcomeEvidence struct {
	// PageRead: the page could be read at least once after the
	// submission. When false, the other observations are all false and
	// mean nothing.
	PageRead bool
	// NewDocument: a different document from the one that was filled in.
	NewDocument bool
	// PortalOrigin: the page is on the portal's own origin.
	PortalOrigin bool
	// PathChanged: the page's path differs from the sign-in page's.
	PathChanged bool
	// PasswordField: the page has a password field.
	PasswordField bool
	// ErrorBanner: the page shows the portal's sign-in error banner.
	ErrorBanner bool
	// Elapsed: how long the answer was waited for.
	Elapsed time.Duration
}

// String renders the evidence for an error message.
func (e OutcomeEvidence) String() string {
	waited := "after waiting " + e.Elapsed.Round(100*time.Millisecond).String()
	if !e.PageRead {
		return "the page could not be read " + waited
	}
	return fmt.Sprintf("page last seen %s: new document=%t, portal origin=%t, path changed=%t, password field=%t, error banner=%t",
		waited, e.NewDocument, e.PortalOrigin, e.PathChanged, e.PasswordField, e.ErrorBanner)
}

// evidenceFrom reduces the last page read after a submission to yes/no
// observations. last is nil when no read succeeded.
func evidenceFrom(last *pageState, before pageState, want string, elapsed time.Duration) *OutcomeEvidence {
	ev := &OutcomeEvidence{Elapsed: elapsed}
	if last == nil {
		return ev
	}
	ev.PageRead = true
	ev.NewDocument = last.Document != 0 && last.Document != before.Document
	ev.PortalOrigin = checkOrigin(last.Origin, want) == nil
	ev.PathChanged = last.Path != before.Path
	ev.PasswordField = last.HasPassword
	ev.ErrorBanner = last.Rejected
	return ev
}

// cookieURLs are the URLs whose cookies make up the session: the portal's
// base, its sign-in page, and the page the sign-in ended on when that is on
// the portal's origin.
func cookieURLs(baseURL, currentHref, wantOrigin string) []string {
	urls := []string{baseURL + "/", baseURL + "/public/"}
	if currentHref != "" && canonicalOrigin(currentHref) == wantOrigin {
		if i := strings.IndexByte(currentHref, '#'); i >= 0 {
			currentHref = currentHref[:i]
		}
		urls = append(urls, currentHref)
	}
	return urls
}

// filterCookies keeps the cookies a browser would send to the portal's
// host: the cookie's domain is the host or a parent domain of it, and a
// Secure cookie only when the portal is on https. Cookies of other sites
// the page embeds or passed through are dropped: the client attaches every
// cookie it holds to its requests to the portal.
func filterCookies(cookies []*network.Cookie, baseURL string) []*network.Cookie {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	secure := strings.EqualFold(u.Scheme, "https")
	var out []*network.Cookie
	for _, c := range cookies {
		if c == nil || !cookieDomainMatches(host, c.Domain) {
			continue
		}
		if c.Secure && !secure {
			continue
		}
		out = append(out, c)
	}
	return out
}

// cookieDomainMatches reports whether a cookie with the given Domain
// attribute would be sent to host.
func cookieDomainMatches(host, domain string) bool {
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	if host == "" || domain == "" {
		return false
	}
	if host == domain {
		return true
	}
	// A parent domain matches only a host name, never an IP address.
	if net.ParseIP(host) != nil {
		return false
	}
	return strings.HasSuffix(host, "."+domain)
}

// readPortalCookies reads the cookies of the sign-in from the page of ctx,
// restricted to those that belong to the portal.
func readPortalCookies(ctx context.Context, baseURL, wantOrigin string) ([]*network.Cookie, error) {
	href := ""
	if st, err := readPageState(ctx); err == nil {
		href = st.Href
	}
	got, err := network.GetCookies().WithURLs(cookieURLs(baseURL, href, wantOrigin)).Do(ctx)
	if err != nil {
		return nil, err
	}
	return filterCookies(got, baseURL), nil
}
