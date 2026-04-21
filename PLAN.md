# PowerSchool Go Library - Implementation Plan

## Project Overview

**Repository**: github.com/leftathome/powerschool-go
**License**: MIT
**Purpose**: Standalone Go client library for accessing PowerSchool e-learning platform
**Parent Project**: Trunchbull Academic Dashboard

## Goals

1. Create a modular, accessible, easy-to-test Go library for PowerSchool
2. Support credential-based authentication using chromedp (browser automation)
3. Provide clean, idiomatic Go API for common PowerSchool operations
4. Enable the Trunchbull dashboard to fetch student data, grades, assignments, and attendance
5. Fill ecosystem gap - no maintained Go libraries exist for PowerSchool

## Design Principles

1. **Modular & Reusable** - Standalone library, not monolithic
2. **Go-native** - Leverage Go's strengths (concurrency, strong typing)
3. **Well-tested** - Comprehensive test coverage
4. **Well-documented** - Easy for others to use
5. **MIT licensed** - Maximum reusability
6. **Credential-based** - Start with chromedp approach (Approach 2)

## Architecture

### Repository Structure

```
powerschool-go/
├── README.md              # Documentation and usage examples
├── LICENSE                # MIT License
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
├── PLAN.md                # This file
├── client.go              # Main client
├── auth.go                # Authentication logic
├── student.go             # Student endpoints
├── grades.go              # Grades endpoints
├── assignments.go         # Assignment endpoints
├── attendance.go          # Attendance endpoints (Phase 3)
├── calendar.go            # Calendar/events (Phase 3)
├── types.go               # Data models and types
├── errors.go              # Error types and handling
├── examples/              # Example programs
│   ├── basic/
│   │   └── main.go       # Basic usage example
│   ├── credentials/
│   │   └── main.go       # Credential-based auth example
│   └── session/
│       └── main.go       # Session persistence example
├── internal/              # Internal implementation details
│   ├── browser/          # Chromedp automation
│   │   └── browser.go
│   └── scraper/          # HTML parsing
│       └── scraper.go
└── *_test.go             # Test files alongside implementation
```

## API Design

### Core Client

```go
package powerschool

// Client is the main PowerSchool client
type Client struct {
    baseURL     string
    credentials *Credentials
    session     *Session
    httpClient  *http.Client
}

// NewClient creates a new PowerSchool client
func NewClient(baseURL string, opts ...Option) (*Client, error)

// Option configures the client
type Option func(*Client) error

// WithCredentials sets username/password authentication
func WithCredentials(username, password string) Option

// WithSession sets session token authentication
func WithSession(token string, cookies []*http.Cookie) Option

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option

// Authenticate logs in and obtains session
func (c *Client) Authenticate(ctx context.Context) error

// GetStudents returns all students accessible to this account
func (c *Client) GetStudents(ctx context.Context) ([]*Student, error)

// GetGrades returns grades for a student
func (c *Client) GetGrades(ctx context.Context, studentID string) ([]*Grade, error)

// GetAssignments returns assignments for a student
func (c *Client) GetAssignments(ctx context.Context, studentID string) ([]*Assignment, error)

// GetGPA returns GPA for a student
func (c *Client) GetGPA(ctx context.Context, studentID string) (*GPA, error)
```

### Data Types

Key types to implement:
- `Student` - Student information
- `Grade` - Course grade information
- `Assignment` - Assignment details
- `GPA` - GPA calculation
- `Attendance` - Attendance records (Phase 3)
- `Event` - Calendar events (Phase 3)

## Implementation Phases

### Phase 1: Core Functionality (Current - Week 1)

**Status**: In Progress

**Tasks**:
- [x] Create repository with MIT license
- [x] Initialize Go module
- [x] Define core types and data models
- [x] Implement error handling types
- [x] Create main client structure
- [x] Implement chromedp-based authentication
- [x] Create internal browser automation package
- [x] Implement student data retrieval (basic - current student only)
- [ ] Implement grades retrieval
- [ ] Implement basic assignment retrieval
- [ ] Write unit tests for core functionality
- [ ] Create README with documentation
- [ ] Create example programs
- [ ] Publish v0.1.0

**Deliverable**: v0.1.0 with authentication, student listing, grades, and assignments

### Phase 2: Enhanced Features (Week 2)

**Tasks**:
- [ ] Implement attendance tracking
- [ ] Implement calendar/events retrieval
- [ ] Add session persistence and caching
- [ ] Enhance error handling and retry logic
- [ ] Add context timeout handling
- [ ] Improve HTML parsing robustness
- [ ] Add more comprehensive tests
- [ ] Update documentation

