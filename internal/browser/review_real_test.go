package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leftathome/powerschool-go/internal/cdptest"
)

// Regression tests for defects found in review of the remote sign-in, each
// reproduced against a real headless Chrome. They are skipped when no
// browser is installed; guard_test.go holds the parts of the same checks
// that need none.

const reviewForm = `<!doctype html><html><head><title>Sign In</title></head><body>
%ALERT%
<form id="LoginForm" action="/guardian/home.html" method="post">
  <input type="text" id="fieldAccount" name="account">
  <input type="password" id="fieldPassword" name="pw">
  <button type="submit" id="btn-enter-sign-in">Sign In</button>
</form>
%SCRIPT%
</body></html>`

// autosubmitScript sends the credentials by itself once three characters
// are in the password field, through a form of its own: a page that does
// not wait for the click, and that the click would submit a second time.
const autosubmitScript = `<script>
document.getElementById('fieldPassword').addEventListener('input', function (e) {
  if (e.target.value.length !== 3 || window.sent) { return; }
  window.sent = true;
  var f = document.createElement('form');
  f.method = 'post'; f.action = '/guardian/home.html';
  ['account', 'pw'].forEach(function (n) {
    var i = document.createElement('input'); i.type = 'hidden'; i.name = n;
    i.value = document.getElementsByName(n)[0].value; f.appendChild(i);
  });
  document.body.appendChild(f); f.submit();
});
</script>`

type reviewPortal struct {
	srv *httptest.Server

	mu          sync.Mutex
	posts       []string
	postDelay   time.Duration
	wantUser    string
	wantPw      string
	noBanner    bool   // answer a wrong password with the form, without the banner
	autosubmit  bool   // the sign-in page submits itself while being filled
	homeEmbeds  string // URL the signed-in page embeds in an iframe
	release     chan struct{}
	releaseOnce sync.Once
}

func newReviewPortal(t *testing.T) *reviewPortal {
	t.Helper()
	p := &reviewPortal{wantUser: testUsername, wantPw: testPassword, release: make(chan struct{})}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(func() {
		p.releaseOnce.Do(func() { close(p.release) })
		p.srv.CloseClientConnections()
		p.srv.Close()
	})
	return p
}

func (p *reviewPortal) form(alert string) string {
	script := ""
	if p.autosubmit {
		script = autosubmitScript
	}
	return strings.NewReplacer("%ALERT%", alert, "%SCRIPT%", script).Replace(reviewForm)
}

func (p *reviewPortal) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/public/":
		http.SetCookie(w, &http.Cookie{Name: "pre", Value: "prelogin", Path: "/"})
		fmt.Fprint(w, p.form(""))
	case r.Method == http.MethodPost:
		_ = r.ParseForm()
		account, pw := r.PostForm.Get("account"), r.PostForm.Get("pw")
		p.mu.Lock()
		p.posts = append(p.posts, fmt.Sprintf("account=%q pw=%q", account, pw))
		delay := p.postDelay
		p.mu.Unlock()
		select {
		case <-time.After(delay):
		case <-p.release:
		}
		if account == p.wantUser && pw == p.wantPw {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "good", Path: "/"})
			http.Redirect(w, r, "/guardian/home.html", http.StatusSeeOther)
			return
		}
		if p.noBanner {
			fmt.Fprint(w, p.form(""))
			return
		}
		fmt.Fprint(w, p.form(`<div class="feedback-alert">Invalid Username or Password!</div>`))
	case r.Method == http.MethodGet && r.URL.Path == "/guardian/home.html":
		embed := ""
		if p.homeEmbeds != "" {
			embed = `<iframe src="` + p.homeEmbeds + `"></iframe>`
		}
		fmt.Fprint(w, `<!doctype html><html><head><title>Home</title></head><body>signed in`+embed+`</body></html>`)
	default:
		http.NotFound(w, r)
	}
}

func (p *reviewPortal) postList() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.posts...)
}

func reviewOpts(p *reviewPortal, ws string, timeout time.Duration) *AuthenticateOptions {
	o := DefaultAuthenticateOptions()
	o.BaseURL = p.srv.URL
	o.Username = testUsername
	o.Password = testPassword
	o.Timeout = timeout
	o.BrowserURL = ws
	return o
}

func cookieNames(res *AuthResult) map[string]string {
	out := map[string]string{}
	for _, c := range res.Cookies {
		out[c.Name] = c.Domain
	}
	return out
}

