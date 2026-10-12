package browser

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/network"
)

// The tests in this file need no browser.

func TestValidateCredentials(t *testing.T) {
	bad := map[string]string{
		"trailing newline":       "user-example\n",
		"trailing CRLF":          "user-example\r\n",
		"carriage return":        "user\rexample",
		"leading newline":        "\nuser-example",
		"tab":                    "user\texample",
		"NUL":                    "user\x00example",
		"escape":                 "user\x1bexample",
		"backspace":              "user\bexample",
		"delete":                 "user\x7fexample",
		"C1 control":             "user\u0085example",
		"automation key rune":    "user\u0301example",
		"zero-width space":       "user\u200bexample",
		"line separator":         "user\u2028example",
		"byte order mark at end": "user-example\ufeff",
	}
	for name, value := range bad {
		for _, field := range []string{"username", "password"} {
			for _, mode := range []string{"local", "remote"} {
				o := remoteOpts("", time.Second)
				if mode == "remote" {
					o.BrowserURL = "ws://browser.example.internal:3000?token=" + fakeToken
				}
				if field == "username" {
					o.Username = value
				} else {
					o.Password = value
				}
				err := validateCredentials(o)
				if err == nil {
					t.Errorf("%s in the %s (%s): accepted", name, field, mode)
					continue
				}
				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("%s in the %s: not ErrInvalidOptions: %v", name, field, err)
				}
				assertNoStage(t, err)
				if !strings.Contains(err.Error(), "the "+field) {
					t.Errorf("%s: error does not name the %s: %v", name, field, err)
				}
				// Neither the value nor any part of it is repeated.
				assertNoSecret(t, err, value, "user-example", "example", fakeToken)
				if got := strings.HasPrefix(err.Error(), "remote "); got != (mode == "remote") {
					t.Errorf("%s (%s): wrong prefix: %v", name, mode, err)
				}
			}
		}
	}

	good := []string{
		"", "user-example", "User Example", "p@ss w0rd!#$%^&*()_+-=[]{}|;':\",./<>?`~",
		"p\u00e4ssw\u00f6rd", "\u00e9l\u00e8ve", "\u5bc6\u7801", "emoji-free but astral: \U0001d11e",
	}
	for _, value := range good {
		o := remoteOpts("", time.Second)
		o.Username, o.Password = value, value
		if err := validateCredentials(o); err != nil {
			t.Errorf("%q rejected: %v", value, err)
		}
	}
}

// A newline in a credential is refused before any browser work: nothing is
// dialled in remote mode, and nothing is started in local mode (this test
// passes on a machine with no browser installed, and returns at once).
func TestAuthenticate_NewlineInCredential_NoBrowserWork(t *testing.T) {
	for _, field := range []string{"username", "password"} {
		t.Run("remote/"+field, func(t *testing.T) {
			fake := newFakeCDP(t)
			o := remoteOpts(fake.url(), 5*time.Second)
			if field == "username" {
				o.Username += "\n"
			} else {
				o.Password += "\n"
			}
			res, err := Authenticate(context.Background(), o)
			if res != nil || !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Authenticate = %v, %v; want ErrInvalidOptions", res, err)
			}
			assertNoStage(t, err)
			assertNoSecret(t, err, fakeToken, testPassword, testUsername)
			if n := len(fake.connections()); n != 0 {
				t.Errorf("%d connections to the browser, want 0", n)
			}
		})
		t.Run("local/"+field, func(t *testing.T) {
			o := remoteOpts("", 30*time.Second)
			if field == "username" {
				o.Username += "\n"
			} else {
				o.Password += "\n"
			}
			start := time.Now()
			res, err := Authenticate(context.Background(), o)
			if res != nil || !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Authenticate = %v, %v; want ErrInvalidOptions", res, err)
			}
			assertNoStage(t, err)
			assertNoSecret(t, err, testPassword, testUsername)
			assertWithin(t, start, 500*time.Millisecond)
		})
	}
}

func TestCanonicalOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://portal.example.org":                   "https://portal.example.org",
		"https://portal.example.org/":                  "https://portal.example.org",
		"https://Portal.Example.ORG:443/public/?a=b#c": "https://portal.example.org",
		"HTTPS://portal.example.org:8443/x":            "https://portal.example.org:8443",
		"http://portal.example.org:80":                 "http://portal.example.org",
		"http://portal.example.org:443":                "http://portal.example.org:443",
		"http://127.0.0.1:8080/public/":                "http://127.0.0.1:8080",
		"http://[::1]:8080/":                           "http://[::1]:8080",
		"https://user:secret@portal.example.org/":      "https://portal.example.org",
		"null":                          "",
		"":                              "",
		"about:blank":                   "",
		"chrome-error://chromewebdata/": "",
		"data:text/html,hello":          "",
		"file:///etc/passwd":            "",
		"ws://portal.example.org":       "",
		"https://":                      "",
		"https://portal.example.org\x00.evil.example":     "",
		"https://portal example.org":                      "",
		"https://p\u00f6rtal.example.org":                 "",
		"https://portal.example.org:65x/":                 "",
		"javascript:alert(1)":                             "",
		"https://portal.example.org.evil.example/public/": "https://portal.example.org.evil.example",
	} {
		if got := canonicalOrigin(in); got != want {
			t.Errorf("canonicalOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckOrigin(t *testing.T) {
	const want = "https://portal.example.org"
	for _, reported := range []string{"https://portal.example.org", "https://PORTAL.example.org:443"} {
		if err := checkOrigin(reported, want); err != nil {
			t.Errorf("checkOrigin(%q) = %v, want nil", reported, err)
		}
	}
	for reported, shown := range map[string]string{
		"http://portal.example.org":                          "http://portal.example.org",
		"https://portal.example.org:8443":                    "https://portal.example.org:8443",
		"https://www.portal.example.org":                     "https://www.portal.example.org",
		"https://portal.example.org.evil.example":            "https://portal.example.org.evil.example",
		"https://evil.example/login?next=query-secret-value": "https://evil.example",
		"null": unreadableOrigin,
		"":     unreadableOrigin,
		"https://evil.example/<script>pw</script>": "https://evil.example",
		"pw-example-not-a-real-one":                unreadableOrigin,
	} {
		err := checkOrigin(reported, want)
		var om *originMismatch
		if !errors.As(err, &om) {
			t.Errorf("checkOrigin(%q) = %v, want a mismatch", reported, err)
			continue
		}
		if om.got != shown {
			t.Errorf("checkOrigin(%q) shows %q, want %q", reported, om.got, shown)
		}
		for _, leak := range []string{"query-secret-value", "/login", "<script>", testPassword} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("checkOrigin(%q) error repeats %q: %v", reported, leak, err)
			}
		}
	}
}

