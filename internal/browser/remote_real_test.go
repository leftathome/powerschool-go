package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"github.com/leftathome/powerschool-go/internal/cdptest"
)

// The tests in this file run the remote sign-in against a real headless
// Chrome started locally with its DevTools port open, and a local stand-in
// for the portal. They are skipped when no browser is installed.

const (
	stubLoginForm = `<!doctype html><html><head><title>Sign In</title></head><body>
%ALERT%
<form id="LoginForm" action="/guardian/home.html" method="post">
  <input type="text" id="fieldAccount" name="account">
  <input type="password" id="fieldPassword" name="pw">
  <button type="submit" id="btn-enter-sign-in">Sign In</button>
</form>
</body></html>`

	stubNoForm = `<!doctype html><html><head><title>Maintenance</title></head><body>
<p>The portal is unavailable.</p></body></html>`

	stubHome = `<!doctype html><html><head><title>Home</title></head><body><p>signed in</p></body></html>`

	stubSessionValue = "session-value-invented-1"
	stubGatewayValue = "gateway-value-invented-1"
)

// portalStub stands in for the portal: a gateway that sets a cookie and
// redirects before the sign-in page is served, a sign-in form, and a home
// page behind it.
type portalStub struct {
	srv *httptest.Server

	// mode: "ok", "reject", "no-form", "hang-get", "hang-post".
	mode string

	mu               sync.Mutex
	posts            int
	gotAccount       string
	gotPassword      string
	postHadGateway   bool
	sawBystander     bool
	loginUserAgents  []string
	postArrived      chan struct{}
	getArrived       chan struct{}
	release          chan struct{}
	arrivedOnce      sync.Once
	getArrivedOnce   sync.Once
	releaseOnce      sync.Once
	requestsToPortal int
}

func newPortalStub(t *testing.T, mode string) *portalStub {
	t.Helper()
	p := &portalStub{
		mode:        mode,
		postArrived: make(chan struct{}),
		getArrived:  make(chan struct{}),
		release:     make(chan struct{}),
	}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(func() {
		p.unblock()
		p.srv.CloseClientConnections()
		p.srv.Close()
	})
	return p
}

func (p *portalStub) unblock() { p.releaseOnce.Do(func() { close(p.release) }) }

func (p *portalStub) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if r.URL.Path == "/bystander" {
		http.SetCookie(w, &http.Cookie{Name: "bystander", Value: "other-client", Path: "/"})
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>bystander</title></head><body>someone else's tab</body></html>`))
		return
	}
	if r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}

	p.mu.Lock()
	p.requestsToPortal++
	if _, err := r.Cookie("bystander"); err == nil {
		p.sawBystander = true
	}
	p.loginUserAgents = append(p.loginUserAgents, r.UserAgent())
	p.mu.Unlock()

	_, gwErr := r.Cookie("gw")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/public/":
		if gwErr != nil {
			// The gateway: set its cookie, then send the browser
			// round again. Without the cookie being carried across
			// this redirect there is no sign-in page.
			http.SetCookie(w, &http.Cookie{Name: "gw", Value: stubGatewayValue, Path: "/"})
			http.Redirect(w, r, "/public/?via=gateway", http.StatusFound)
			return
		}
		switch p.mode {
		case "no-form":
			_, _ = w.Write([]byte(stubNoForm))
		case "hang-get":
			p.getArrivedOnce.Do(func() { close(p.getArrived) })
			<-p.release
		default:
			_, _ = w.Write([]byte(strings.Replace(stubLoginForm, "%ALERT%", "", 1)))
		}

	case r.Method == http.MethodPost && r.URL.Path == "/guardian/home.html":
		_ = r.ParseForm()
		p.mu.Lock()
		p.posts++
		p.gotAccount = r.PostForm.Get("account")
		p.gotPassword = r.PostForm.Get("pw")
		p.postHadGateway = gwErr == nil
		p.mu.Unlock()
		p.arrivedOnce.Do(func() { close(p.postArrived) })

		switch p.mode {
		case "hang-post":
			<-p.release
		case "reject":
			_, _ = w.Write([]byte(strings.Replace(stubLoginForm, "%ALERT%", `<div class="feedback-alert">Invalid Username or Password!</div>`, 1)))
		default:
			http.SetCookie(w, &http.Cookie{Name: "session", Value: stubSessionValue, Path: "/", HttpOnly: true})
			http.Redirect(w, r, "/guardian/home.html", http.StatusSeeOther)
		}

	case r.Method == http.MethodGet && r.URL.Path == "/guardian/home.html":
		_, _ = w.Write([]byte(stubHome))

	default:
		http.NotFound(w, r)
	}
}

