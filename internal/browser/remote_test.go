package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Invented credentials. The tests in this file never get far enough to type
// them; the tests assert that.
const (
	testUsername = "user-example"
	testPassword = "pw-example-not-a-real-one"
)

func remoteOpts(browserURL string, timeout time.Duration) *AuthenticateOptions {
	o := DefaultAuthenticateOptions()
	o.BaseURL = "http://portal.invalid"
	o.Username = testUsername
	o.Password = testPassword
	o.Timeout = timeout
	o.BrowserURL = browserURL
	return o
}

// errorTree returns err and every error reachable from it through Unwrap,
// single or joined.
func errorTree(err error) []error {
	if err == nil {
		return nil
	}
	out := []error{err}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		out = append(out, errorTree(u.Unwrap())...)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			out = append(out, errorTree(e)...)
		}
	}
	return out
}

// assertNoSecret fails if any of secrets shows up in any rendering of any
// error in err's chain.
func assertNoSecret(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error")
	}
	for _, e := range errorTree(err) {
		for _, format := range []string{"%v", "%+v", "%#v", "%q", "%s"} {
			text := fmt.Sprintf(format, e)
			for _, secret := range secrets {
				if strings.Contains(text, secret) {
					t.Errorf("secret %q leaked through %s of %T: %s", secret, format, e, text)
				}
			}
		}
	}
}

// captureStdLog redirects the standard logger, which is where chromedp
// writes unless told otherwise, and returns what was written.
func captureStdLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func assertStage(t *testing.T, err error, wantSubmitted bool) {
	t.Helper()
	if got := errors.Is(err, ErrSubmitted); got != wantSubmitted {
		t.Errorf("errors.Is(err, ErrSubmitted) = %v, want %v (err: %v)", got, wantSubmitted, err)
	}
	if got := errors.Is(err, ErrNotSubmitted); got == wantSubmitted {
		t.Errorf("errors.Is(err, ErrNotSubmitted) = %v, want %v (err: %v)", got, !wantSubmitted, err)
	}
}

func assertWithin(t *testing.T, start time.Time, limit time.Duration) {
	t.Helper()
	if d := time.Since(start); d > limit {
		t.Errorf("took %v, want at most %v", d.Round(time.Millisecond), limit)
	}
}

// slack is what a bounded call may exceed its bound by on a loaded machine.
const slack = 750 * time.Millisecond

