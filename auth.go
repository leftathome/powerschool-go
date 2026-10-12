package powerschool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/leftathome/powerschool-go/internal/browser"
)

// Authenticate performs authentication with PowerSchool using stored credentials
// This method uses browser automation (chromedp) to handle the login flow
func (c *Client) Authenticate(ctx context.Context) error {
	return c.AuthenticateWithOptions(ctx, nil)
}

// AuthenticateWithOptions performs authentication with custom options
func (c *Client) AuthenticateWithOptions(ctx context.Context, opts *AuthOptions) error {
	if c.credentials == nil {
		return ErrNoCredentials
	}

	c.contextDisposalUnconfirmed = false

	// Build browser authentication options
	browserOpts := browser.DefaultAuthenticateOptions()
	browserOpts.BaseURL = c.baseURL
	browserOpts.Username = c.credentials.Username
	browserOpts.Password = c.credentials.Password

	// Apply custom options if provided
	if opts != nil {
		if opts.Headless != nil {
			browserOpts.Headless = *opts.Headless
		}
		if opts.Timeout > 0 {
			browserOpts.Timeout = opts.Timeout
		}
		if opts.DebugLog {
			browserOpts.DebugLog = true
		}
		if opts.UserDataDir != "" {
			browserOpts.UserDataDir = opts.UserDataDir
		}
		if opts.UserAgent != "" {
			browserOpts.UserAgent = opts.UserAgent
		}
		if opts.NoSandbox {
			browserOpts.NoSandbox = true
		}
		if opts.BrowserURL != "" {
			browserOpts.BrowserURL = opts.BrowserURL
		}
	}

	// Perform authentication
	result, err := browser.Authenticate(ctx, browserOpts)
	if err != nil {
		// Only a remote sign-in reports a rejection this way.
		if errors.Is(err, browser.ErrRejected) {
			return NewError(CodeInvalidCredentials, "authentication failed",
				fmt.Errorf("%w: %w", ErrInvalidCredentials, err))
		}
		return WrapAuthError(err)
	}
	if result.ContextDisposalUnconfirmed {
		c.contextDisposalUnconfirmed = true
		c.logger.Warn("remote browser did not confirm disposal of the sign-in's browser context; the connection was closed, which disposes it")
	}

	// Store session
	c.session = &Session{
		Cookies:   result.Cookies,
		ExpiresAt: result.ExpiresAt,
	}

	return nil
}

// BrowserContextDisposalUnconfirmed reports whether the most recent
// successful sign-in through a remote browser (AuthOptions.BrowserURL)
// ended without that browser confirming that it disposed the sign-in's
// browser context, the one that held the session cookies. The sign-in
// still succeeded and the connection was closed, which makes a browser
// dispose the context by itself; a caller that needs certainty can act on
// this (for example restart the browser service, or end the session and
// sign in again later). It is false after a local sign-in, after a failed
// one, and after a confirmed disposal. Every sign-in attempt resets it.
func (c *Client) BrowserContextDisposalUnconfirmed() bool {
	return c.contextDisposalUnconfirmed
}

// AuthOptions contains options for authentication
type AuthOptions struct {
	// Headless controls whether to run browser in headless mode
	// Default: true (headless)
	Headless *bool

	// Timeout for the authentication process
	// Default: 60 seconds
	Timeout time.Duration

	// DebugLog enables debug logging for browser automation
	// Default: false
	DebugLog bool

	// UserDataDir specifies a custom user data directory for Chrome
	// Default: temporary directory
	UserDataDir string

	// UserAgent specifies a custom user agent string
	// Default: Chrome user agent
	UserAgent string

	// NoSandbox passes --no-sandbox to Chrome. Required when running as root
	// (e.g. inside a container or WSL). Default: false.
	NoSandbox bool

	// BrowserURL is the DevTools WebSocket endpoint of an already-running
	// browser, for example a shared browser service:
	// "ws://browser.example.internal:3000?token=...". When set, the sign-in
	// is driven in that browser and no local Chrome is started; when
	// empty (the default), behaviour is unchanged.
	//
	// Only ws:// and wss:// are accepted, and the URL is dialled exactly as
	// given. It usually carries a token, so it is treated as a secret:
	// errors and logs show it as scheme://host[:port] and nothing more.
	//
	// The sign-in runs in a browser context of its own (never the remote
	// browser's default one) that is disposed, with its page, cookies and
	// storage, on every way out; the remote browser itself and anything
	// else in it are left alone. Timeout covers the dial, the sign-in and
	// that disposal. Still, the password is typed into, and the session
	// cookies briefly exist in, a browser the caller does not own: see
	// "Remote browser" in the README.
	//
	// UserDataDir, NoSandbox and Headless=false describe a browser this
	// library starts. Setting any of them together with BrowserURL is an
	// error (ErrInvalidAuthOptions), not silently ignored. UserAgent is
	// applied to the sign-in's page. DebugLog prints step names only.
	//
	// A failed remote sign-in says how far it got: errors.Is reports
	// ErrCredentialsNotSubmitted or ErrCredentialsSubmitted (see those),
	// and ErrBrowserConnection when the browser could not be reached or
	// went away. The form is submitted at most once per call, and success
	// is reported only on positive evidence that the browser left the
	// sign-in form. If the remote browser does not confirm the disposal
	// of the browser context, the sign-in still succeeds and
	// Client.BrowserContextDisposalUnconfirmed reports it.
	BrowserURL string
}

// RefreshSession attempts to refresh the current session
// Returns an error if no valid credentials are available
func (c *Client) RefreshSession(ctx context.Context) error {
	if c.credentials == nil {
		return NewError(CodeSessionExpired, "cannot refresh session without credentials", nil)
	}

	return c.Authenticate(ctx)
}

// ClearSession clears the current session
func (c *Client) ClearSession() {
	c.session = nil
}

// IsAuthenticated returns true if the client has a valid session
func (c *Client) IsAuthenticated() bool {
	return c.session != nil && c.session.IsValid()
}

// GetSessionExpiry returns the session expiration time
// Returns zero time if no session exists
func (c *Client) GetSessionExpiry() time.Time {
	if c.session == nil {
		return time.Time{}
	}
	c.session.mu.RLock()
	defer c.session.mu.RUnlock()
	return c.session.ExpiresAt
}

// ExportSession exports the current session for persistence
// Returns nil if no valid session exists
func (c *Client) ExportSession() *SessionExport {
	if c.session == nil || !c.session.IsValid() {
		return nil
	}

	return &SessionExport{
		Cookies:   c.session.GetCookies(),
		ExpiresAt: c.GetSessionExpiry(),
	}
}

// ImportSession imports a previously exported session
func (c *Client) ImportSession(export *SessionExport) error {
	if export == nil {
		return NewError(CodeUnknown, "cannot import nil session", nil)
	}

	if len(export.Cookies) == 0 {
		return NewError(CodeSessionExpired, "session has no cookies", nil)
	}

	// Check if session is expired
	if export.ExpiresAt.Before(time.Now()) {
		return NewError(CodeSessionExpired, "imported session is expired", nil)
	}

	c.session = &Session{
		Cookies:   export.Cookies,
		ExpiresAt: export.ExpiresAt,
	}

	return nil
}

// SessionExport represents an exported session
type SessionExport struct {
	Cookies   []*http.Cookie `json:"cookies"`
	ExpiresAt time.Time      `json:"expires_at"`
}