func (p *portalStub) postCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.posts
}

// bystander is another client of the same browser, with a tab of its own in
// the browser's default context. It is what the sign-in must not disturb,
// and it is also how the tests look at what is left in the browser.
type bystander struct {
	ctx context.Context
}

func newBystander(t *testing.T, wsURL, pageURL string) *bystander {
	t.Helper()
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), wsURL, chromedp.NoModifyURL)
	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(func() {
		cancel()
		cancelAlloc()
	})
	// No timeout on this context: chromedp ties the connection to the
	// context of the first Run.
	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL)); err != nil {
		t.Fatalf("bystander tab: %v", err)
	}
	return &bystander{ctx: ctx}
}

type browserState struct {
	contexts []cdp.BrowserContextID
	pageURLs []string
	cookies  map[string]string // default context
	title    string            // of the bystander's own tab
}

func (b *bystander) state(t *testing.T) browserState {
	t.Helper()
	var st browserState
	st.cookies = map[string]string{}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	err := chromedp.Run(ctx,
		chromedp.Title(&st.title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			bx := cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Browser)
			ids, err := target.GetBrowserContexts().Do(bx)
			if err != nil {
				return err
			}
			st.contexts = ids
			infos, err := target.GetTargets().Do(bx)
			if err != nil {
				return err
			}
			for _, info := range infos {
				if info.Type == "page" {
					st.pageURLs = append(st.pageURLs, info.URL)
				}
			}
			cookies, err := storage.GetCookies().Do(bx)
			if err != nil {
				return err
			}
			for _, c := range cookies {
				st.cookies[c.Name] = c.Value
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("the shared browser is no longer usable by its other client: %v", err)
	}
	return st
}

// assertNothingLeft checks the shared browser after a sign-in: it is still
// running, the other client's tab and cookie are intact, and no browser
// context, page, or cookie of the sign-in remains.
func assertNothingLeft(t *testing.T, b *bystander, before browserState) {
	t.Helper()
	after := b.state(t)
	if len(after.contexts) != 0 {
		t.Errorf("%d browser contexts left in the shared browser, want 0", len(after.contexts))
	}
	if after.title != "bystander" {
		t.Errorf("the other client's tab was disturbed: title %q", after.title)
	}
	if len(after.pageURLs) != len(before.pageURLs) {
		t.Errorf("pages in the shared browser: %d before the sign-in, %d after", len(before.pageURLs), len(after.pageURLs))
	}
	for _, u := range after.pageURLs {
		if strings.Contains(u, "/public/") || strings.Contains(u, "/guardian/") {
			t.Errorf("a page of the sign-in is still open in the shared browser")
		}
	}
	if after.cookies["bystander"] != "other-client" {
		t.Errorf("the other client's cookie is gone from the default context")
	}
	for _, name := range []string{"session", "gw"} {
		if _, ok := after.cookies[name]; ok {
			t.Errorf("cookie %q of the sign-in is in the shared browser's default context", name)
		}
	}
}

func realOpts(p *portalStub, wsURL string, timeout time.Duration) *AuthenticateOptions {
	o := DefaultAuthenticateOptions()
	o.BaseURL = p.srv.URL
	o.Username = testUsername
	o.Password = testPassword
	o.Timeout = timeout
	o.BrowserURL = wsURL
	return o
}

// withToken appends a token to the browser's endpoint, as a browser service
// would require. Chrome itself ignores the query.
func withToken(wsURL string) string {
	return wsURL + "?token=" + fakeToken
}

func TestRemoteReal_SignInOnASharedBrowser(t *testing.T) {
	wsURL, _ := cdptest.StartChrome(t)

	t.Run("accepted", func(t *testing.T) {
		portal := newPortalStub(t, "ok")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)
		logged := captureStdLog(t)

		opts := realOpts(portal, withToken(wsURL), 60*time.Second)
		res, err := Authenticate(context.Background(), opts)
		if err != nil {
			t.Fatalf("Authenticate: %v", err)
		}
		if res.ContextDisposalUnconfirmed {
			t.Errorf("the browser did not confirm the disposal of the sign-in's context")
		}

		got := map[string]string{}
		for _, c := range res.Cookies {
			got[c.Name] = c.Value
		}
		if got["session"] != stubSessionValue {
			t.Errorf("the session cookie was not returned")
		}
		if got["gw"] != stubGatewayValue {
			t.Errorf("the gateway cookie was not returned")
		}
		if _, ok := got["bystander"]; ok {
			t.Errorf("a cookie of the browser's default context was returned: the sign-in did not run in a context of its own")
		}
		if !res.ExpiresAt.After(time.Now()) {
			t.Errorf("ExpiresAt = %v, want a time in the future", res.ExpiresAt)
		}

		portal.mu.Lock()
		if portal.posts != 1 {
			t.Errorf("the form was submitted %d times, want exactly 1", portal.posts)
		}
		if portal.gotAccount != testUsername || portal.gotPassword != testPassword {
			t.Errorf("the portal did not receive the credentials as given")
		}
		if !portal.postHadGateway {
			t.Errorf("the gateway cookie was not carried to the submission")
		}
		if portal.sawBystander {
			t.Errorf("the sign-in sent a cookie of the default context to the portal: it did not run in a context of its own")
		}
		for _, ua := range portal.loginUserAgents {
			if ua != opts.UserAgent {
				t.Errorf("the portal saw user agent %q, want the configured one", ua)
				break
			}
		}
		portal.mu.Unlock()

		assertNothingLeft(t, other, before)
		if s := logged.String(); strings.Contains(s, fakeToken) || strings.Contains(s, testPassword) || strings.Contains(s, stubSessionValue) {
			t.Errorf("a secret was logged")
		}
	})

	t.Run("rejected", func(t *testing.T) {
		portal := newPortalStub(t, "reject")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)

		res, err := Authenticate(context.Background(), realOpts(portal, withToken(wsURL), 60*time.Second))
		if err == nil || res != nil {
			t.Fatalf("Authenticate = %v, %v; want a rejection", res, err)
		}
		if !errors.Is(err, ErrRejected) {
			t.Errorf("error is not ErrRejected: %v", err)
		}
		assertStage(t, err, true)
		assertNoSecret(t, err, fakeToken, testPassword, testUsername, "Invalid Username")
		if n := portal.postCount(); n != 1 {
			t.Errorf("a rejected sign-in was submitted %d times, want exactly 1", n)
		}
		assertNothingLeft(t, other, before)
	})

	t.Run("no sign-in form", func(t *testing.T) {
		prev := remoteFieldWait
		remoteFieldWait = time.Second
		defer func() { remoteFieldWait = prev }()

		portal := newPortalStub(t, "no-form")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)

		_, err := Authenticate(context.Background(), realOpts(portal, withToken(wsURL), 60*time.Second))
		if err == nil || !strings.Contains(err.Error(), "username field was not found") {
			t.Fatalf("error = %v, want the missing username field", err)
		}
		assertStage(t, err, false)
		assertNoSecret(t, err, fakeToken, testPassword, "unavailable")
		if n := portal.postCount(); n != 0 {
			t.Errorf("%d submissions without a form, want 0", n)
		}
		assertNothingLeft(t, other, before)
	})

	t.Run("portal unreachable from the browser", func(t *testing.T) {
		portal := newPortalStub(t, "ok")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)

		opts := realOpts(portal, withToken(wsURL), 60*time.Second)
		opts.BaseURL = "http://" + closedAddr(t)
		_, err := Authenticate(context.Background(), opts)
		if err == nil || !strings.Contains(err.Error(), "net::ERR_CONNECTION_REFUSED") {
			t.Fatalf("error = %v, want the browser's network error code", err)
		}
		assertStage(t, err, false)
		if errors.Is(err, ErrBrowserConnection) {
			t.Errorf("an unreachable portal is not an unreachable browser: %v", err)
		}
		assertNoSecret(t, err, fakeToken, testPassword)
		assertNothingLeft(t, other, before)
	})

	t.Run("cancelled after submitting", func(t *testing.T) {
		portal := newPortalStub(t, "hang-post")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-portal.postArrived
			cancel()
		}()

		_, err := Authenticate(ctx, realOpts(portal, withToken(wsURL), 60*time.Second))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error is not context.Canceled: %v", err)
		}
		assertStage(t, err, true)
		assertNoSecret(t, err, fakeToken, testPassword)
		if n := portal.postCount(); n != 1 {
			t.Errorf("submitted %d times, want exactly 1", n)
		}
		portal.unblock()
		assertNothingLeft(t, other, before)
	})

	t.Run("timeout while the sign-in page loads", func(t *testing.T) {
		portal := newPortalStub(t, "hang-get")
		other := newBystander(t, wsURL, portal.srv.URL+"/bystander")
		before := other.state(t)
		const timeout = 6 * time.Second

		start := time.Now()
		_, err := Authenticate(context.Background(), realOpts(portal, withToken(wsURL), timeout))
		assertWithin(t, start, timeout+slack)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error is not context.DeadlineExceeded: %v", err)
		}
		assertStage(t, err, false)
		if n := portal.postCount(); n != 0 {
			t.Errorf("%d submissions before the page loaded, want 0", n)
		}
		portal.unblock()
		assertNothingLeft(t, other, before)
	})
}