func TestRemoteOptionsValidation(t *testing.T) {
	fake := newFakeCDP(t)
	good := fake.url()

	cases := []struct {
		name   string
		mutate func(o *AuthenticateOptions)
		want   string
	}{
		{"http scheme", func(o *AuthenticateOptions) { o.BrowserURL = "http://127.0.0.1:9/?token=" + fakeToken }, "ws:// or wss://"},
		{"https scheme", func(o *AuthenticateOptions) { o.BrowserURL = "https://127.0.0.1:9/?token=" + fakeToken }, "ws:// or wss://"},
		{"file scheme", func(o *AuthenticateOptions) { o.BrowserURL = "file:///tmp/" + fakeToken }, "ws:// or wss://"},
		{"no scheme", func(o *AuthenticateOptions) { o.BrowserURL = "127.0.0.1:3000?token=" + fakeToken }, ""},
		{"scheme-relative", func(o *AuthenticateOptions) { o.BrowserURL = "//host.invalid:3000?token=" + fakeToken }, "ws:// or wss://"},
		{"no host", func(o *AuthenticateOptions) { o.BrowserURL = "ws:///path?token=" + fakeToken }, "no host"},
		{"opaque", func(o *AuthenticateOptions) { o.BrowserURL = "ws:" + fakeToken }, "no host"},
		{"unparseable", func(o *AuthenticateOptions) { o.BrowserURL = "ws://host.invalid/%zz?token=" + fakeToken }, "could not be parsed"},
		{"control character", func(o *AuthenticateOptions) { o.BrowserURL = "ws://host.invalid/\x7f?token=" + fakeToken }, "could not be parsed"},
		{"user data dir", func(o *AuthenticateOptions) { o.UserDataDir = "/tmp/profile-" + fakeToken }, "UserDataDir"},
		{"no sandbox", func(o *AuthenticateOptions) { o.NoSandbox = true }, "NoSandbox"},
		{"headed", func(o *AuthenticateOptions) { o.Headless = false }, "Headless"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := remoteOpts(good, 5*time.Second)
			tc.mutate(o)
			res, err := Authenticate(context.Background(), o)
			if err == nil || res != nil {
				t.Fatalf("Authenticate = %v, %v; want an error", res, err)
			}
			if !errors.Is(err, ErrInvalidOptions) {
				t.Errorf("error is not ErrInvalidOptions: %v", err)
			}
			assertStage(t, err, false)
			if errors.Is(err, ErrBrowserConnection) {
				t.Errorf("an options error must not look like a connection failure: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			assertNoSecret(t, err, fakeToken)
		})
	}
	if n := len(fake.connections()); n != 0 {
		t.Errorf("%d connections were made to the remote browser by rejected options, want 0", n)
	}
}

func TestRemoteOptionsValidation_Accepted(t *testing.T) {
	for _, u := range []string{
		"ws://browser.example.internal:3000?token=abc",
		"wss://browser.example.internal/?token=abc",
		"WS://browser.example.internal:3000",
		"ws://user:pw@browser.example.internal:3000/chromium?token=abc&launch=x",
		"ws://[::1]:3000/devtools/browser/0000",
	} {
		o := remoteOpts(u, time.Second)
		if err := validateRemoteOptions(o); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
}

func TestRedactor(t *testing.T) {
	cases := []struct {
		raw, display string
		secrets      []string
	}{
		{"ws://browser.example.internal:3000?token=abc123XYZ", "ws://browser.example.internal:3000", []string{"abc123XYZ"}},
		{"wss://browser.example.internal/chromium/path?token=abc123XYZ&other=zzTOPzz", "wss://browser.example.internal", []string{"abc123XYZ", "zzTOPzz", "/chromium/path"}},
		{"ws://someuser:hunter2hunter2@browser.example.internal:3000/", "ws://browser.example.internal:3000", []string{"hunter2hunter2", "someuser"}},
		{"ws://browser.example.internal:3000/?token=a%2Fb%2Bc", "ws://browser.example.internal:3000", []string{"a%2Fb%2Bc", "a/b+c"}},
		{"WS://Browser.Example.Internal:3000#frag-secret-1", "ws://Browser.Example.Internal:3000", []string{"frag-secret-1"}},
		{"ws://host.invalid/%zz?token=abc123XYZ", unparseableEndpoint, []string{"abc123XYZ"}},
	}
	for _, tc := range cases {
		r := newRedactor(tc.raw)
		if r.display != tc.display {
			t.Errorf("display of %q = %q, want %q", tc.raw, r.display, tc.display)
		}
		for _, secret := range tc.secrets {
			if strings.Contains(r.display, secret) {
				t.Errorf("display %q contains %q", r.display, secret)
			}
		}
		// Shapes in which a dialer or a URL parser reports the endpoint.
		for _, text := range []string{
			fmt.Sprintf("could not dial %q: boom", tc.raw),
			"parse \"" + tc.raw + "\": invalid URL escape",
			"GET " + tc.raw + " failed",
			"secrets: " + strings.Join(tc.secrets, " | "),
		} {
			got := r.scrub(text)
			for _, secret := range append([]string{tc.raw}, tc.secrets...) {
				if strings.Contains(got, secret) {
					t.Errorf("scrub(%q) = %q still contains %q", text, got, secret)
				}
			}
		}
	}
}

// closedAddr returns an address on which nothing listens.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestRemoteDialFailures_NeverLeakTheToken(t *testing.T) {
	// A service that refuses the handshake and echoes the request back,
	// the way a proxy might in an error page.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Was", r.URL.String())
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad token: " + r.URL.String()))
	}))
	defer echo.Close()

	// A service that redirects, repeating the token in the Location.
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://elsewhere.invalid/"+r.URL.RawQuery, http.StatusFound)
	}))
	defer redirect.Close()

	// A TLS-less service addressed as wss://.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer plain.Close()

	hostOf := func(s *httptest.Server) string { return strings.TrimPrefix(s.URL, "http://") }
	query := "/some/path?token=" + fakeToken + "&launch=" + fakeToken

	cases := []struct {
		name string
		url  string
		host string
	}{
		{"connection refused", "ws://" + closedAddr(t) + query, ""},
		{"handshake refused with echo", "ws://" + hostOf(echo) + query, hostOf(echo)},
		{"handshake redirected", "ws://" + hostOf(redirect) + query, hostOf(redirect)},
		{"tls to a plain server", "wss://" + hostOf(plain) + query, hostOf(plain)},
		{"token in userinfo", "ws://" + fakeToken + ":" + fakeToken + "@" + closedAddr(t) + "/", ""},
		{"unresolvable host", "ws://no-such-host.invalid:3000" + query, "no-such-host.invalid:3000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureStdLog(t)
			start := time.Now()
			res, err := Authenticate(context.Background(), remoteOpts(tc.url, 8*time.Second))
			if err == nil || res != nil {
				t.Fatalf("Authenticate = %v, %v; want an error", res, err)
			}
			t.Logf("error as returned: %v", err)
			assertWithin(t, start, 8*time.Second+slack)
			assertNoSecret(t, err, fakeToken, "/some/path")
			if !errors.Is(err, ErrBrowserConnection) {
				t.Errorf("error is not ErrBrowserConnection: %v", err)
			}
			assertStage(t, err, false)
			if tc.host != "" && !strings.Contains(err.Error(), tc.host) {
				t.Errorf("error %q does not name the host %q", err, tc.host)
			}
			if s := logged.String(); strings.Contains(s, fakeToken) {
				t.Errorf("the token was logged: %s", s)
			}
		})
	}
}

