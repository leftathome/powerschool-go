package powerschool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leftathome/powerschool-go/internal/cdptest"
)

// Invented values: the token a browser service would require, and
// credentials no portal has ever seen.
const (
	remoteTestToken    = "tok-0f-the-shared-browser-5ecr3t"
	remoteTestUser     = "user-example"
	remoteTestPassword = "pw-example-not-a-real-one"
)

func remoteTestClient(t *testing.T, baseURL string, logs *bytes.Buffer) *Client {
	t.Helper()
	c, err := NewClient("portal.invalid",
		WithCredentials(remoteTestUser, remoteTestPassword),
		WithLogger(NewLogger(LogLevelDebug, logs)),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if baseURL != "" {
		// NewClient insists on https; the local stand-in speaks http.
		c.baseURL = baseURL
	}
	return c
}

// everyError returns err and everything reachable from it by unwrapping.
func everyError(err error) []error {
	if err == nil {
		return nil
	}
	out := []error{err}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		out = append(out, everyError(u.Unwrap())...)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			out = append(out, everyError(e)...)
		}
	}
	return out
}

func assertRemoteErrorIsClean(t *testing.T, err error, logs *bytes.Buffer, secrets ...string) {
	t.Helper()
	for _, e := range everyError(err) {
		for _, format := range []string{"%v", "%+v", "%#v", "%q"} {
			text := fmt.Sprintf(format, e)
			for _, secret := range secrets {
				if strings.Contains(text, secret) {
					t.Errorf("secret %q leaked through %s of %T", secret, format, e)
				}
			}
		}
	}
	for _, secret := range secrets {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("secret %q was logged", secret)
		}
	}
}

func TestAuthenticateWithOptions_BrowserURL_DialFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	var logs bytes.Buffer
	c := remoteTestClient(t, "", &logs)
	err = c.AuthenticateWithOptions(context.Background(), &AuthOptions{
		BrowserURL: "ws://" + addr + "/chromium?token=" + remoteTestToken,
		Timeout:    10 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !IsAuthError(err) {
		t.Errorf("IsAuthError = false for %v", err)
	}
	if !errors.Is(err, ErrBrowserConnection) {
		t.Errorf("error is not ErrBrowserConnection: %v", err)
	}
	if !errors.Is(err, ErrCredentialsNotSubmitted) || errors.Is(err, ErrCredentialsSubmitted) {
		t.Errorf("error does not say the credentials were not submitted: %v", err)
	}
	if !strings.Contains(err.Error(), "ws://"+addr) {
		t.Errorf("error does not name the endpoint's host: %v", err)
	}
	if c.IsAuthenticated() {
		t.Errorf("the client is authenticated after a failed sign-in")
	}
	assertRemoteErrorIsClean(t, err, &logs, remoteTestToken, remoteTestPassword, "/chromium")
}

func TestAuthenticateWithOptions_BrowserURL_InvalidCombinations(t *testing.T) {
	headed := false
	good := "ws://browser.example.internal:3000?token=" + remoteTestToken
	cases := map[string]*AuthOptions{
		"http endpoint":  {BrowserURL: "http://browser.example.internal:3000?token=" + remoteTestToken},
		"https endpoint": {BrowserURL: "https://browser.example.internal:3000?token=" + remoteTestToken},
		"other scheme":   {BrowserURL: "tcp://browser.example.internal:3000?token=" + remoteTestToken},
		"UserDataDir":    {BrowserURL: good, UserDataDir: "/tmp/profile"},
		"NoSandbox":      {BrowserURL: good, NoSandbox: true},
		"Headless=false": {BrowserURL: good, Headless: &headed},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			c := remoteTestClient(t, "", &logs)
			err := c.AuthenticateWithOptions(context.Background(), opts)
			if !errors.Is(err, ErrInvalidAuthOptions) {
				t.Fatalf("error is not ErrInvalidAuthOptions: %v", err)
			}
			// Not "not submitted": a caller that retries on that
			// sentinel must not retry options that can never work.
			if errors.Is(err, ErrCredentialsNotSubmitted) || errors.Is(err, ErrCredentialsSubmitted) {
				t.Errorf("an options error matches a submission sentinel: %v", err)
			}
			if errors.Is(err, ErrBrowserConnection) {
				t.Errorf("an options error must not look like a connection failure: %v", err)
			}
			if !IsAuthError(err) {
				t.Errorf("IsAuthError = false for %v", err)
			}
			assertRemoteErrorIsClean(t, err, &logs, remoteTestToken, remoteTestPassword)
		})
	}
}

