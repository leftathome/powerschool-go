package powerschool

import (
	"net/http"
	"testing"
	"time"
)

func TestSessionIsValid(t *testing.T) {
	tests := []struct {
		name      string
		session   *Session
		wantValid bool
	}{
		{
			name:      "nil session",
			session:   nil,
			wantValid: false,
		},
		{
			name: "valid session",
			session: &Session{
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			wantValid: true,
		},
		{
			name: "expired session",
			session: &Session{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.IsValid(); got != tt.wantValid {
				t.Errorf("Session.IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestSessionGetCookies(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session_id", Value: "abc123"},
		{Name: "user_id", Value: "user456"},
	}

	session := &Session{
		Cookies:   cookies,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	got := session.GetCookies()
	if len(got) != len(cookies) {
		t.Errorf("GetCookies() returned %d cookies, want %d", len(got), len(cookies))
	}

	for i, cookie := range got {
		if cookie.Name != cookies[i].Name {
			t.Errorf("Cookie[%d].Name = %v, want %v", i, cookie.Name, cookies[i].Name)
		}
		if cookie.Value != cookies[i].Value {
			t.Errorf("Cookie[%d].Value = %v, want %v", i, cookie.Value, cookies[i].Value)
		}
	}
}

func TestSessionSetExpiry(t *testing.T) {
	session := &Session{
		ExpiresAt: time.Now(),
	}

	newExpiry := time.Now().Add(2 * time.Hour)
	session.SetExpiry(newExpiry)

	if !session.ExpiresAt.Equal(newExpiry) {
		t.Errorf("SetExpiry() expiry = %v, want %v", session.ExpiresAt, newExpiry)
	}
}

func TestAssignmentStatus(t *testing.T) {
	statuses := []AssignmentStatus{
		StatusPending,
		StatusSubmitted,
		StatusGraded,
		StatusLate,
		StatusMissing,
	}

	expected := []string{
		"pending",
		"submitted",
		"graded",
		"late",
		"missing",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("AssignmentStatus[%d] = %v, want %v", i, status, expected[i])
		}
	}
}

func TestAttendanceStatus(t *testing.T) {
	statuses := []AttendanceStatus{
		AttendancePresent,
		AttendanceAbsent,
		AttendanceTardy,
		AttendanceExcused,
	}

	expected := []string{
		"present",
		"absent",
		"tardy",
		"excused",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("AttendanceStatus[%d] = %v, want %v", i, status, expected[i])
		}
	}
}

func TestEventType(t *testing.T) {
	types := []EventType{
		EventHoliday,
		EventNoSchool,
		EventEarlyRelease,
		EventOther,
	}

	expected := []string{
		"holiday",
		"no_school",
		"early_release",
		"other",
	}

	for i, eventType := range types {
		if string(eventType) != expected[i] {
			t.Errorf("EventType[%d] = %v, want %v", i, eventType, expected[i])
		}
	}
}
