package browser

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// AuthResult contains the result of browser-based authentication
type AuthResult struct {
	Cookies   []*http.Cookie
	ExpiresAt time.Time
}

// AuthenticateOptions contains options for browser authentication
type AuthenticateOptions struct {
	BaseURL         string
	Username        string
	Password        string
	Headless        bool
	Timeout         time.Duration
	DebugLog        bool
	UserDataDir     string
	DisableGPU      bool
	NoSandbox       bool
	WindowSize      [2]int
	UserAgent       string
}

// DefaultAuthenticateOptions returns default authentication options
func DefaultAuthenticateOptions() *AuthenticateOptions {
	return &AuthenticateOptions{
		Headless:    true,
		Timeout:     60 * time.Second,
		DebugLog:    false,
		DisableGPU:  true,
		NoSandbox:   false,
		WindowSize:  [2]int{1920, 1080},
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}
}

// Authenticate performs browser-based authentication to PowerSchool
func Authenticate(ctx context.Context, opts *AuthenticateOptions) (*AuthResult, error) {
	if opts == nil {
		opts = DefaultAuthenticateOptions()
	}

	// Set up chromedp options
	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", opts.Headless),
		chromedp.Flag("disable-gpu", opts.DisableGPU),
		chromedp.Flag("no-sandbox", opts.NoSandbox),
		chromedp.WindowSize(opts.WindowSize[0], opts.WindowSize[1]),
		chromedp.UserAgent(opts.UserAgent),
		// Disable Chrome profile/sync prompts
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-features", "TranslateUI"),
		chromedp.Flag("disable-extensions", true),
	}

	if opts.UserDataDir != "" {
		allocOpts = append(allocOpts, chromedp.UserDataDir(opts.UserDataDir))
	}

	// Create allocator context
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()

	// Create browser context
	var contextOpts []chromedp.ContextOption
	if opts.DebugLog {
		contextOpts = append(contextOpts, chromedp.WithDebugf(func(format string, args ...interface{}) {
			fmt.Printf("[chromedp] "+format+"\n", args...)
		}))
	}

	browserCtx, browserCancel := chromedp.NewContext(allocCtx, contextOpts...)
	defer browserCancel()

	// Set timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, opts.Timeout)
	defer timeoutCancel()

	// Perform authentication
	var sessionCookies []*network.Cookie
	loginURL := opts.BaseURL + "/public/"

	err := chromedp.Run(timeoutCtx,
		// Navigate to login page
		chromedp.Navigate(loginURL),

		// Wait for page to load
		chromedp.Sleep(1*time.Second),

		// Debug: Print page title and URL
		chromedp.ActionFunc(func(ctx context.Context) error {
			if opts.DebugLog {
				var title, url string
				chromedp.Title(&title).Do(ctx)
				chromedp.Location(&url).Do(ctx)
				fmt.Printf("[chromedp] Page loaded - Title: %s, URL: %s\n", title, url)

				// Also print form fields found
				var html string
				chromedp.OuterHTML("form", &html, chromedp.ByQuery).Do(ctx)
				if html != "" {
					fmt.Printf("[chromedp] Login form HTML (first 500 chars):\n%s\n", truncate(html, 500))
				}
			}
			return nil
		}),

		// Try multiple possible selectors for username field
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Try common username field selectors
			selectors := []string{
				`input[name="account"]`,
				`input[name="username"]`,
				`input[id="fieldAccount"]`,
				`input[type="text"]`,
			}

			for _, sel := range selectors {
				var nodes []*cdp.Node
				if err := chromedp.Nodes(sel, &nodes, chromedp.ByQuery).Do(ctx); err == nil && len(nodes) > 0 {
					if opts.DebugLog {
						fmt.Printf("[chromedp] Found username field with selector: %s\n", sel)
					}
					return chromedp.SendKeys(sel, opts.Username, chromedp.ByQuery).Do(ctx)
				}
			}
			return fmt.Errorf("could not find username input field")
		}),

		// Try multiple possible selectors for password field
		chromedp.ActionFunc(func(ctx context.Context) error {
			selectors := []string{
				`input[name="pw"]`,
				`input[name="password"]`,
				`input[id="fieldPassword"]`,
				`input[type="password"]`,
			}

			for _, sel := range selectors {
				var nodes []*cdp.Node
				if err := chromedp.Nodes(sel, &nodes, chromedp.ByQuery).Do(ctx); err == nil && len(nodes) > 0 {
					if opts.DebugLog {
						fmt.Printf("[chromedp] Found password field with selector: %s\n", sel)
					}
					return chromedp.SendKeys(sel, opts.Password, chromedp.ByQuery).Do(ctx)
				}
			}
			return fmt.Errorf("could not find password input field")
		}),

		// Wait a moment for fields to be filled
		chromedp.Sleep(500*time.Millisecond),

		// Try multiple possible selectors for submit button
		chromedp.ActionFunc(func(ctx context.Context) error {
			selectors := []string{
				// Reference-instance selector (PowerSchool's own login button id)
				`#btn-enter-sign-in`,
				`#btn-enter`,
				// Common patterns
				`button[type="submit"]`,
				`input[type="submit"]`,
				`button.button-login`,
				`input.button-login`,
				`button[id*="sign-in"]`,
				`button[id*="login"]`,
				`form#LoginForm button`,
				`fieldset#login-inputs button`,
			}

			if opts.DebugLog {
				fmt.Printf("[chromedp] Searching for submit button...\n")
			}

			for _, sel := range selectors {
				var nodes []*cdp.Node
				if err := chromedp.Nodes(sel, &nodes, chromedp.ByQuery).Do(ctx); err == nil && len(nodes) > 0 {
					if opts.DebugLog {
						fmt.Printf("[chromedp] Found submit button with selector: %s (found %d nodes)\n", sel, len(nodes))
					}
					// Try clicking
					if err := chromedp.Click(sel, chromedp.NodeVisible, chromedp.ByQuery).Do(ctx); err != nil {
						if opts.DebugLog {
							fmt.Printf("[chromedp] Failed to click button: %v, trying next...\n", err)
						}
						continue
					}
					if opts.DebugLog {
						fmt.Printf("[chromedp] Successfully clicked submit button!\n")
					}
					return nil
				} else if opts.DebugLog && err != nil {
					fmt.Printf("[chromedp] Selector '%s' error: %v\n", sel, err)
				}
			}

			// If no button found, try submitting the form via Enter key
			if opts.DebugLog {
				fmt.Printf("[chromedp] No submit button found after trying all selectors, trying Enter key on password field\n")
			}
			return chromedp.SendKeys(`input[type="password"]`, "\n", chromedp.ByQuery).Do(ctx)
		}),

		// Wait for navigation to complete
		chromedp.Sleep(3*time.Second),

		// Check if we successfully logged in
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Check for error message
			var errorExists bool
			err := chromedp.Evaluate(`document.querySelector('.feedback-alert') !== null`, &errorExists).Do(ctx)
			if err == nil && errorExists {
				return fmt.Errorf("login failed: invalid credentials")
			}

			// If no error, we should be on the dashboard or home page
			// Extract all cookies using the correct API
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to get cookies: %w", err)
			}

			sessionCookies = cookies
			return nil
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Convert network cookies to http.Cookie
	httpCookies := make([]*http.Cookie, 0, len(sessionCookies))
	var maxExpires time.Time

	for _, nc := range sessionCookies {
		// Only include session-related cookies
		if nc.Domain == "" || nc.Name == "" {
			continue
		}

		expires := time.Unix(int64(nc.Expires), 0)
		if expires.After(maxExpires) {
			maxExpires = expires
		}

		httpCookie := &http.Cookie{
			Name:     nc.Name,
			Value:    nc.Value,
			Path:     nc.Path,
			Domain:   nc.Domain,
			Expires:  expires,
			Secure:   nc.Secure,
			HttpOnly: nc.HTTPOnly,
			SameSite: convertSameSite(nc.SameSite),
		}

		httpCookies = append(httpCookies, httpCookie)
	}

	// If no expiry found, set default to 4 hours
	if maxExpires.IsZero() || maxExpires.Before(time.Now()) {
		maxExpires = time.Now().Add(4 * time.Hour)
	}

	return &AuthResult{
		Cookies:   httpCookies,
		ExpiresAt: maxExpires,
	}, nil
}