// A headless true pointer is what callers already pass; it must not be
// mistaken for a conflict.
func TestAuthenticateWithOptions_BrowserURL_HeadlessTrueIsNotAConflict(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	headless := true
	var logs bytes.Buffer
	c := remoteTestClient(t, "", &logs)
	err = c.AuthenticateWithOptions(context.Background(), &AuthOptions{
		BrowserURL: "ws://" + addr,
		Headless:   &headless,
		Timeout:    5 * time.Second,
	})
	if errors.Is(err, ErrInvalidAuthOptions) {
		t.Errorf("Headless=true was treated as a conflict: %v", err)
	}
	if !errors.Is(err, ErrBrowserConnection) {
		t.Errorf("error is not ErrBrowserConnection: %v", err)
	}
}

const remoteTestLoginPage = `<!doctype html><html><head><title>Sign In</title></head><body>
%ALERT%
<form id="LoginForm" action="/guardian/home.html" method="post">
  <input type="text" id="fieldAccount" name="account">
  <input type="password" id="fieldPassword" name="pw">
  <button type="submit" id="btn-enter-sign-in">Sign In</button>
</form>
</body></html>`

// remoteTestPortal is a local stand-in for the sign-in page. It accepts the
// invented credentials and rejects anything else, counting submissions.
func remoteTestPortal(t *testing.T, posts *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/public/":
			_, _ = w.Write([]byte(strings.Replace(remoteTestLoginPage, "%ALERT%", "", 1)))
		case r.Method == http.MethodPost && r.URL.Path == "/guardian/home.html":
			posts.Add(1)
			_ = r.ParseForm()
			if r.PostForm.Get("account") != remoteTestUser || r.PostForm.Get("pw") != remoteTestPassword {
				_, _ = w.Write([]byte(strings.Replace(remoteTestLoginPage, "%ALERT%",
					`<div class="feedback-alert">Invalid Username or Password!</div>`, 1)))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "session-value-invented", Path: "/"})
			http.Redirect(w, r, "/guardian/home.html", http.StatusSeeOther)
		case r.URL.Path == "/guardian/home.html":
			_, _ = w.Write([]byte(`<!doctype html><html><head><title>Home</title></head><body>signed in</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// These two run the whole public path against a real headless Chrome
// started locally with its DevTools port open. Skipped without a browser.

func TestAuthenticateWithOptions_BrowserURL_RealBrowser_Accepted(t *testing.T) {
	wsURL, _ := cdptest.StartChrome(t)
	var posts atomic.Int32
	portal := remoteTestPortal(t, &posts)

	var logs bytes.Buffer
	c := remoteTestClient(t, portal.URL, &logs)
	err := c.AuthenticateWithOptions(context.Background(), &AuthOptions{
		BrowserURL: wsURL + "?token=" + remoteTestToken,
		Timeout:    60 * time.Second,
	})
	if err != nil {
		t.Fatalf("AuthenticateWithOptions: %v", err)
	}
	if !c.IsAuthenticated() {
		t.Fatalf("the client is not authenticated after a successful sign-in")
	}
	found := false
	for _, ck := range c.ExportSession().Cookies {
		if ck.Name == "session" && ck.Value == "session-value-invented" {
			found = true
		}
	}
	if !found {
		t.Errorf("the session cookie is not in the client's session")
	}
	if n := posts.Load(); n != 1 {
		t.Errorf("the form was submitted %d times, want exactly 1", n)
	}
	for _, secret := range []string{remoteTestToken, remoteTestPassword, "session-value-invented"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("secret %q was logged", secret)
		}
	}
	if strings.Contains(logs.String(), "did not confirm disposal") {
		t.Errorf("the disposal of the browser context was not confirmed: %s", logs.String())
	}
	if c.BrowserContextDisposalUnconfirmed() {
		t.Errorf("BrowserContextDisposalUnconfirmed = true after a confirmed disposal")
	}
}

// A sign-in that succeeds while the remote browser never confirms the
// disposal of its browser context still succeeds; the caller can tell, and
// not only from a log line.
func TestAuthenticateWithOptions_BrowserURL_RealBrowser_UnconfirmedDisposal(t *testing.T) {
	wsURL, _ := cdptest.StartChrome(t)
	proxy := cdptest.NewLossyProxy(t, wsURL, func(method string, _ json.RawMessage) bool {
		return method == "Target.disposeBrowserContext"
	})
	var posts atomic.Int32
	portal := remoteTestPortal(t, &posts)

	var logs bytes.Buffer
	c := remoteTestClient(t, portal.URL, &logs)
	err := c.AuthenticateWithOptions(context.Background(), &AuthOptions{
		BrowserURL: proxy.URL + "/?token=" + remoteTestToken,
		Timeout:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("AuthenticateWithOptions: %v", err)
	}
	if !c.IsAuthenticated() {
		t.Errorf("the client is not authenticated")
	}
	if !c.BrowserContextDisposalUnconfirmed() {
		t.Errorf("BrowserContextDisposalUnconfirmed = false although the browser never confirmed")
	}

	// The next attempt resets it, whatever its own outcome.
	_ = c.AuthenticateWithOptions(context.Background(), &AuthOptions{BrowserURL: "http://not-a-websocket.invalid"})
	if c.BrowserContextDisposalUnconfirmed() {
		t.Errorf("BrowserContextDisposalUnconfirmed was not reset by the next sign-in attempt")
	}
}

// A credential with a line break in it (a secret read from a file with a
// trailing newline) is refused before any browser is started or reached,
// in both modes, and is not something to retry.
func TestAuthenticateWithOptions_CredentialWithNewline(t *testing.T) {
	for name, opts := range map[string]*AuthOptions{
		"local":  {Timeout: 30 * time.Second},
		"remote": {BrowserURL: "ws://browser.example.internal:3000?token=" + remoteTestToken, Timeout: 30 * time.Second},
	} {
		for _, field := range []string{"username", "password"} {
			t.Run(name+"/"+field, func(t *testing.T) {
				user, pw := remoteTestUser, remoteTestPassword
				if field == "username" {
					user += "\n"
				} else {
					pw += "\r\n"
				}
				var logs bytes.Buffer
				c, err := NewClient("portal.invalid", WithCredentials(user, pw), WithLogger(NewLogger(LogLevelDebug, &logs)))
				if err != nil {
					t.Fatalf("NewClient: %v", err)
				}
				start := time.Now()
				err = c.AuthenticateWithOptions(context.Background(), opts)
				if time.Since(start) > 2*time.Second {
					t.Errorf("took %v: browser work was started", time.Since(start))
				}
				if !errors.Is(err, ErrInvalidAuthOptions) {
					t.Fatalf("error is not ErrInvalidAuthOptions: %v", err)
				}
				if errors.Is(err, ErrCredentialsNotSubmitted) || errors.Is(err, ErrCredentialsSubmitted) {
					t.Errorf("an options error matches a submission sentinel: %v", err)
				}
				if !IsAuthError(err) {
					t.Errorf("IsAuthError = false for %v", err)
				}
				assertRemoteErrorIsClean(t, err, &logs, remoteTestToken, remoteTestPassword, remoteTestUser)
			})
		}
	}
}

// The origin sentinel is the internal one, so errors.Is works through the
// public name.
func TestErrUnexpectedOrigin_IsExported(t *testing.T) {
	wrapped := WrapAuthError(fmt.Errorf("authentication failed: %w", ErrUnexpectedOrigin))
	if !errors.Is(wrapped, ErrUnexpectedOrigin) || !IsAuthError(wrapped) {
		t.Errorf("ErrUnexpectedOrigin is not matched through an auth error: %v", wrapped)
	}
}

func TestAuthenticateWithOptions_BrowserURL_RealBrowser_Rejected(t *testing.T) {
	wsURL, _ := cdptest.StartChrome(t)
	var posts atomic.Int32
	portal := remoteTestPortal(t, &posts)

	var logs bytes.Buffer
	c, err := NewClient("portal.invalid",
		WithCredentials(remoteTestUser, "a-wrong-password-example"),
		WithLogger(NewLogger(LogLevelDebug, &logs)),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = portal.URL

	err = c.AuthenticateWithOptions(context.Background(), &AuthOptions{
		BrowserURL: wsURL + "?token=" + remoteTestToken,
		Timeout:    60 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected a rejection")
	}
	if !IsAuthError(err) {
		t.Errorf("IsAuthError = false for %v", err)
	}
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("error is not ErrInvalidCredentials: %v", err)
	}
	var pe *Error
	if !errors.As(err, &pe) || pe.Code != CodeInvalidCredentials {
		t.Errorf("error code is not CodeInvalidCredentials: %v", err)
	}
	if !errors.Is(err, ErrCredentialsSubmitted) || errors.Is(err, ErrCredentialsNotSubmitted) {
		t.Errorf("error does not say the credentials were submitted: %v", err)
	}
	if n := posts.Load(); n != 1 {
		t.Errorf("a rejected sign-in was submitted %d times, want exactly 1", n)
	}
	if c.IsAuthenticated() {
		t.Errorf("the client is authenticated after a rejected sign-in")
	}
	assertRemoteErrorIsClean(t, err, &logs, remoteTestToken, "a-wrong-password-example", "Invalid Username")
}
