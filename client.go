package powerschool

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the main PowerSchool client
type Client struct {
	baseURL     string
	credentials *Credentials
	session     *Session
	httpClient  *http.Client
	logger      *Logger
}

// Option configures the Client
type Option func(*Client) error

// NewClient creates a new PowerSchool client
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, ErrInvalidBaseURL
	}

	// Validate and normalize base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, NewError(CodeUnknown, "invalid base URL", err)
	}

	// Ensure HTTPS for security
	if parsedURL.Scheme == "http" {
		parsedURL.Scheme = "https"
	}

	// Remove trailing slash
	baseURL = strings.TrimSuffix(parsedURL.String(), "/")

	client := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: NewLogger(LogLevelInfo, nil), // Default to Info level
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, err
		}
	}

	return client, nil
}

// WithCredentials sets username/password authentication
func WithCredentials(username, password string) Option {
	return func(c *Client) error {
		if username == "" || password == "" {
			return ErrInvalidCredentials
		}
		c.credentials = &Credentials{
			Username: username,
			Password: password,
		}
		return nil
	}
}

// WithSession sets session cookie authentication
func WithSession(cookies []*http.Cookie, expiresAt time.Time) Option {
	return func(c *Client) error {
		if len(cookies) == 0 {
			return NewError(CodeSessionExpired, "no session cookies provided", nil)
		}
		c.session = &Session{
			Cookies:   cookies,
			ExpiresAt: expiresAt,
		}
		return nil
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) error {
		if httpClient == nil {
			return NewError(CodeUnknown, "http client cannot be nil", nil)
		}
		c.httpClient = httpClient
		return nil
	}
}

// WithTimeout sets a custom timeout for HTTP requests
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		if timeout <= 0 {
			return NewError(CodeUnknown, "timeout must be positive", nil)
		}
		c.httpClient.Timeout = timeout
		return nil
	}
}

// WithLogger sets a custom logger
func WithLogger(logger *Logger) Option {
	return func(c *Client) error {
		if logger == nil {
			return NewError(CodeUnknown, "logger cannot be nil", nil)
		}
		c.logger = logger
		return nil
	}
}

// WithLogLevel sets the logging level
func WithLogLevel(level LogLevel) Option {
	return func(c *Client) error {
		c.logger.SetLevel(level)
		return nil
	}
}

// ensureAuthenticated ensures the client has a valid session
func (c *Client) ensureAuthenticated(ctx context.Context) error {
	// Check if we have a valid session
	if c.session != nil && c.session.IsValid() {
		return nil
	}

	// If no credentials, we can't re-authenticate
	if c.credentials == nil {
		return ErrSessionExpired
	}

	// Re-authenticate
	return c.Authenticate(ctx)
}

// GetBaseURL returns the base URL of the PowerSchool instance
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// GetSession returns the current session (if any)
// Returns nil if no session exists or session is expired
func (c *Client) GetSession() *Session {
	if c.session != nil && c.session.IsValid() {
		return c.session
	}
	return nil
}

// HasCredentials returns true if the client has credentials configured
func (c *Client) HasCredentials() bool {
	return c.credentials != nil
}

// doRequest performs an HTTP request with the current session cookies
func (c *Client) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	return c.doRequestWithBody(ctx, method, path, nil, "")
}

// doRequestWithBody performs an HTTP request with body and session cookies
func (c *Client) doRequestWithBody(ctx context.Context, method, path string, body []byte, contentType string) (*http.Response, error) {
	return c.doRequestWithHeaders(ctx, method, path, body, contentType, nil)
}

// doRequestWithHeaders performs an HTTP request with body, session cookies, and custom headers
func (c *Client) doRequestWithHeaders(ctx context.Context, method, path string, body []byte, contentType string, headers map[string]string) (*http.Response, error) {
	// Ensure we're authenticated
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// Build full URL
	fullURL := c.baseURL + path

	// Create request
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, fullURL, strings.NewReader(string(body)))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, fullURL, nil)
	}
	if err != nil {
		return nil, WrapNetworkError(err)
	}

	// Add session cookies
	if c.session != nil {
		for _, cookie := range c.session.GetCookies() {
			req.AddCookie(cookie)
		}
	}

	// Set default headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
		// If sending JSON, expect JSON back and set CORS headers
		if contentType == "application/json" || contentType == "application/json;charset=UTF-8" {
			req.Header.Set("Accept", "application/json, text/plain, */*")
			req.Header.Set("Origin", c.baseURL)
		}
	}

	// Set custom headers (these override defaults)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err)
	}

	// Check for session expiration
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		c.session = nil
		return nil, ErrSessionExpired
	}

	return resp, nil
}

// buildURL builds a full URL from a path
func (c *Client) buildURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// String returns a string representation of the client
func (c *Client) String() string {
	sessionStatus := "no session"
	if c.session != nil {
		if c.session.IsValid() {
			sessionStatus = fmt.Sprintf("valid session (expires %s)", c.session.ExpiresAt.Format(time.RFC3339))
		} else {
			sessionStatus = "expired session"
		}
	}

	credStatus := "no credentials"
	if c.credentials != nil {
		credStatus = fmt.Sprintf("credentials for user %s", c.credentials.Username)
	}

	return fmt.Sprintf("PowerSchool Client{baseURL: %s, %s, %s}", c.baseURL, sessionStatus, credStatus)
}
