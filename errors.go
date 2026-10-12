package powerschool

import (
	"errors"
	"fmt"

	"github.com/leftathome/powerschool-go/internal/browser"
)

// Sentinel errors
var (
	// ErrAuthFailed indicates authentication failed
	ErrAuthFailed = errors.New("authentication failed")

	// ErrSessionExpired indicates the session has expired
	ErrSessionExpired = errors.New("session expired")

	// ErrNotFound indicates the requested resource was not found
	ErrNotFound = errors.New("not found")

	// ErrRateLimited indicates rate limiting is in effect
	ErrRateLimited = errors.New("rate limited")

	// ErrInvalidCredentials indicates invalid username or password
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrNoCredentials indicates no credentials were provided
	ErrNoCredentials = errors.New("no credentials provided")

	// ErrInvalidBaseURL indicates the base URL is invalid
	ErrInvalidBaseURL = errors.New("invalid base URL")

	// ErrParseError indicates an error parsing HTML or data
	ErrParseError = errors.New("parse error")

	// ErrNetworkError indicates a network error occurred
	ErrNetworkError = errors.New("network error")

	// ErrSelectionUnknown indicates the page does not explicitly mark which
	// student the session has selected, so its contents cannot be
	// attributed to a student.
	ErrSelectionUnknown = errors.New("page does not mark which student is selected")

	// ErrStudentNotSelected indicates the session has a different student
	// selected than the one asked for.
	ErrStudentNotSelected = errors.New("the requested student is not the one the session has selected")

	// ErrSwitchNotConfirmed indicates the student-switch form was posted
	// but the page read afterwards does not mark the requested student as
	// selected.
	ErrSwitchNotConfirmed = errors.New("student switch was not confirmed by the page")
)

// Sentinel errors of the browser sign-in, reported through errors.Is
// alongside the usual CodeAuthFailed / CodeInvalidCredentials
// classification.
//
// ErrInvalidAuthOptions and ErrUnexpectedOrigin are reported by both the
// local sign-in and the one through a remote browser
// (AuthOptions.BrowserURL). The submission sentinels and
// ErrBrowserConnection describe a remote sign-in. Apart from the two shared
// checks, a failed LOCAL sign-in matches neither ErrCredentialsNotSubmitted
// nor ErrCredentialsSubmitted: treat it as "outcome unknown", like
// ErrCredentialsSubmitted.
var (
	// ErrInvalidAuthOptions indicates input that can never work: options
	// that cannot be used together with BrowserURL, a BrowserURL that is
	// not a ws:// or wss:// URL, or a username or password containing a
	// line break, a tab or another character that cannot be typed as
	// text (the value is not trimmed for you). Nothing was sent anywhere,
	// and trying again unchanged cannot succeed: it deliberately does NOT
	// match ErrCredentialsNotSubmitted.
	ErrInvalidAuthOptions = browser.ErrInvalidOptions

	// ErrUnexpectedOrigin indicates the page the credentials were about
	// to be typed into, or submitted from, was not on the origin (scheme,
	// host and port) of the client's base URL: for example the sign-in
	// URL redirected to another host. The credentials are not typed into
	// such a page. Check ErrCredentialsSubmitted to learn whether the
	// password was already in the page when this was noticed.
	ErrUnexpectedOrigin = browser.ErrOrigin

	// ErrBrowserConnection indicates the remote browser could not be
	// reached, or the connection to it was lost during the sign-in. Check
	// ErrCredentialsSubmitted to learn whether the form had been submitted.
	ErrBrowserConnection = browser.ErrBrowserConnection

	// ErrCredentialsNotSubmitted indicates the sign-in stopped before the
	// password was typed into the page: the form cannot have been
	// submitted, the portal has not seen this attempt, and trying again
	// does not count against the account.
	ErrCredentialsNotSubmitted = browser.ErrNotSubmitted

	// ErrCredentialsSubmitted indicates the sign-in stopped at some point
	// after the password had started to be typed into the page. The line
	// is drawn there, not at the click, because a page can submit itself
	// while it is being filled. The outcome is unknown: the portal may
	// have accepted the sign-in (ending any other session of the account),
	// rejected it (counting towards a lockout), or never seen it. Do not
	// retry blindly. Together with ErrInvalidCredentials it means the
	// portal answered with its sign-in error.
	ErrCredentialsSubmitted = browser.ErrSubmitted
)