// hangingListener accepts connections and never answers them.
func hangingListener(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return l.Addr().String()
}

func TestRemoteDial_TimeoutBoundsTheDial(t *testing.T) {
	addr := hangingListener(t)
	const timeout = 1500 * time.Millisecond

	start := time.Now()
	_, err := Authenticate(context.Background(), remoteOpts("ws://"+addr+"/?token="+fakeToken, timeout))
	assertWithin(t, start, timeout+slack)
	assertNoSecret(t, err, fakeToken)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	if !errors.Is(err, ErrBrowserConnection) {
		t.Errorf("error is not ErrBrowserConnection: %v", err)
	}
	assertStage(t, err, false)
}

func TestRemoteDial_CallerCancelStopsTheDial(t *testing.T) {
	addr := hangingListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	start := time.Now()
	_, err := Authenticate(ctx, remoteOpts("ws://"+addr+"/?token="+fakeToken, 30*time.Second))
	assertWithin(t, start, 300*time.Millisecond+2*time.Second)
	assertNoSecret(t, err, fakeToken)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error is not context.Canceled: %v", err)
	}
	assertStage(t, err, false)
}

func TestRemote_ContextAlreadyDone(t *testing.T) {
	fake := newFakeCDP(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Authenticate(ctx, remoteOpts(fake.url(), 5*time.Second))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error is not context.Canceled: %v", err)
	}
	assertStage(t, err, false)
	if n := len(fake.connections()); n != 0 {
		t.Errorf("%d connections made with a context that was already done, want 0", n)
	}
}

// The following tests run the sign-in against the fake DevTools server. The
// fake never finishes loading a page, so each of them ends before the form
// is filled; what they establish is the lifecycle of the browser context.