// The local sign-in shares its selectors and its cookie conversion with the
// remote one. This runs it, unchanged, with a browser it starts itself.
func TestLocalAuthenticate_RealBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary on PATH")
	}
	portal := newPortalStub(t, "ok")
	opts := realOpts(portal, "", 60*time.Second)

	res, err := Authenticate(context.Background(), opts)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	got := map[string]string{}
	for _, c := range res.Cookies {
		got[c.Name] = c.Value
	}
	if got["session"] != stubSessionValue || got["gw"] != stubGatewayValue {
		t.Errorf("the session and gateway cookies were not both returned")
	}
	if n := portal.postCount(); n != 1 {
		t.Errorf("the form was submitted %d times, want exactly 1", n)
	}
	if res.ContextDisposalUnconfirmed {
		t.Errorf("ContextDisposalUnconfirmed is set by a local sign-in")
	}
}

func TestRemoteReal_BrowserKilledBeforeSubmitting(t *testing.T) {
	wsURL, kill := cdptest.StartChrome(t)
	portal := newPortalStub(t, "hang-get")
	go func() {
		<-portal.getArrived
		kill()
	}()

	start := time.Now()
	_, err := Authenticate(context.Background(), realOpts(portal, withToken(wsURL), 60*time.Second))
	assertWithin(t, start, 20*time.Second)
	if !errors.Is(err, ErrBrowserConnection) {
		t.Fatalf("error is not ErrBrowserConnection: %v", err)
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, fakeToken, testPassword)
	if n := portal.postCount(); n != 0 {
		t.Errorf("%d submissions, want 0", n)
	}
}

func TestRemoteReal_BrowserKilledAfterSubmitting(t *testing.T) {
	wsURL, kill := cdptest.StartChrome(t)
	portal := newPortalStub(t, "hang-post")
	go func() {
		<-portal.postArrived
		kill()
	}()

	start := time.Now()
	_, err := Authenticate(context.Background(), realOpts(portal, withToken(wsURL), 60*time.Second))
	assertWithin(t, start, 20*time.Second)
	if !errors.Is(err, ErrBrowserConnection) {
		t.Fatalf("error is not ErrBrowserConnection: %v", err)
	}
	assertStage(t, err, true)
	assertNoSecret(t, err, fakeToken, testPassword)
	if n := portal.postCount(); n != 1 {
		t.Errorf("%d submissions, want exactly 1", n)
	}
}