**Deliverable**: v0.2.0 with full feature set

### Phase 3: Production Ready (Week 3)

**Tasks**:
- [ ] Performance optimization
- [ ] Add rate limiting support
- [ ] Circuit breaker pattern
- [ ] Comprehensive logging
- [ ] Session auto-refresh
- [ ] Integration tests with mock data
- [ ] CI/CD setup (GitHub Actions)
- [ ] Code coverage reporting
- [ ] Security audit
- [ ] Final documentation polish

**Deliverable**: v1.0.0 production-ready library

### Phase 4: Trunchbull Integration (Week 4)

**Tasks**:
- [ ] Import library into Trunchbull
- [ ] Build aggregation layer
- [ ] Handle multiple students
- [ ] Implement background sync
- [ ] Error handling and recovery
- [ ] Integration testing

**Deliverable**: Working Trunchbull dashboard using powerschool-go

## Technical Decisions

### Authentication: chromedp (Hybrid Approach)

**Choice**: Hybrid approach using chromedp for auth + session cookies for API calls

**Current Implementation**:
- chromedp handles initial authentication (the reference instance: ~7 seconds, 24-hour session)
- Session cookies extracted and stored
- Subsequent page requests use chromedp with session cookies (ensures JavaScript execution)
- HTML parsing done with goquery (browser-independent)

**Rationale**:
- Handles JavaScript-heavy login flows
- Works with SSO and complex authentication (the reference instance uses BigIP F5 load balancer)
- More reliable than HTTP-only approach
- Easier to debug (can run non-headless)
- Extracts 24-hour session cookies for reuse
- Successfully tested with reference-instance PowerSchool instance

**Trade-offs**:
- Heavier dependency (requires Chrome)
- Slower initial login (~7 seconds)
- Page fetching still uses browser automation (could be optimized to HTTP in Phase 2)

**Future Optimization Path** (Phase 2):
- Keep chromedp for authentication (most reliable)
- Try direct HTTP requests with session cookies for page fetching
- Add HTTP-only auth option for simpler PowerSchool instances
- Benchmark and compare approaches

### HTML Parsing: goquery

**Choice**: Use goquery for HTML parsing

**Rationale**:
- jQuery-like selectors (familiar, readable)
- Well-maintained library
- Good performance
- Easy to test

### Session Management

**Approach**: Automatic refresh with fallback

- Store session cookies after authentication
- Check session validity before API calls
- Auto-refresh when expired (if credentials available)
- Return clear error if session expired without credentials

### Error Handling

**Strategy**:
- Define clear error types
- Use errors.Is/As for error checking
- Wrap errors with context
- Never expose credentials in error messages

## Dependencies

### Required

```go
require (
    github.com/chromedp/chromedp v0.9.5
    github.com/PuerkitoBio/goquery v1.9.0
)
```

### Testing

```go
require (
    github.com/stretchr/testify v1.8.4
)
```

## Testing Strategy

### Unit Tests
- Test each method independently
- Mock browser automation for auth tests
- Use sample HTML for parsing tests
- Test error conditions

### Integration Tests
- Use mock HTML responses
- Test full authentication flow
- Test session management
- Test data retrieval flow

### Example Tests
- Ensure all examples compile
- Add README example to tests

## Documentation Requirements

### README.md
- Project description and goals
- Installation instructions
- Quick start guide
- Authentication examples
- Common usage patterns
- API reference (or link to pkg.go.dev)
- Contributing guidelines
- License information

### Code Documentation
- Package-level documentation
- Function/method comments (godoc format)
- Type and constant documentation
- Example code in doc comments

### Examples
- Basic usage example
- Credential authentication
- Session persistence
- Error handling
- Multiple students

## Success Criteria

### v0.1.0 Success
- [x] MIT license in place
- [ ] Authentication with credentials works
- [ ] Can list students
- [ ] Can retrieve grades
- [ ] Can retrieve assignments
- [ ] All tests pass
- [ ] README complete
- [ ] At least one working example

### v1.0.0 Success
- [ ] All planned features implemented
- [ ] >80% code coverage
- [ ] Comprehensive documentation
- [ ] Example programs for all major features
- [ ] No known critical bugs
- [ ] Successfully used in Trunchbull

## Timeline

