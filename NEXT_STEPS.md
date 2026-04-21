# Next Steps - Authentication Working!

Great news! Authentication is now working with reference-instance PowerSchool. 🎉

## What's Working

✅ Authentication with credentials
✅ Session cookie extraction
✅ 24-hour session validity

## What Needs Fixing

The HTML parsers need to be updated based on the reference instance's actual HTML structure.

### Current Error

```
Failed to fetch students: [PARSE_ERROR] failed to parse students list: no students found in HTML
```

This is expected - we guessed at the HTML structure, and now we need to use the actual structure.

## How to Fix the Parsers

### Option 1: Manual Browser Inspection (Recommended)

After you log in to PowerSchool manually in Chrome:

1. **Find the Student Selector** (if you have multiple students):
   - Look for a dropdown or list
   - Right-click → Inspect
   - Note the HTML structure (id, class, name attributes)
   - Screenshot or copy the HTML

2. **Navigate to Grades Page**:
   - Click on "Grades" or similar
   - Note the URL pattern (e.g., `/guardian/scores.html?frn=XXX`)
   - Right-click on the grades table → Inspect
   - Note the table structure

3. **Navigate to Assignments**:
   - Find assignments list
   - Note the URL
   - Inspect the HTML structure

### Option 2: Save HTML Programmatically

We can modify the test to save HTML to files:

```powershell
# After authentication succeeds, manually browse to pages and save HTML
# Then we can parse it offline
```

## Information Needed

Please share:

### 1. URLs After Login

After you log in, what URLs do you see for:
- Home/dashboard: `https://ps.example.org/guardian/____?`
- Students list: (if applicable)
- Grades page: `https://ps.example.org/guardian/____?`
- Assignments: `https://ps.example.org/guardian/____?`

### 2. Student Selector HTML

If you have multiple students, how do you switch between them?
- Dropdown menu?
- Links?
- Tabs?

Please inspect and share the HTML (right-click → Inspect → copy outer HTML).

Example of what I'm looking for:
```html
<select id="student-selector" name="frn">
  <option value="12345">Student Name</option>
  <option value="67890">Another Student</option>
</select>
```

### 3. Grades Table HTML

Open the grades page, find the table/list of grades, inspect it:

```html
<table class="grades-table">
  <tr>
    <td class="course">Math</td>
    <td class="grade">A</td>
    <td class="percent">95%</td>
  </tr>
</table>
```

### 4. Quick Screenshots (Optional but Helpful)

- Screenshot of the home page after login
- Screenshot of the grades page
- Screenshot of assignments page

## What I'll Do With This Info

Once you share the HTML structure, I'll:

1. Update `student.go` with correct selectors for student list
2. Update `grades.go` with correct selectors for grades table
3. Update `assignments.go` with correct selectors for assignments
4. Add reference-instance-specific configuration
5. Create tests to verify parsing works

## Temporary Workaround

If you want to explore what data we CAN get right now, I can:

1. Create a simple tool that just dumps raw HTML from pages
2. You save it to files
3. We parse it together

Let me know if you want this approach!

## Current Code That Needs Updating

The parsers that need fixing:

### student.go
Currently looks for:
- `select#studentid option`
- `select[name='frn'] option`
- `.student-info`

### grades.go
Currently looks for:
- `table.linkDescList tr`
- `table.grid tr`

### assignments.go
Currently looks for:
- `table.linkDescList tr`
- `table.grid tr`
- `table[id*='assignment'] tr`

These are generic guesses that probably don't match the reference instance's actual HTML.

---

**Ready to proceed?** Just share the URL patterns and some HTML snippets, and I'll update the parsers!
