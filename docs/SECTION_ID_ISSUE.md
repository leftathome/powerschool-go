# Section ID vs FRN Parameter Issue

## Status: RESOLVED (2026-04-20)

`data-sectionid` is present in the **raw** `scores.html` response (not
injected by Angular), so `parseScoresMetadata` pulls it directly out of the
HTML body. The same parser also extracts `studentFRN` from the inline
`data-ng-init` expression — turns out the lookup API wants that studentFRN's
DCID, not the `switchStudent()` nav ID that shows up on `Student.ID`.

Both are cached back onto `*Grade` after the first lookup so subsequent
calls for the same grade skip the scores-page round trip.

## Problem

The PowerSchool assignment API requires a `section_id`, but we only have the `frn` parameter from the grades table.

## What We Have vs What We Need

### From Grades Table URL
```
/guardian/scores.html?frn=00111222333
```
- `frn` = `00111222333` (parsed to `111222333`)

### API Requires
```json
{
  "section_ids": [654321],
  "student_ids": [999999],
  ...
}
```
- `section_id` = `654321` (completely different number!)

## Why This Happens

PowerSchool uses multiple identifiers for the same course:
- **FRN** (from URL): External reference number, used in URLs
- **Section DCID** (sectionsdcid): Internal database ID, used in API calls
- **Section ID**: Yet another ID that might be different

From your API response, we can see:
```json
{
  "sectionsdcid": 654000,  // This is close to 654321
  ...
}
```

## Possible Solutions

### Option 1: Parse Section ID from HTML (RECOMMENDED)
The scores.html page likely contains the section ID somewhere in:
- JavaScript variables
- Hidden form fields
- Data attributes on Angular components

**Next Step**: Visit the scores page in browser, view source, and search for `654321` to see where it's defined.

### Option 2: Fetch Assignments Page with Browser
Instead of using the API, use chromedp to:
1. Navigate to `/guardian/scores.html?frn=00111222333`
2. Wait 5-10 seconds for Angular to load assignments
3. Extract HTML after JavaScript execution
4. Parse assignments from the rendered DOM

**Pros**:
- No need to find section ID
- Works even if API changes
- Handles all Angular rendering

**Cons**:
- Slower (browser startup + JavaScript execution)
- More fragile (depends on HTML structure)

### Option 3: Try Simpler API Endpoints
PowerSchool might have other endpoints that accept `frn` instead of `section_id`:
- `/guardian/scores.json?frn=XXX`
- `/guardian/assignments.json?frn=XXX`
- `/ws/guardian/scores?frn=XXX`

**Next Step**: In browser DevTools Network tab, check if there are other API calls when viewing assignments.

## Recommendation

**Use Option 1** - Find where section_id is stored in the HTML and parse it.

This gives us the best of both worlds:
- Fast API calls (no browser needed)
- Reliable (uses official API)
- Simple mapping (frn → fetch page once → extract section_id → cache it)

## Implementation Plan

1. **Fetch scores.html page once per course**
   ```go
   html := getPageHTML(ctx, "/guardian/scores.html?frn=" + courseID)
   ```

2. **Parse section ID from HTML**
   - Look for JavaScript: `var sectionId = 654321;`
   - Look for data attributes: `data-section-id="654321"`
   - Look for Angular: `data-pss-student-assignment-scores section-id="654321"`

3. **Cache section ID in Grade struct**
   ```go
   type Grade struct {
       CourseID  string  // frn parameter
       SectionID string  // Actual section DCID for API calls
       ...
   }
   ```

4. **Use section ID in API calls**
   ```go
   sectionIDInt, _ := strconv.ParseInt(grade.SectionID, 10, 64)
   payload := map[string]interface{}{
       "section_ids": []int64{sectionIDInt},
       ...
   }
   ```

## What to Try Next

Please:
1. Navigate to: `https://ps.example.org/guardian/scores.html?frn=00111222333`
2. Right-click → View Page Source
3. Search for `654321` (the section_id from your cURL)
4. Share where/how it appears in the HTML

This will tell us exactly how to extract the section ID!