func TestRemoteFake_TimeoutWhileLoading_DisposesItsContext(t *testing.T) {
	fake := newFakeCDP(t)
	logged := captureStdLog(t)
	const timeout = 2 * time.Second

	start := time.Now()
	res, err := Authenticate(context.Background(), remoteOpts(fake.url(), timeout))
	assertWithin(t, start, timeout+slack)
	if err == nil || res != nil {
		t.Fatalf("Authenticate = %v, %v; want a timeout", res, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	if errors.Is(err, ErrBrowserConnection) {
		t.Errorf("a timeout on a live connection must not be a connection failure: %v", err)
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, fakeToken, testPassword, testUsername)
	if !strings.Contains(err.Error(), "opening the sign-in page") {
		t.Errorf("error does not say which step timed out: %v", err)
	}

	fake.waitClosed(t)
	calls := fake.snapshot()
	assertIsolatedAndDisposed(t, calls, true)
	assertNoInput(t, calls)
	if indexOfCall(calls, "Page.navigate") < 0 {
		t.Errorf("the navigation was never started; calls: %v", methodsOf(calls))
	}

	// The endpoint is dialled exactly as given, once, with no discovery
	// request that could repeat the token elsewhere.
	if conns := fake.connections(); len(conns) != 1 || conns[0] != "/?token="+fakeToken {
		t.Errorf("connections = %q, want exactly the endpoint as given", conns)
	}
	if s := logged.String(); strings.Contains(s, fakeToken) || strings.Contains(s, testPassword) {
		t.Errorf("a secret was logged: %s", s)
	}
}

func TestRemoteFake_BrowserDiesMidLogin_ClassifiedNotHung(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Page.navigate", "close")

	start := time.Now()
	res, err := Authenticate(context.Background(), remoteOpts(fake.url(), 30*time.Second))
	assertWithin(t, start, 5*time.Second)
	if err == nil || res != nil {
		t.Fatalf("Authenticate = %v, %v; want an error", res, err)
	}
	if !errors.Is(err, ErrBrowserConnection) {
		t.Errorf("error is not ErrBrowserConnection: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Errorf("a lost connection must not be reported as the caller's timeout or cancellation: %v", err)
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, fakeToken, testPassword)
	if strings.Contains(err.Error(), "not confirmed") {
		t.Errorf("a context that went away with its connection needs no disposal warning: %v", err)
	}
	assertNothingElseTouched(t, fake.snapshot())
	assertNoInput(t, fake.snapshot())
}

func TestRemoteFake_CallerCancel_DisposesItsContext(t *testing.T) {
	fake := newFakeCDP(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-fake.sawMethod("Page.navigate")
		cancel()
	}()

	start := time.Now()
	_, err := Authenticate(ctx, remoteOpts(fake.url(), 30*time.Second))
	assertWithin(t, start, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error is not context.Canceled: %v", err)
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, fakeToken, testPassword)

	fake.waitClosed(t)
	assertIsolatedAndDisposed(t, fake.snapshot(), true)
	assertNoInput(t, fake.snapshot())
}

func TestRemoteFake_StallCreatingThePage_StillDisposesItsContext(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Target.createTarget", "stall")
	const timeout = 2 * time.Second

	start := time.Now()
	_, err := Authenticate(context.Background(), remoteOpts(fake.url(), timeout))
	assertWithin(t, start, timeout+slack)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	assertStage(t, err, false)

	fake.waitClosed(t)
	calls := fake.snapshot()
	assertIsolatedAndDisposed(t, calls, false)
	if n := countCalls(calls, "Target.attachToTarget"); n != 0 {
		t.Errorf("attached to %d targets, want 0", n)
	}
}

// A browser with windows refuses a tab in a browser context that has no
// window yet. The page is then asked for as a window of the same context;
// nothing is created outside it.
func TestRemoteFake_PageRefusedAsATab_IsCreatedAsAWindowOfTheSameContext(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Target.createTarget", "refuse-once")
	const timeout = 2 * time.Second

	_, err := Authenticate(context.Background(), remoteOpts(fake.url(), timeout))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	fake.waitClosed(t)
	calls := fake.snapshot()
	assertIsolatedAndDisposed(t, calls, true)

	var creates []fakeCall
	for _, c := range calls {
		if c.Method == "Target.createTarget" {
			creates = append(creates, c)
		}
	}
	if len(creates) != 2 {
		t.Fatalf("%d attempts to create the page, want 2", len(creates))
	}
	if creates[0].Params["newWindow"] == true {
		t.Errorf("the first attempt already asked for a window")
	}
	if creates[1].Params["newWindow"] != true || creates[1].Params["browserContextId"] != fakeContextID {
		t.Errorf("the second attempt was not a window of the sign-in's context: %v", creates[1].Params)
	}
	if indexOfCall(calls, "Page.navigate") < 0 {
		t.Errorf("the sign-in did not go on after the page was created; calls: %v", methodsOf(calls))
	}
}

func TestRemoteFake_StallCreatingTheContext_ClosesTheConnection(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Target.createBrowserContext", "stall")
	const timeout = 2 * time.Second

	start := time.Now()
	_, err := Authenticate(context.Background(), remoteOpts(fake.url(), timeout))
	assertWithin(t, start, timeout+slack)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	assertStage(t, err, false)

	// The context's id was never learned, so it cannot be disposed by id.
	// Closing the connection is what disposes it (disposeOnDetach).
	fake.waitClosed(t)
	calls := fake.snapshot()
	ci := indexOfCall(calls, "Target.createBrowserContext")
	if ci < 0 || calls[ci].Params["disposeOnDetach"] != true {
		t.Errorf("the browser context was not requested with disposeOnDetach; calls: %v", methodsOf(calls))
	}
	if n := countCalls(calls, "Target.createTarget"); n != 0 {
		t.Errorf("%d targets created without a browser context, want 0", n)
	}
	assertNothingElseTouched(t, calls)
}

func TestRemoteFake_DisposalThatNeverAnswers_IsBoundedAndReported(t *testing.T) {
	fake := newFakeCDP(t)
	fake.on("Target.disposeBrowserContext", "stall")
	const timeout = 2 * time.Second

	start := time.Now()
	_, err := Authenticate(context.Background(), remoteOpts(fake.url(), timeout))
	assertWithin(t, start, timeout+slack)
	if err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Errorf("error does not report the unconfirmed disposal: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error is not context.DeadlineExceeded: %v", err)
	}
	assertStage(t, err, false)
	assertNoSecret(t, err, fakeToken)

	// The connection is closed all the same, shortly after.
	fake.waitClosed(t)
	assertNothingElseTouched(t, fake.snapshot())
}

func TestRemoteFake_Panic_DisposesItsContext(t *testing.T) {
	fake := newFakeCDP(t)
	remoteTestHook = func(point string) {
		if point == "context-open" {
			panic("boom in the middle of a sign-in")
		}
	}
	defer func() { remoteTestHook = nil }()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("the panic did not propagate")
			}
		}()
		_, _ = Authenticate(context.Background(), remoteOpts(fake.url(), 10*time.Second))
	}()

	fake.waitClosed(t)
	assertIsolatedAndDisposed(t, fake.snapshot(), true)
}