// HTTPStatusError reports an unexpected HTTP status. It carries the status
// and the size of the response body, never the body itself or the request's
// query string: both can identify a student.
type HTTPStatusError struct {
	// Op names the request, e.g. "GET /guardian/home.html" (no query).
	Op string
	// StatusCode is the HTTP status received.
	StatusCode int
	// BodyLen is the number of body bytes read (capped), for diagnosis.
	BodyLen int
}

// Error implements the error interface.
func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s: unexpected HTTP status %d (%d-byte body)", e.Op, e.StatusCode, e.BodyLen)
}

// errLoginPage is the typed session error for "the portal answered with its
// login page". what names the page that was asked for, without its query.
func errLoginPage(what string) error {
	return NewError(CodeSessionExpired,
		"the portal returned its login page in place of the "+what+
			" (session expired or rejected, or the request lacked required parameters)",
		ErrSessionExpired)
}

// ErrorCode represents an error code
type ErrorCode string

const (
	// CodeAuthFailed indicates authentication failure
	CodeAuthFailed ErrorCode = "AUTH_FAILED"

	// CodeSessionExpired indicates session expiration
	CodeSessionExpired ErrorCode = "SESSION_EXPIRED"

	// CodeNotFound indicates resource not found
	CodeNotFound ErrorCode = "NOT_FOUND"

	// CodeRateLimited indicates rate limiting
	CodeRateLimited ErrorCode = "RATE_LIMITED"

	// CodeInvalidCredentials indicates invalid credentials
	CodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS"

	// CodeParseError indicates parsing error
	CodeParseError ErrorCode = "PARSE_ERROR"

	// CodeNetworkError indicates network error
	CodeNetworkError ErrorCode = "NETWORK_ERROR"

	// CodeUnknown indicates an unknown error
	CodeUnknown ErrorCode = "UNKNOWN"
)

// Error represents a PowerSchool API error
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
	return e.Err
}

// NewError creates a new Error with the given code and message
func NewError(code ErrorCode, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WrapAuthError wraps an authentication error
func WrapAuthError(err error) error {
	if err == nil {
		return nil
	}
	return NewError(CodeAuthFailed, "authentication failed", err)
}

// WrapParseError wraps a parsing error
func WrapParseError(err error, context string) error {
	if err == nil {
		return nil
	}
	return NewError(CodeParseError, fmt.Sprintf("failed to parse %s", context), err)
}

// WrapNetworkError wraps a network error
func WrapNetworkError(err error) error {
	if err == nil {
		return nil
	}
	return NewError(CodeNetworkError, "network request failed", err)
}

// IsAuthError checks if the error is an authentication error
func IsAuthError(err error) bool {
	var psErr *Error
	if errors.As(err, &psErr) {
		return psErr.Code == CodeAuthFailed || psErr.Code == CodeInvalidCredentials
	}
	return errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrInvalidCredentials)
}

// IsSessionExpired checks if the error is a session expiration error
func IsSessionExpired(err error) bool {
	var psErr *Error
	if errors.As(err, &psErr) {
		return psErr.Code == CodeSessionExpired
	}
	return errors.Is(err, ErrSessionExpired)
}

// IsNotFound checks if the error is a not found error
func IsNotFound(err error) bool {
	var psErr *Error
	if errors.As(err, &psErr) {
		return psErr.Code == CodeNotFound
	}
	return errors.Is(err, ErrNotFound)
}
