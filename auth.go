package powerschool

import (
	"context"
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
	}

	// Perform authentication
	result, err := browser.Authenticate(ctx, browserOpts)
	if err != nil {
		return WrapAuthError(err)
	}

	// Store session
	c.session = &Session{
		Cookies:   result.Cookies,
		ExpiresAt: result.ExpiresAt,
	}

	return nil
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