func TestJudgeOutcome(t *testing.T) {
	const want = "https://portal.example.org"
	before := pageState{Origin: want, Path: "/public/", HasPassword: true, Ready: "complete", Document: 1000}
	home := pageState{Origin: want, Path: "/guardian/home.html", Ready: "complete", Document: 2000}

	with := func(base pageState, f func(*pageState)) pageState {
		f(&base)
		return base
	}
	cases := []struct {
		name   string
		now    pageState
		before pageState
		want   signInOutcome
	}{
		{"signed-in page", home, before, outcomeAccepted},
		{"signed-in page still loading subresources", with(home, func(s *pageState) { s.Ready = "interactive" }), before, outcomeAccepted},
		// The reviewer's case: the portal has not answered yet. The page
		// is still the sign-in form; after the old fixed wait this was
		// reported as a success.
		{"portal has not answered yet", before, before, outcomeUnknown},
		{"new document but still parsing", with(home, func(s *pageState) { s.Ready = "loading" }), before, outcomeUnknown},
		{"sign-in form again, no banner", with(before, func(s *pageState) { s.Document = 2000; s.Path = "/guardian/home.html" }), before, outcomeUnknown},
		{"same path, no password field", with(home, func(s *pageState) { s.Path = "/public/" }), before, outcomeUnknown},
		{"same document, password field removed by script", with(before, func(s *pageState) { s.HasPassword = false; s.Path = "/guardian/home.html" }), before, outcomeUnknown},
		{"document id unreadable", with(home, func(s *pageState) { s.Document = 0 }), before, outcomeUnknown},
		{"signed-in looking page on another origin", with(home, func(s *pageState) { s.Origin = "https://evil.example" }), before, outcomeUnknown},
		{"opaque origin", with(home, func(s *pageState) { s.Origin = "null" }), before, outcomeUnknown},
		{"zero value (failed read)", pageState{}, before, outcomeUnknown},

		{"banner on the answer page", with(before, func(s *pageState) { s.Document = 2000; s.Rejected = true; s.Path = "/guardian/home.html" }), before, outcomeRejected},
		{"banner on the answer page, same path", with(before, func(s *pageState) { s.Document = 2000; s.Rejected = true }), before, outcomeRejected},
		{"banner shown in place", with(before, func(s *pageState) { s.Rejected = true }), before, outcomeRejected},
		{"banner wins over a page without a password field", with(home, func(s *pageState) { s.Rejected = true }), before, outcomeRejected},
		{"banner on another origin proves nothing", with(home, func(s *pageState) { s.Rejected = true; s.Origin = "https://evil.example" }), before, outcomeUnknown},
		// A notice that was on the sign-in page before the submission
		// (a timed-out session, say) is not the portal's answer.
		{"stale banner, same document", with(before, func(s *pageState) { s.Rejected = true }), with(before, func(s *pageState) { s.Rejected = true }), outcomeUnknown},
		{"stale banner, then a real rejection", with(before, func(s *pageState) { s.Rejected = true; s.Document = 2000 }), with(before, func(s *pageState) { s.Rejected = true }), outcomeRejected},
		{"stale banner, then signed in", home, with(before, func(s *pageState) { s.Rejected = true }), outcomeAccepted},
	}
	for _, tc := range cases {
		if got := judgeOutcome(tc.now, tc.before, want); got != tc.want {
			t.Errorf("%s: outcome %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestAlreadySubmitted(t *testing.T) {
	before := pageState{Document: 1000}
	if alreadySubmitted(pageState{Document: 1000}, nil, before) {
		t.Errorf("an untouched page counts as submitted")
	}
	for name, tc := range map[string]struct {
		now pageState
		err error
	}{
		"the page submitted itself":   {pageState{Document: 1000, SelfSubmitted: true}, nil},
		"the page was replaced":       {pageState{Document: 2000}, nil},
		"the page cannot be read now": {pageState{}, errors.New("execution context was destroyed")},
	} {
		if !alreadySubmitted(tc.now, tc.err, before) {
			t.Errorf("%s: not treated as already submitted; the click would be a second submission", name)
		}
	}
}

func TestCookieDomainMatches(t *testing.T) {
	for _, tc := range []struct {
		host, domain string
		want         bool
	}{
		{"portal.example.org", "portal.example.org", true},
		{"portal.example.org", ".portal.example.org", true},
		{"portal.example.org", ".example.org", true},
		{"portal.example.org", "example.org", true},
		{"portal.example.org", "PORTAL.example.org", true},
		{"portal.example.org", "other.example.org", false},
		{"portal.example.org", "evil-example.org", false},
		{"portal.example.org", "l.example.org", false},
		{"portal.example.org", "sub.portal.example.org", false},
		{"portal.example.org", "", false},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1", "0.0.1", false},
		{"127.0.0.1", "localhost", false},
		{"localhost", "127.0.0.1", false},
	} {
		if got := cookieDomainMatches(tc.host, tc.domain); got != tc.want {
			t.Errorf("cookieDomainMatches(%q, %q) = %v, want %v", tc.host, tc.domain, got, tc.want)
		}
	}
}

func TestFilterCookies(t *testing.T) {
	all := []*network.Cookie{
		{Name: "session", Domain: "portal.example.org", Path: "/"},
		{Name: "gateway", Domain: ".example.org", Path: "/"},
		{Name: "scoped", Domain: "portal.example.org", Path: "/guardian"},
		{Name: "secure", Domain: "portal.example.org", Secure: true},
		{Name: "tracker", Domain: ".ads.example.net", Path: "/"},
		{Name: "sibling", Domain: "other.example.org", Path: "/"},
		{Name: "lookalike", Domain: "portal.example.org.evil.example", Path: "/"},
		nil,
	}
	names := func(cs []*network.Cookie) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(filterCookies(all, "https://portal.example.org")); got != "session,gateway,scoped,secure" {
		t.Errorf("https portal: kept %q", got)
	}
	if got := names(filterCookies(all, "http://portal.example.org:8080")); got != "session,gateway,scoped" {
		t.Errorf("http portal: kept %q (a Secure cookie is not sent over http)", got)
	}
	if got := filterCookies(all, "://bad"); len(got) != 0 {
		t.Errorf("unparseable base URL: kept %d cookies, want 0", len(got))
	}
}

func TestCookieURLs(t *testing.T) {
	const base, origin = "https://portal.example.org", "https://portal.example.org"
	got := cookieURLs(base, "https://portal.example.org/guardian/home.html#top", origin)
	want := []string{base + "/", base + "/public/", "https://portal.example.org/guardian/home.html"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("cookieURLs = %v, want %v", got, want)
	}
	// A page on another origin contributes nothing.
	for _, href := range []string{"https://evil.example/home", "", "about:blank"} {
		if got := cookieURLs(base, href, origin); len(got) != 2 {
			t.Errorf("cookieURLs with %q = %v, want only the portal's own URLs", href, got)
		}
	}
}

func TestNeedsWindow(t *testing.T) {
	if !needsWindow(&cdproto.Error{Code: -32000, Message: "Failed to open new tab - no browser is open"}) {
		t.Errorf("the browser's refusal of a tab in a windowless context is not recognised")
	}
	for _, err := range []error{
		nil,
		errors.New("Failed to open new tab - no browser is open"),
		&cdproto.Error{Code: -32000, Message: "Failed to find browser context with id X"},
		&cdproto.Error{Code: -32602, Message: "Invalid parameters"},
		&cdproto.Error{Code: -32601, Message: "no browser is open"},
		context.DeadlineExceeded,
	} {
		if needsWindow(err) {
			t.Errorf("needsWindow(%v) = true, want false", err)
		}
	}
}

// Any other refusal of the page is not retried, and what the browser wrote
// in its refusal is not repeated: only the numeric code is.
func TestRemoteFake_OtherRefusal_NotRetried_MessageNotRepeated(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Target.createTarget", "refuse-echo")

	_, err := Authenticate(context.Background(), remoteOpts(fake.url(), 5*time.Second))
	if err == nil {
		t.Fatalf("expected an error")
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, testPassword, fakeToken, "Invalid parameters")
	if !strings.Contains(err.Error(), "-32602") {
		t.Errorf("error does not carry the protocol error code: %v", err)
	}

	fake.waitClosed(t)
	calls := fake.snapshot()
	if n := countCalls(calls, "Target.createTarget"); n != 1 {
		t.Errorf("%d attempts to create the page, want 1 (no retry)", n)
	}
	if indexOfCall(calls, "Target.disposeBrowserContext") < 0 {
		t.Errorf("the browser context was not disposed; calls: %v", methodsOf(calls))
	}
	assertNothingElseTouched(t, calls)
}

func TestSignInError_Text(t *testing.T) {
	for _, tc := range []struct {
		err  *SignInError
		want string
	}{
		{&SignInError{Stage: StageOptions, local: true, msg: "m"}, "browser sign-in: m (nothing was sent)"},
		{&SignInError{Stage: StageOptions, msg: "m"}, "remote browser sign-in: m (nothing was sent)"},
		{&SignInError{Stage: StageBeforeSubmit, msg: "m"}, "remote browser sign-in: m (credentials were not submitted)"},
		{&SignInError{Stage: StageAfterSubmit, msg: "m"}, "remote browser sign-in: m (credentials may have been submitted; outcome unknown)"},
		{&SignInError{Stage: StageAfterSubmit, Rejected: true, msg: "m"}, "remote browser sign-in: m (credentials were submitted once)"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}
