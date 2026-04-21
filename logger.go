package powerschool

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// LogLevelNone disables all logging
	LogLevelNone LogLevel = iota
	// LogLevelError logs only errors
	LogLevelError
	// LogLevelWarn logs warnings and errors
	LogLevelWarn
	// LogLevelInfo logs info, warnings, and errors
	LogLevelInfo
	// LogLevelDebug logs everything including debug information
	LogLevelDebug
)

// Logger handles logging for the PowerSchool client
type Logger struct {
	level  LogLevel
	logger *log.Logger
}

// NewLogger creates a new logger with the specified level
func NewLogger(level LogLevel, output io.Writer) *Logger {
	if output == nil {
		output = os.Stderr
	}
	return &Logger{
		level:  level,
		logger: log.New(output, "[powerschool] ", log.LstdFlags),
	}
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level >= LogLevelDebug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level >= LogLevelInfo {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level >= LogLevelWarn {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level >= LogLevelError {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// DebugRequest logs HTTP request details (only in debug mode)
func (l *Logger) DebugRequest(method, url string, headers map[string]string) {
	if l.level >= LogLevelDebug {
		l.logger.Printf("[DEBUG] Request: %s %s", method, url)
		if headers != nil && len(headers) > 0 {
			l.logger.Printf("[DEBUG] Headers: %v", sanitizeHeaders(headers))
		}
	}
}

// DebugResponse logs HTTP response details (only in debug mode)
func (l *Logger) DebugResponse(statusCode int, bodySnippet string) {
	if l.level >= LogLevelDebug {
		l.logger.Printf("[DEBUG] Response: Status %d", statusCode)
		if bodySnippet != "" {
			// Truncate body snippet to avoid huge logs
			if len(bodySnippet) > 500 {
				bodySnippet = bodySnippet[:500] + "... (truncated)"
			}
			l.logger.Printf("[DEBUG] Body snippet: %s", bodySnippet)
		}
	}
}

// DebugHTML logs HTML content (only in debug mode, with truncation)
func (l *Logger) DebugHTML(context string, html string) {
	if l.level >= LogLevelDebug {
		l.logger.Printf("[DEBUG] HTML for %s (%d bytes)", context, len(html))
		// Optionally log a snippet
		if len(html) > 1000 {
			l.logger.Printf("[DEBUG] HTML snippet: %s... (truncated)", html[:1000])
		} else if len(html) > 0 {
			l.logger.Printf("[DEBUG] HTML: %s", html)
		}
	}
}

// sanitizeHeaders removes sensitive headers from logging
func sanitizeHeaders(headers map[string]string) map[string]string {
	sanitized := make(map[string]string)
	sensitiveHeaders := []string{"authorization", "cookie", "set-cookie", "x-csrf-token"}

	for key, value := range headers {
		lowerKey := strings.ToLower(key)
		isSensitive := false
		for _, sh := range sensitiveHeaders {
			if strings.Contains(lowerKey, sh) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			sanitized[key] = "[REDACTED]"
		} else {
			sanitized[key] = value
		}
	}

	return sanitized
}

// ParseLogLevel parses a string log level to LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	case "none":
		return LogLevelNone
	default:
		return LogLevelInfo
	}
}

// String returns the string representation of a LogLevel
func (l LogLevel) String() string {
	switch l {
	case LogLevelNone:
		return "none"
	case LogLevelError:
		return "error"
	case LogLevelWarn:
		return "warn"
	case LogLevelInfo:
		return "info"
	case LogLevelDebug:
		return "debug"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}
