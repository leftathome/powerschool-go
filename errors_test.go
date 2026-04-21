package powerschool

import (
	"errors"
	"testing"
)

func TestError(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name: "error with wrapped error",
			err: &Error{
				Code:    CodeAuthFailed,
				Message: "authentication failed",
				Err:     errors.New("invalid password"),
			},
			expected: "[AUTH_FAILED] authentication failed: invalid password",
		},
		{
			name: "error without wrapped error",
			err: &Error{
				Code:    CodeNotFound,
				Message: "resource not found",
			},
			expected: "[NOT_FOUND] resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &Error{
		Code:    CodeNetworkError,
		Message: "network failed",
		Err:     innerErr,
	}

	if got := err.Unwrap(); got != innerErr {
		t.Errorf("Error.Unwrap() = %v, want %v", got, innerErr)
	}
}

func TestNewError(t *testing.T) {
	code := CodeParseError
	message := "parsing failed"
	innerErr := errors.New("invalid HTML")

	err := NewError(code, message, innerErr)

	if err.Code != code {
		t.Errorf("NewError() Code = %v, want %v", err.Code, code)
	}
	if err.Message != message {
		t.Errorf("NewError() Message = %v, want %v", err.Message, message)
	}
	if err.Err != innerErr {
		t.Errorf("NewError() Err = %v, want %v", err.Err, innerErr)
	}
}

func TestWrapAuthError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "non-nil error",
			err:     errors.New("auth failed"),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapAuthError(tt.err)
			if (got == nil) != tt.wantNil {
				t.Errorf("WrapAuthError() = %v, wantNil %v", got, tt.wantNil)
			}
			if !tt.wantNil {
				var psErr *Error
				if !errors.As(got, &psErr) {
					t.Error("WrapAuthError() did not return *Error type")
				}
				if psErr.Code != CodeAuthFailed {
					t.Errorf("WrapAuthError() Code = %v, want %v", psErr.Code, CodeAuthFailed)
				}
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "auth error",
			err:  WrapAuthError(errors.New("test")),
			want: true,
		},
		{
			name: "invalid credentials error",
			err:  ErrInvalidCredentials,
			want: true,
		},
		{
			name: "other error",
			err:  ErrNotFound,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.want {
				t.Errorf("IsAuthError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSessionExpired(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "session expired error",
			err:  ErrSessionExpired,
			want: true,
		},
		{
			name: "wrapped session expired",
			err:  NewError(CodeSessionExpired, "session expired", nil),
			want: true,
		},
		{
			name: "other error",
			err:  ErrNotFound,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSessionExpired(tt.err); got != tt.want {
				t.Errorf("IsSessionExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "not found error",
			err:  ErrNotFound,
			want: true,
		},
		{
			name: "wrapped not found",
			err:  NewError(CodeNotFound, "not found", nil),
			want: true,
		},
		{
			name: "other error",
			err:  ErrAuthFailed,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