func TestNetErrorCode(t *testing.T) {
	for in, want := range map[string]string{
		"page load error net::ERR_NAME_NOT_RESOLVED":                   "net::ERR_NAME_NOT_RESOLVED",
		"page load error net::ERR_CONNECTION_REFUSED at http://x/?a=b": "net::ERR_CONNECTION_REFUSED",
		"page load error net::ERR_HTTP2_PROTOCOL_ERROR":                "net::ERR_HTTP2_PROTOCOL_ERROR",
		"something else entirely, with a secret in it":                 "",
		"net::ERR_": "net::ERR_",
		"page load error net::ERR_ABORTED<script>secret</script>": "net::ERR_ABORTED",
	} {
		if got := netErrorCode(errors.New(in)); got != want {
			t.Errorf("netErrorCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTeardownBudget(t *testing.T) {
	for _, tc := range []struct{ timeout, want time.Duration }{
		{60 * time.Second, 5 * time.Second},
		{20 * time.Second, 5 * time.Second},
		{8 * time.Second, 2 * time.Second},
		{time.Second, 250 * time.Millisecond},
	} {
		if got := teardownBudget(tc.timeout); got != tc.want {
			t.Errorf("teardownBudget(%v) = %v, want %v", tc.timeout, got, tc.want)
		}
	}
}

func TestRemoteError_Classification(t *testing.T) {
	cases := []struct {
		name string
		err  *RemoteError
		is   []error
		not  []error
	}{
		{"options", &RemoteError{Stage: StageOptions, msg: "m"},
			[]error{ErrInvalidOptions, ErrNotSubmitted}, []error{ErrSubmitted, ErrBrowserConnection, ErrRejected}},
		{"connect", &RemoteError{Stage: StageConnect, Connection: true, msg: "m"},
			[]error{ErrBrowserConnection, ErrNotSubmitted}, []error{ErrSubmitted, ErrInvalidOptions, ErrRejected}},
		{"before submit, timeout", &RemoteError{Stage: StageBeforeSubmit, msg: "m", ctxErr: context.DeadlineExceeded},
			[]error{ErrNotSubmitted, context.DeadlineExceeded}, []error{ErrSubmitted, ErrBrowserConnection}},
		{"after submit, lost", &RemoteError{Stage: StageAfterSubmit, Connection: true, msg: "m"},
			[]error{ErrSubmitted, ErrBrowserConnection}, []error{ErrNotSubmitted, ErrRejected}},
		{"rejected", &RemoteError{Stage: StageAfterSubmit, Rejected: true, msg: "m"},
			[]error{ErrSubmitted, ErrRejected}, []error{ErrNotSubmitted, ErrBrowserConnection}},
	}
	for _, tc := range cases {
		for _, target := range tc.is {
			if !errors.Is(tc.err, target) {
				t.Errorf("%s: errors.Is(%v) = false, want true", tc.name, target)
			}
		}
		for _, target := range tc.not {
			if errors.Is(tc.err, target) {
				t.Errorf("%s: errors.Is(%v) = true, want false", tc.name, target)
			}
		}
	}
}

// scriptedSubmit is a submitDriver that records what is asked of it.
type scriptedSubmit struct {
	button            string
	findErr, readyErr error
	clickErr          error
	log               []string
}

func (s *scriptedSubmit) find(context.Context) (string, error) {
	s.log = append(s.log, "find")
	return s.button, s.findErr
}

func (s *scriptedSubmit) ready(context.Context, string) error {
	s.log = append(s.log, "ready")
	return s.readyErr
}

func (s *scriptedSubmit) click(_ context.Context, sel string) error {
	s.log = append(s.log, "click "+sel)
	return s.clickErr
}

func (s *scriptedSubmit) enter(context.Context) error {
	s.log = append(s.log, "enter")
	return nil
}

func TestSubmitOnce(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name      string
		d         *scriptedSubmit
		wantLog   string
		wantMarks int
		wantErr   bool
	}{
		{"button clicked once", &scriptedSubmit{button: "#b"}, "find ready mark click #b", 1, false},
		{"a failed click is not followed by anything", &scriptedSubmit{button: "#b", clickErr: boom}, "find ready mark click #b", 1, true},
		{"no button: enter once", &scriptedSubmit{}, "find mark enter", 1, false},
		{"button never clickable: nothing sent", &scriptedSubmit{button: "#b", readyErr: boom}, "find ready", 0, true},
		{"lookup interrupted: nothing sent", &scriptedSubmit{findErr: boom}, "find", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marks := 0
			err := submitOnce(context.Background(), tc.d, func() {
				marks++
				tc.d.log = append(tc.d.log, "mark")
			})
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got := strings.Join(tc.d.log, " "); got != tc.wantLog {
				t.Errorf("sequence = %q, want %q", got, tc.wantLog)
			}
			if marks != tc.wantMarks {
				t.Errorf("marked submitted %d times, want %d", marks, tc.wantMarks)
			}
		})
	}
}
