package powerschool

import (
	"errors"
	"fmt"
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
)

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
