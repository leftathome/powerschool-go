# Debugging Assignment API Issue

## Status: RESOLVED (2026-04-20)

**Root cause was three bugs stacked on top of each other**, not the single
"headers" suspicion this doc originally held:

1. **`scores.html?frn=X` alone redirects to login.** All four of
   `frn`, `begdate`, `enddate`, `fg`, and `schoolid` are required; missing
   any one bounces the request to `/public/home.html`. Fix: `Grade.ScoresURL`
   now captures the full URL from the home-page grades table and
   `FetchSectionID` / `FetchScoresMetadata` use it verbatim.
2. **The `student_ids` the API wants is NOT the `switchStudent()` nav ID.**
   Student Alpha's nav ID is `111111` but the API expects `999999` — derived from the
   scores page's `data-ng-init="studentFRN = '001999999'"`, with the `001`
   prefix stripped. `FetchScoresMetadata` now pulls both `SectionID` and
   `StudentAPIID` from the scores page; `GetAssignments` uses the API one.
3. **The POST does require `Referer`** (confirmed — stripping it returns a
   400 + HTML error page, which was the original symptom). This had already
   been added; the first two bugs masked it while we were debugging.

The httptest-backed unit test in `assignments_test.go` locks the Referer
behavior in place, and the integration test (`-tags integration`) exercises
the full live pipeline.

## Original notes (kept for context)

The `/ws/xte/assignment/lookup` API endpoint returns HTML instead of JSON when called from our Go code.

## What Works

- **Authentication**: Successfully logs in and maintains 24-hour session
- **Grades Parsing**: Successfully parses all 7 courses with complete details
- **Students**: Successfully finds and parses both students

## What We Know

### API Endpoint (from DevTools)
- **URL**: `https://ps.example.org/ws/xte/assignment/lookup?_=1761632682329`
- **Method**: POST
- **Content-Type**: application/json
- **Payload**:
```json
{
  "section_ids": [654321],
  "student_ids": [999999],
  "start_date": "2025-9-3",
  "end_date": "2025-11-5"
}
```

### Current Implementation
- Located in: [assignments.go](../assignments.go)
- Sends POST with JSON payload
- Uses session cookies from authentication
- Sets headers:
  - `User-Agent: powerschool-go/0.1.0`
  - `Content-Type: application/json`
  - `Accept: application/json, */*`

### Error
```
Failed to parse API response: invalid character '<' looking for beginning of value
```

This means the API is returning HTML (probably an error page) instead of JSON.

## Possible Causes

1. **Missing CSRF Token**: Many APIs require a CSRF/XSRF token
2. **Missing Referer Header**: API might check where request came from
3. **Missing Origin Header**: Cross-origin request blocking
4. **Session State Issue**: Cookies might not be sufficient, might need additional session state
5. **Request ID/Timestamp**: The `_=timestamp` parameter might need to match something in session
6. **Different Cookie Domain**: Cookies might need specific domain/path settings

## How to Debug

### Option 1: Compare Headers (Recommended)

In Chrome DevTools when viewing the successful API call:

1. Right-click on the `/ws/xte/assignment/lookup` request
2. Select "Copy" → "Copy as cURL (bash)"
3. Share the full cURL command (it will show ALL headers)
4. We can compare to what our Go code sends

### Option 2: Check Response

Run the test program with debug logging enabled:

```bash
go run ./cmd/test-auth -headless -debug 2>&1 | tee output.log
```

Then look in `output.log` for "API response" to see what HTML we're actually getting.

### Option 3: Browser Network Export

1. In Chrome DevTools Network tab
2. Right-click on the request
3. Select "Copy" → "Copy as Fetch"
4. Share the code - it will show exactly what JavaScript sends

## Next Steps

Once we see the full headers from a successful browser request, we can:

1. Add any missing headers to our Go code
2. Extract and send any CSRF tokens
3. Update cookie handling if needed
4. If API is truly inaccessible, fall back to HTML parsing after JavaScript execution

## Alternative Approach

If the API proves too difficult to access directly, we can:

1. Use browser automation (chromedp) to fetch the page
2. Wait longer for Angular to load (5-10 seconds)
3. Extract the rendered HTML after JavaScript executes
4. Parse assignments from the DOM

This is slower but more reliable if the API has complex auth requirements.

## Code Locations

- API Implementation: [assignments.go](../assignments.go#L13-L162)
- HTTP Client: [client.go](../client.go#L172-L220)
- API Documentation: [API_ENDPOINTS.md](API_ENDPOINTS.md)