// convertSameSite converts chromedp cookie SameSite to http.Cookie SameSite
func convertSameSite(sameSite network.CookieSameSite) http.SameSite {
	switch sameSite {
	case network.CookieSameSiteStrict:
		return http.SameSiteStrictMode
	case network.CookieSameSiteLax:
		return http.SameSiteLaxMode
	case network.CookieSameSiteNone:
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}

// GetPageContent retrieves page content using browser automation
// This is useful for scraping pages that require JavaScript
func GetPageContent(ctx context.Context, url string, cookies []*http.Cookie, timeout time.Duration) (string, error) {
	return GetPageContentWithWait(ctx, url, cookies, timeout, 2*time.Second)
}

// GetPageContentWithWait retrieves page content using browser automation with custom wait time
// waitTime specifies how long to wait for JavaScript to execute before extracting HTML
func GetPageContentWithWait(ctx context.Context, url string, cookies []*http.Cookie, timeout time.Duration, waitTime time.Duration) (string, error) {
	// Chrome refuses to start as root without --no-sandbox (WSL/containers),
	// so enable it when needed. The flag is safe on other platforms too.
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", needsNoSandbox()),
	)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, timeout)
	defer timeoutCancel()

	// Convert http.Cookie to chromedp network.CookieParam
	var networkCookies []*network.CookieParam
	for _, c := range cookies {
		networkCookies = append(networkCookies, &network.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HttpOnly,
			SameSite: convertHTTPSameSiteToNetwork(c.SameSite),
		})
	}

	var content string
	err := chromedp.Run(timeoutCtx,
		// Set cookies
		network.SetCookies(networkCookies),

		// Navigate to page
		chromedp.Navigate(url),

		// Wait for page to load (configurable wait time for JavaScript execution)
		chromedp.Sleep(waitTime),

		// Get HTML content
		chromedp.ActionFunc(func(ctx context.Context) error {
			var html string
			err := chromedp.Evaluate(`document.documentElement.outerHTML`, &html).Do(ctx)
			if err != nil {
				return err
			}
			content = html
			return nil
		}),
	)

	if err != nil {
		return "", fmt.Errorf("failed to get page content: %w", err)
	}

	return content, nil
}

// convertHTTPSameSiteToNetwork converts http.Cookie SameSite to network.CookieSameSite
func convertHTTPSameSiteToNetwork(sameSite http.SameSite) network.CookieSameSite {
	switch sameSite {
	case http.SameSiteStrictMode:
		return network.CookieSameSiteStrict
	case http.SameSiteLaxMode:
		return network.CookieSameSiteLax
	case http.SameSiteNoneMode:
		return network.CookieSameSiteNone
	default:
		return network.CookieSameSiteLax
	}
}

// needsNoSandbox reports whether Chrome needs --no-sandbox to start. Only
// Linux/WSL running as root hits this; other platforms just return false and
// get the default sandbox behavior.
func needsNoSandbox() bool {
	return runtime.GOOS == "linux" && os.Geteuid() == 0
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
