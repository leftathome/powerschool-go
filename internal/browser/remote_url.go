package browser

import (
	"net/url"
	"sort"
	"strings"
)

// redactedMark replaces anything that could be part of the endpoint's secret.
const redactedMark = "[redacted]"

// unparseableEndpoint stands in for an endpoint that could not be parsed.
const unparseableEndpoint = "[unparseable browser URL]"

// redactor knows one remote-browser endpoint: how to show it (scheme, host
// and port only) and which strings must never leave this package.
type redactor struct {
	display string
	secrets []string
}

// newRedactor builds the redactor for an endpoint. It never fails: an
// endpoint that does not parse is shown as a fixed placeholder and scrubbed
// as a whole.
func newRedactor(raw string) *redactor {
	r := &redactor{display: unparseableEndpoint}
	add := func(vals ...string) {
		for _, v := range vals {
			if v != "" {
				r.secrets = append(r.secrets, v)
			}
		}
	}
	add(raw)
	// Whatever follows the first '?' or '#' or precedes an '@' is secret
	// even when the rest does not parse.
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		tail := raw[i+1:]
		add(tail)
		for _, pair := range strings.FieldsFunc(tail, func(c rune) bool { return strings.ContainsRune("&;?#", c) }) {
			add(pair)
			if _, v, ok := strings.Cut(pair, "="); ok {
				add(v)
			}
		}
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			add(rest[:at])
			add(strings.Split(rest[:at], ":")...)
		}
	}

	if u, err := url.Parse(raw); err == nil {
		if u.Scheme != "" && u.Host != "" {
			r.display = strings.ToLower(u.Scheme) + "://" + u.Host
		}
		add(u.String(), u.RequestURI(), u.RawQuery, u.Fragment, u.RawFragment)
		if len(u.Path) > 1 {
			add(u.Path, u.EscapedPath())
		}
		if u.User != nil {
			pw, _ := u.User.Password()
			add(u.User.String(), u.User.Username(), pw)
		}
		for key, vals := range u.Query() {
			for _, v := range vals {
				add(v, url.QueryEscape(v), url.PathEscape(v), key+"="+v)
			}
		}
		for _, pair := range strings.FieldsFunc(u.RawQuery, func(c rune) bool { return c == '&' || c == ';' }) {
			add(pair)
			if _, v, ok := strings.Cut(pair, "="); ok {
				add(v)
			}
		}
	}

	// Longest first, so a secret that contains another is replaced whole.
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
	return r
}

// scrub removes every secret of the endpoint from s.
func (r *redactor) scrub(s string) string {
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, redactedMark)
	}
	return s
}

// validateRemoteOptions checks the endpoint and the options that cannot be
// combined with it. Its errors never contain any part of the endpoint.
func validateRemoteOptions(opts *AuthenticateOptions) error {
	invalid := func(msg string) error {
		return &SignInError{Stage: StageOptions, msg: msg}
	}

	u, err := url.Parse(opts.BrowserURL)
	if err != nil {
		return invalid("the browser URL could not be parsed")
	}
	switch strings.ToLower(u.Scheme) {
	case "ws", "wss":
	case "http", "https":
		return invalid("the browser URL must be the browser's ws:// or wss:// DevTools WebSocket endpoint; http:// and https:// endpoints are not resolved")
	default:
		return invalid("the browser URL must use the ws:// or wss:// scheme")
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return invalid("the browser URL has no host")
	}

	// A remote browser is started by someone else, with its own profile
	// and sandbox. An option that only makes sense for a browser this
	// library starts is an error, so that it cannot be believed to apply.
	if opts.UserDataDir != "" {
		return invalid("UserDataDir cannot be combined with BrowserURL: the remote sign-in always runs in a fresh browser context that is disposed afterwards")
	}
	if opts.NoSandbox {
		return invalid("NoSandbox cannot be combined with BrowserURL: the sandbox of a remote browser is set where that browser is started")
	}
	if !opts.Headless {
		return invalid("Headless=false cannot be combined with BrowserURL: a remote browser cannot be shown")
	}
	return nil
}
