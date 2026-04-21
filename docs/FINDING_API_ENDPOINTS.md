# Finding PowerSchool API Endpoints

## Why This Matters

reference-instance PowerSchool uses Angular to load assignment data dynamically. Instead of fighting with browser automation timing issues, we can call the same API endpoints that Angular uses.

This approach is:
- **Faster**: Direct HTTP requests vs. waiting for JavaScript
- **More reliable**: No timing issues or race conditions
- **Easier to maintain**: JSON parsing vs. HTML scraping
- **True hybrid architecture**: Browser for auth, HTTP for data

## Step-by-Step Guide

### 1. Open Chrome DevTools

1. Login to PowerSchool manually: https://ps.example.org
2. Navigate to a course's assignments page (click on a course from grades)
3. Press `F12` to open DevTools
4. Click the **Network** tab
5. Check "Preserve log" to keep requests when navigating

### 2. Filter for API Calls

1. In the Network tab filter box, select **Fetch/XHR**
   - This shows only API calls (not images, CSS, etc.)
2. Refresh the page or navigate to trigger the Angular component to load
3. Look for requests with names like:
   - `scores` or `scores.json`
   - `assignments`
   - `categories`
   - Anything in `/ws/` or `/api/` paths

### 3. Inspect Each Request

For each promising request, click on it and note:

#### Headers Tab
- **Request URL**: Full URL including query parameters
  - Example: `https://ps.example.org/ws/xte/assignment/scores?frn=00111222333`
- **Request Method**: Usually `GET` or `POST`
- **Status Code**: Should be `200 OK`

#### Payload Tab (for POST requests)
- Note any form data or JSON sent in request body

#### Preview Tab
- See the structured response data
- Confirms if it's the data we want

#### Response Tab
- Raw JSON response
- Copy this - we'll need it for parsing

### 4. Common PowerSchool API Patterns to Look For

Based on typical PowerSchool instances:

```
# Assignment scores (most likely)
GET /guardian/scores.json?frn=COURSEID
GET /ws/xte/assignment/scores?frn=COURSEID&fg=QUARTER

# Assignment details
GET /ws/xte/assignment/detail?assignmentid=XXXXX

# Categories/Standards
GET /guardian/categories.json?frn=COURSEID
GET /ws/xte/assignment/categories?frn=COURSEID

# Student switching
POST /guardian/student/switch
  Body: studentid=XXXXXXX
```

### 5. Test an Endpoint

Once you find a promising endpoint:

1. Right-click on the request in DevTools
2. Select "Copy" → "Copy as cURL"
3. Paste in a text file and share it with me

Example of what you'll get:
```bash
curl 'https://ps.example.org/ws/xte/assignment/scores?frn=00111222333' \
  -H 'Cookie: JSESSIONID=...; ...' \
  -H 'Accept: application/json'
```

## What to Share

For each API endpoint you find, please share:

1. **The URL**: e.g., `/ws/xte/assignment/scores?frn=00111222333`
2. **Method**: GET or POST
3. **Parameters**: What query parameters or POST body fields are used
4. **Sample response**: Copy the JSON response (sanitize student names if needed)
5. **When it's called**: What triggers this API call (page load, button click, etc.)

## Quick Check: Does the API Exist?

The easiest way to check:

1. Go to a course assignments page
2. Open DevTools → Network → XHR filter
3. Do you see ANY requests?
   - **Yes**: Great! We can use those endpoints
   - **No**: All data might be server-side rendered (less likely with Angular)

## Alternative: Look at Page Source

If you can't find XHR requests:

1. On the assignments page, right-click → "View Page Source"
2. Search for `<script>` tags
3. Look for:
   - API endpoint URLs in the JavaScript
   - Angular configuration objects
   - Data embedded as JSON in the page

Example:
```html
<script>
  window.assignmentData = {"courseId": "...", "apiUrl": "/ws/..."};
</script>
```

## Next Steps

Once you share the API endpoints, I'll:

1. Implement HTTP-only fetching for assignments
2. Parse JSON responses (much easier than HTML!)
3. Update the library to use hybrid approach everywhere:
   - Browser: Authentication only (~6 seconds one time)
   - HTTP: All data fetching (fast, reliable)

This will make the library much faster and more maintainable!