| Week | Phase | Deliverable |
|------|-------|-------------|
| 1 | Phase 1: Core | v0.1.0 - Auth, Students, Grades, Assignments |
| 2 | Phase 2: Enhanced | v0.2.0 - Attendance, Calendar, Session mgmt |
| 3 | Phase 3: Production | v1.0.0 - Production ready |
| 4 | Phase 4: Integration | Working Trunchbull dashboard |

**Total**: 4 weeks to production-ready library and integrated dashboard

## Current Status

**Phase**: 1 (Core Functionality)
**Status**: In Progress
**Last Updated**: 2025-10-27

### Completed
- [x] MIT License created
- [x] Project plan created
- [x] Go module initialized (go 1.25, chromedp v0.14.2)
- [x] Core type definitions (Student, Grade, Assignment, etc.)
- [x] Error handling types (ErrNotFound, ErrSessionExpired, etc.)
- [x] Main client structure with options pattern
- [x] chromedp-based authentication (the reference instance: ~7s, 24h session)
- [x] Internal browser automation package
- [x] Logger with multiple levels (None, Error, Warn, Info, Debug)
- [x] Student discovery from navigation bar (switchStudent JavaScript)
- [x] Student detail parsing for currently-selected student
- [x] 1Password integration scripts (bash + PowerShell)
- [x] Test program (cmd/test-auth)
- [x] GetGrades() implementation - Successfully parsing 7 courses with all details
- [x] API endpoint discovery for assignments

### Currently Working On
- [ ] Debugging assignments API endpoint

**Issue**: `/ws/xte/assignment/lookup` API returns HTML instead of JSON
- Found API endpoint via browser DevTools: POST `/ws/xte/assignment/lookup`
- Implemented JSON request/response handling
- API returning HTML error page instead of JSON (might need CSRF token or special headers)
- Need to investigate what headers/auth the browser sends that we're missing

### In Progress
- [x] GetAssignments() code structure completed
- [ ] GetAssignments() API debugging needed

### Next Steps
1. Debug why API returns HTML (check headers, CSRF tokens, etc.)
2. Either fix API call or fall back to HTML parsing approach
3. Implement GetGPA() from grade history
4. Add progress reports retrieval
5. Write unit tests
6. Create README

## Notes

- Following instructions from .claude/CLAUDE.md:
  - No emoji in code
  - Test as we go with proper test framework
  - Document schema and types carefully
  - Use containers for testing where applicable

- PowerSchool URLs vary by district (e.g., ps.example.org)
- HTML structure may vary between PowerSchool versions - need robust parsing
- Consider adding district-specific configuration options

### Reference Instance Implementation Notes

**Authentication**:
- Login URL: https://ps.example.org/public/
- Uses BigIP F5 load balancer with session cookies
- Button selector: `#btn-enter-sign-in`
- Session duration: 24 hours
- Cookies: 15 cookies including JSESSIONID, MRHSession, etc.

**Student Discovery**:
- Navigation bar: `#students-list` with `li` elements
- Selected student marked with class `selected`
- Student switching via: `javascript:switchStudent(studentID)`
- Student IDs: numeric (e.g., 111111, 111112)

**Student Details** (table.student-demo):
- Student ID #: Official student number (e.g., 8199499)
- State ID #: State identifier (e.g., 9984053837)
- Grade Level: Numeric grade 1-12 (e.g., 8)
- Student Portal Username / Email: Login email
- Source Username: Username without domain
- School: Extracted from `#print-school span` (e.g., "Example Middle School")

**Grades Table** (table#tblgrades):
- Last Week / This Week attendance columns (M, T, W, H, F)
- Attendance codes: ME (Medical/Sick), A (Absent), L (Late), etc.
- Course info with teacher contact links
- Quarter grades: Q1, Q2, S1, Q3, Q4, S2
- Absences and Tardies columns with links to detail pages
- Color-coded grades: A (green #87BD6C), B (blue #CFE7FF), C (yellow #FFFF8D), D (orange #F9AC48), E/F (red #EF3D3D)

**Data Access Pattern**:
- Currently retrieves data for whichever student is displayed on home page
- To get all students' data, would need to implement student switching
- Home page shows one student's details at a time

## Questions for Future Consideration

1. Should we support multiple PowerSchool versions?
2. How to handle district-specific customizations?
3. Should we cache HTML parsing selectors?
4. Rate limiting - what are reasonable defaults?
5. Should session cookies be exportable for debugging?

---

**Document Version**: 1.0
**Last Updated**: 2025-10-24
**Status**: Active Development