// foreignSite is another origin (the same listener family, a different host
// name) that shows a sign-in form of its own, sets cookies, and records
// what is posted to it.
type foreignSite struct {
	srv *httptest.Server
	url string // with "localhost" as its host

	mu  sync.Mutex
	got []string
}

func newForeignSite(t *testing.T) *foreignSite {
	t.Helper()
	f := &foreignSite{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.SetCookie(w, &http.Cookie{Name: "foreign", Value: "from-another-site", Path: "/"})
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			f.mu.Lock()
			f.got = append(f.got, fmt.Sprintf("account=%q", r.PostForm.Get("account")))
			f.mu.Unlock()
			fmt.Fprint(w, "<html><body>thanks</body></html>")
			return
		}
		fmt.Fprint(w, strings.NewReplacer("%ALERT%", "", "%SCRIPT%", "").Replace(reviewForm))
	}))
	t.Cleanup(f.srv.Close)
	f.url = strings.Replace(f.srv.URL, "127.0.0.1", "localhost", 1)
	return f
}

func (f *foreignSite) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

func TestReviewReal_RemoteSignIn(t *testing.T) {
	ws, _ := cdptest.StartChrome(t)

	// 1. A trailing newline in the username was typed as the Enter key:
	// two submissions in one call.
	t.Run("newline in the username is refused before any submission", func(t *testing.T) {
		p := newReviewPortal(t)
		o := reviewOpts(p, ws, 40*time.Second)
		o.Username = testUsername + "\n"
		res, err := Authenticate(context.Background(), o)
		if res != nil || !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("Authenticate = %v, %v; want ErrInvalidOptions", res, err)
		}
		assertNoStage(t, err)
		if posts := p.postList(); len(posts) != 0 {
			t.Errorf("%d submissions, want 0: %v", len(posts), posts)
		}
	})

	// 1b. ... and with a portal that stalls, one submission was delivered
	// and the call said "not submitted".
	t.Run("newline in the username with a stalling portal", func(t *testing.T) {
		p := newReviewPortal(t)
		p.postDelay = 30 * time.Second
		o := reviewOpts(p, ws, 12*time.Second)
		o.Username = testUsername + "\n"
		_, err := Authenticate(context.Background(), o)
		if !errors.Is(err, ErrInvalidOptions) || errors.Is(err, ErrNotSubmitted) {
			t.Fatalf("error = %v; want ErrInvalidOptions and not ErrNotSubmitted", err)
		}
		if posts := p.postList(); len(posts) != 0 {
			t.Errorf("%d submissions, want 0", len(posts))
		}
	})

	// 1c. The same hazard without a newline: a page that submits itself
	// while the password is typed. It must be one submission, and never
	// "not submitted".
	t.Run("page that submits itself while being filled, portal stalls", func(t *testing.T) {
		p := newReviewPortal(t)
		p.autosubmit = true
		p.postDelay = 30 * time.Second
		_, err := Authenticate(context.Background(), reviewOpts(p, ws, 12*time.Second))
		if err == nil {
			t.Fatalf("success reported for a sign-in the portal never answered")
		}
		assertStage(t, err, true)
		if posts := p.postList(); len(posts) != 1 {
			t.Errorf("%d submissions, want exactly 1: the click must not submit a second time", len(posts))
		}
	})

	t.Run("page that submits itself while being filled, portal rejects", func(t *testing.T) {
		p := newReviewPortal(t)
		p.autosubmit = true
		_, err := Authenticate(context.Background(), reviewOpts(p, ws, 40*time.Second))
		// Three characters of the password were submitted: a rejection.
		if !errors.Is(err, ErrRejected) {
			t.Fatalf("error = %v; want ErrRejected", err)
		}
		assertStage(t, err, true)
		if posts := p.postList(); len(posts) != 1 {
			t.Errorf("%d submissions, want exactly 1", len(posts))
		}
	})

	// 4. A wrong password answered after 6 s was reported as success,
	// with only the pre-login cookie.
	t.Run("slow rejection is a rejection", func(t *testing.T) {
		p := newReviewPortal(t)
		p.postDelay = 6 * time.Second
		o := reviewOpts(p, ws, 40*time.Second)
		o.Password = "a-wrong-password-example"
		res, err := Authenticate(context.Background(), o)
		if res != nil || !errors.Is(err, ErrRejected) {
			t.Fatalf("Authenticate = %v, %v; want ErrRejected", res, err)
		}
		assertStage(t, err, true)
		if posts := p.postList(); len(posts) != 1 {
			t.Errorf("%d submissions, want exactly 1", len(posts))
		}
	})

	t.Run("slow acceptance is waited for", func(t *testing.T) {
		p := newReviewPortal(t)
		p.postDelay = 6 * time.Second
		res, err := Authenticate(context.Background(), reviewOpts(p, ws, 40*time.Second))
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if _, ok := cookieNames(res)["session"]; !ok {
			t.Errorf("success without the session cookie: %v", cookieNames(res))
		}
		if posts := p.postList(); len(posts) != 1 {
			t.Errorf("%d submissions, want exactly 1", len(posts))
		}
	})

	t.Run("no answer in time is outcome unknown, never success", func(t *testing.T) {
		p := newReviewPortal(t)
		p.postDelay = 60 * time.Second
		const timeout = 12 * time.Second
		start := time.Now()
		res, err := Authenticate(context.Background(), reviewOpts(p, ws, timeout))
		assertWithin(t, start, timeout+slack)
		if res != nil || err == nil {
			t.Fatalf("Authenticate = %v, %v; want an error", res, err)
		}
		if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRejected) {
			t.Errorf("error = %v; want a timeout that is not a rejection", err)
		}
		assertStage(t, err, true)
		if !strings.Contains(err.Error(), "outcome unknown") {
			t.Errorf("error does not say the outcome is unknown: %v", err)
		}
	})

	t.Run("sign-in form again without the banner is outcome unknown", func(t *testing.T) {
		p := newReviewPortal(t)
		p.noBanner = true
		o := reviewOpts(p, ws, 12*time.Second)
		o.Password = "a-wrong-password-example"
		res, err := Authenticate(context.Background(), o)
		if res != nil || err == nil {
			t.Fatalf("Authenticate = %v, %v; want an error: the browser is still on a sign-in form", res, err)
		}
		assertStage(t, err, true)
		if errors.Is(err, ErrRejected) {
			t.Errorf("a page without the banner was reported as a rejection: %v", err)
		}
		// The error says, in yes/no terms only, what the page looked
		// like: here a new document on another path that still has a
		// password field.
		var se *SignInError
		if !errors.As(err, &se) || se.Evidence == nil {
			t.Fatalf("no evidence attached to %v", err)
		}
		ev := *se.Evidence
		if !ev.PageRead || !ev.NewDocument || !ev.PortalOrigin || !ev.PathChanged || !ev.PasswordField || ev.ErrorBanner {
			t.Errorf("evidence = %+v", ev)
		}
		if ev.Elapsed < 3*time.Second || ev.Elapsed > 12*time.Second {
			t.Errorf("elapsed = %v", ev.Elapsed)
		}
		if !strings.Contains(err.Error(), "password field=true") || !strings.Contains(err.Error(), "path changed=true") {
			t.Errorf("the evidence is not in the error text: %v", err)
		}
		assertNoSecret(t, err, "/guardian", "home.html", "/public", "Sign In", "LoginForm", "prelogin", "pre=", "127.0.0.1", "http://", testPassword, testUsername)
	})

	// 2. A sign-in URL that redirects to another origin showing a form
	// received the credentials.
	t.Run("redirect to another origin gets nothing", func(t *testing.T) {
		other := newForeignSite(t)
		portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.url+"/login/path-secret?next=query-secret-value", http.StatusFound)
		}))
		defer portal.Close()

		o := reviewOpts(&reviewPortal{srv: portal}, ws, 40*time.Second)
		res, err := Authenticate(context.Background(), o)
		if res != nil || !errors.Is(err, ErrOrigin) {
			t.Fatalf("Authenticate = %v, %v; want ErrOrigin", res, err)
		}
		assertStage(t, err, false)
		if got := other.received(); len(got) != 0 {
			t.Errorf("the other origin received a submission: %d", len(got))
		}
		assertNoSecret(t, err, "query-secret-value", "path-secret", "/login", testPassword, testUsername)
		if !strings.Contains(err.Error(), canonicalOrigin(other.url)) || !strings.Contains(err.Error(), canonicalOrigin(portal.URL)) {
			t.Errorf("error does not name the two origins: %v", err)
		}
	})

	// 3. Cookies of other sites the signed-in page embeds were returned,
	// and the client sends every cookie it holds to the portal.
	t.Run("only the portal's cookies are returned", func(t *testing.T) {
		assertOnlyPortalCookies(t, ws)
	})
}

