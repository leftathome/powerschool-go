package powerschool

import (
	"net/http"
	"sync"
	"time"
)

// Student represents a student in PowerSchool
type Student struct {
	ID             string // Internal PowerSchool ID
	StudentNumber  string // Official student ID number
	StateID        string // State ID number
	Name           string
	GradeLevel     int
	SchoolID       string
	SchoolName     string
	PortalUsername string // Student portal username/email
	SourceUsername string // Source username
}

// Grade represents a course grade
type Grade struct {
	CourseID         string    // FRN parameter from URL (e.g., "00111222333")
	SectionID        string    // Section DCID for API calls (e.g., "654321")
	// ScoresURL is the server-relative scores.html path captured from the
	// home-page grades table, including begdate/enddate/fg/schoolid params.
	// All four are required — hitting scores.html with only frn redirects to
	// the login page. See docs/ASSIGNMENTS_API_DEBUG.md.
	ScoresURL        string
	// StudentAPIID is the student DCID the assignment-lookup API expects in
	// its student_ids[] payload. It is *not* the switchStudent() nav ID on
	// Student.ID; the two can differ. GetAssignments populates this lazily
	// after its first call for this grade.
	StudentAPIID     string
	CourseName       string
	Section          string    // Course section
	Teacher          string
	TeacherEmail     string
	RoomNumber       string
	Period           string    // Expression (schedule slot/class period)
	Q1Grade          string    // Quarter 1 grade
	Q2Grade          string    // Quarter 2 grade
	S1Grade          string    // Semester 1 grade
	Q3Grade          string    // Quarter 3 grade
	Q4Grade          string    // Quarter 4 grade
	S2Grade          string    // Semester 2 grade
	CurrentGrade     string    // Most recent grade
	Percentage       float64   // Most recent percentage
	LetterGrade      string    // Most recent letter grade
	Absences         int       // Total absences for course
	Tardies          int       // Total tardies for course
	TeacherComments  string    // Teacher comments
	SectionDesc      string    // Section description
	LastUpdated      time.Time
}

// AssignmentStatus represents the status of an assignment
type AssignmentStatus string

const (
	StatusPending    AssignmentStatus = "pending"
	StatusSubmitted  AssignmentStatus = "submitted"
	StatusGraded     AssignmentStatus = "graded"
	StatusLate       AssignmentStatus = "late"
	StatusMissing    AssignmentStatus = "missing"
	StatusCollected  AssignmentStatus = "collected"
	StatusIncomplete AssignmentStatus = "incomplete"
	StatusAbsent     AssignmentStatus = "absent"
	StatusExempt     AssignmentStatus = "exempt"
)

// AssignmentFlag represents flags on assignments (Late, Missing, Collected, etc.)
type AssignmentFlag string

const (
	FlagCollected  AssignmentFlag = "collected"
	FlagLate       AssignmentFlag = "late"
	FlagMissing    AssignmentFlag = "missing"
	FlagExempt     AssignmentFlag = "exempt"
	FlagAbsent     AssignmentFlag = "absent"
	FlagIncomplete AssignmentFlag = "incomplete"
	FlagExcluded   AssignmentFlag = "excluded"
)

// Assignment represents an assignment
type Assignment struct {
	ID            string
	CourseID      string
	Title         string
	Description   string
	DueDate       time.Time
	Category      string
	Score         *float64         // Points earned
	MaxScore      float64          // Points possible
	WeightedScore *float64         // Weighted score if different
	WeightedMax   float64          // Weighted max if different
	Percentage    *float64
	LetterGrade   string
	Status        AssignmentStatus
	Flags         []AssignmentFlag // Collected, Late, Missing, etc.
}

// AssignmentCategory represents a category of assignments with summary stats
type AssignmentCategory struct {
	Name           string
	Count          int
	PointsPossible float64
	PointsEarned   float64
	Percentage     float64
}

// GPA represents GPA information
type GPA struct {
	Current    float64
	Cumulative float64
	Weighted   bool
}

// ProgressReport represents a progress report or report card PDF
type ProgressReport struct {
	ID              string
	StudentID       string
	Type            string    // "Progress Report" or "Report Card"
	Title           string    // e.g., "Q1 Progress Report"
	Year            string    // Academic year
	URL             string    // URL to PDF
	DatePosted      time.Time // When it was posted
	DateAvailable   time.Time // When it became available (if different)
}

// HistoricalGrade represents a completed course from Grade History (transcript)
type HistoricalGrade struct {
	DateCompleted    time.Time
	GradeLevel       int
	School           string
	CourseNumber     string
	CourseName       string
	CreditEarned     float64
	CreditAttempted  float64
	LetterGrade      string
	IncludedInGPA    bool   // false if marked with asterisk
}

// Attendance represents an attendance record
type Attendance struct {
	Date       time.Time
	Status     AttendanceStatus
	Period     string
	CourseName string
}

// AttendanceStatus represents attendance status
type AttendanceStatus string

const (
	AttendancePresent AttendanceStatus = "present"
	AttendanceAbsent  AttendanceStatus = "absent"
	AttendanceTardy   AttendanceStatus = "tardy"
	AttendanceExcused AttendanceStatus = "excused"
)

// Event represents a calendar event
type Event struct {
	ID          string
	Title       string
	Description string
	Date        time.Time
	EventType   EventType
}

// EventType represents the type of event
type EventType string

const (
	EventHoliday      EventType = "holiday"
	EventNoSchool     EventType = "no_school"
	EventEarlyRelease EventType = "early_release"
	EventOther        EventType = "other"
)

// Credentials stores authentication credentials
type Credentials struct {
	Username string
	Password string
}

// Session stores session information
type Session struct {
	Cookies   []*http.Cookie
	CSRFToken string
	ExpiresAt time.Time
	mu        sync.RWMutex
}

// IsValid checks if the session is still valid
func (s *Session) IsValid() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Now().Before(s.ExpiresAt)
}

// SetExpiry sets the session expiry time
func (s *Session) SetExpiry(expiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ExpiresAt = expiresAt
}

// GetCookies returns a copy of the session cookies
func (s *Session) GetCookies() []*http.Cookie {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cookies := make([]*http.Cookie, len(s.Cookies))
	copy(cookies, s.Cookies)
	return cookies
}
