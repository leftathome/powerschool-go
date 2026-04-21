# Implementation Status

## Latest session (2026-04-20)

- Root-caused the `GetAssignments` failure — three bugs stacked: scores.html
  required full query params; the API expects the studentFRN-derived DCID,
  not the switchStudent() nav ID; and Referer is mandatory.
- `Grade` gained `ScoresURL` and `StudentAPIID`; `GetAssignments` /
  `GetAssignmentCategories` now take `*Grade` instead of a bare courseID.
- Added `FetchScoresMetadata` returning both section and student IDs;
  `FetchSectionID` is now a thin wrapper over it.
- Wired `--no-sandbox` into `internal/browser` so chromedp runs as root
  (WSL/containers); auto-detected via `os.Geteuid()`.
- Unit test `assignments_test.go` drives `GetAssignments` against an
  httptest.Server, asserts the outbound Referer, and parses a scrubbed live
  fixture (`testdata/assignments_lookup.json`).
- Integration test (`//go:build integration`) runs the full pipeline against
  a live PowerSchool — passed end-to-end against the reference instance.

## Completed ✅

### Authentication
- ✅ Browser-based login
- ✅ Session cookie extraction
- ✅ 24-hour session validity
- ✅ Chrome profile popup prevention
- ✅ Multiple selector fallbacks for login button
- ✅ Reference-instance specific: `#btn-enter-sign-in` button

### Student Retrieval
- ✅ Parse `javascript:switchStudent(ID)` from navigation
- ✅ Extract student IDs (111111, 111112 pattern)
- ✅ Extract student names from nav links
- ⏳ Enrich with details (student number, state ID, grade level) - *heuristic, needs testing*

## In Progress ⏳

### Grades Parsing
Based on your description, need to parse:

**Grades Summary Table** (home page, "Grades" tab):
- `Exp` column (schedule slot/period)
- Attendance (Last Week: M T W H F, This Week: M T W H F)
- Course info:
  - Course name + section
  - Teacher email link (Last, First Middle format)
  - Room number
- Quarter/Semester grades: Q1, Q2, S1, Q3, Q4, S2
- Absences count
- Tardies count

**Class Score Detail** (drill-down):
- Teacher Comments
- Section Description
- Assignment Categories table
- Assignments table with flags

### Assignments Parsing
Need to parse from **Class Score Detail** page:

**Assignments Table**:
- Due Date
- Category
- Assignment name
- Fields (7 cells grouped)
- Score (e.g., "10/15" or "10/10 (20/20)" for weighted)
- Percentage
- Grade (letter)
- View button

**Flags to detect**:
- Collected
- Late
- Missing
- Exempt from Final Grade
- Absent
- Incomplete
- Excluded

## TODO - Next Implementation Steps

### 1. ✅ Test Student Retrieval - COMPLETED
Students working! Finds both students, parses details for currently displayed one.

### 2. ✅ Test Grades Retrieval - COMPLETED
Grades working! Successfully parsing 7 courses with all details.

### 3. CURRENT: Find API Endpoints for Assignments

**Problem**: The reference instance loads assignments via Angular, which makes browser automation slow and unreliable.

**Solution**: Find the API endpoints that Angular calls and use HTTP requests directly.

**How to find API endpoints**:

1. **Open Chrome DevTools Network Tab**:
   - Login to PowerSchool manually
   - Navigate to a course's assignments page
   - Open DevTools (F12) → Network tab
   - Filter by "Fetch/XHR" to see API calls
   - Look for requests to endpoints like:
     - `/ws/`, `/api/`, `/guardian/...` with JSON responses
     - Anything containing "assignment", "score", "category"

2. **For each endpoint found, note**:
   - Full URL
   - Request method (GET/POST)
   - Request parameters (query string or POST body)
   - Response format (usually JSON)
   - Required cookies/headers

3. **Common PowerSchool API patterns**:
   - `/guardian/scores.json?frn=COURSEID`
   - `/ws/xte/assignment/...`
   - `/ws/schema/query/...`

### 4. Implement HTTP-Based Assignment Fetching
Once API endpoints are found:
- Add HTTP request methods to Client
- Parse JSON responses (easier than HTML!)
- Falls back gracefully if endpoints don't exist

### 5. Document API Endpoints
Create `docs/API_ENDPOINTS.md` documenting:
- All discovered endpoints
- Parameters required
- Response structure
- Example requests/responses

## Questions to Answer

1. **Do grades have CSS classes for colors?** (e.g., `class="grade-a"` or inline `style="color:green"`?)
2. **What's the actual table class/id?** (e.g., `<table class="grades-summary">`)
3. **How are flags shown in assignments?** (images, text, CSS classes?)
4. **Is the "View" button useful?** (You said it wasn't, just confirming)

## File Structure

```
student.go     - ✅ Implemented, needs testing
grades.go      - ⏳ Needs HTML structure
assignments.go - ⏳ Needs HTML structure
types.go       - ✅ Updated with all fields
```

## Testing Strategy

1. ✅ Test authentication → **WORKING**
2. ⏳ Test student retrieval → **YOUR TURN**
3. ⬜ Test grades retrieval → Need HTML
4. ⬜ Test assignments retrieval → Need HTML
5. ⬜ Integration test: Full pipeline

---

**Current blocker**: Need to verify student parsing works, then get grades table HTML.

Run the test and let me know what happens!