// assertOnlyPortalCookies signs in to a portal whose signed-in page embeds
// a page of a sibling host, which sets a cookie of its own. The two hosts
// are names under .localhost, which a browser resolves to the loopback
// address by itself; they share a parent domain so that the browser treats
// the embedded page's cookie as first-party and keeps it. ws is the remote
// browser, or "" for the local sign-in.
func assertOnlyPortalCookies(t *testing.T, ws string) {
	t.Helper()
	const portalHost, siblingHost = "portal.signin.localhost", "sibling.signin.localhost"
	other := newForeignSite(t)
	p := newReviewPortal(t)
	p.homeEmbeds = strings.Replace(other.srv.URL, "127.0.0.1", siblingHost, 1) + "/embedded"

	o := reviewOpts(p, ws, 40*time.Second)
	o.BaseURL = strings.Replace(p.srv.URL, "127.0.0.1", portalHost, 1)
	res, err := Authenticate(context.Background(), o)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	got := cookieNames(res)
	for _, name := range []string{"session", "pre"} {
		if got[name] != portalHost {
			t.Errorf("the portal's cookie %q is missing: %v", name, got)
		}
	}
	for name, domain := range got {
		if domain != portalHost {
			t.Errorf("cookie %q of %q was returned; it does not belong to the portal", name, domain)
		}
	}
}

// 10. The browser acts on the click and the reply to it is lost. That is a
// submission whose outcome is unknown: it must never be "not submitted".
func TestReviewReal_LostReplyToTheClick(t *testing.T) {
	ws, _ := cdptest.StartChrome(t)
	proxy := cdptest.NewLossyProxy(t, ws, func(method string, params json.RawMessage) bool {
		return method == "Input.dispatchMouseEvent" && strings.Contains(string(params), "mouseReleased")
	})
	p := newReviewPortal(t)
	const timeout = 12 * time.Second

	start := time.Now()
	res, err := Authenticate(context.Background(), reviewOpts(p, proxy.URL, timeout))
	assertWithin(t, start, timeout+slack)
	if proxy.Dropped() == 0 {
		t.Fatalf("the proxy withheld nothing: the test did not exercise a lost reply (err: %v)", err)
	}
	if res != nil || err == nil {
		t.Fatalf("Authenticate = %v, %v; want an error", res, err)
	}
	assertStage(t, err, true)
	if posts := p.postList(); len(posts) != 1 {
		t.Errorf("%d submissions, want exactly 1 (the click did reach the page)", len(posts))
	}
}

// 5. A sign-in that succeeds while the browser never confirms the disposal
// of its context still succeeds, and says so in its result.
func TestReviewReal_UnconfirmedDisposalIsReported(t *testing.T) {
	ws, _ := cdptest.StartChrome(t)
	proxy := cdptest.NewLossyProxy(t, ws, func(method string, _ json.RawMessage) bool {
		return method == "Target.disposeBrowserContext"
	})
	p := newReviewPortal(t)
	res, err := Authenticate(context.Background(), reviewOpts(p, proxy.URL, 30*time.Second))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !res.ContextDisposalUnconfirmed {
		t.Errorf("ContextDisposalUnconfirmed = false although the browser never confirmed")
	}
	if _, ok := cookieNames(res)["session"]; !ok {
		t.Errorf("the session cookie is missing")
	}
}

// The local sign-in shares the origin check, the cookie filter and the
// credential check with the remote one.
func TestReviewReal_LocalSignIn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary on PATH")
	}

	t.Run("redirect to another origin gets nothing", func(t *testing.T) {
		other := newForeignSite(t)
		portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.url+"/login/path-secret?next=query-secret-value", http.StatusFound)
		}))
		defer portal.Close()

		o := reviewOpts(&reviewPortal{srv: portal}, "", 40*time.Second)
		res, err := Authenticate(context.Background(), o)
		if res != nil || !errors.Is(err, ErrOrigin) {
			t.Fatalf("Authenticate = %v, %v; want ErrOrigin", res, err)
		}
		assertStage(t, err, false)
		if got := other.received(); len(got) != 0 {
			t.Errorf("the other origin received a submission: %d", len(got))
		}
		assertNoSecret(t, err, "query-secret-value", "path-secret", "/login", testPassword, testUsername)
	})

	t.Run("only the portal's cookies are returned", func(t *testing.T) {
		assertOnlyPortalCookies(t, "")
	})
}
