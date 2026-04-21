# Reference Instance PowerSchool Structure

This document captures the HTML structure and URLs observed on the
reference PowerSchool instance the library was developed against. Other
districts may differ — consult your own instance with DevTools and add
notes here if selectors need to change.

## Authentication ✅
- **Login URL**: `https://ps.example.org/public/`
- **Username field**: Found by selectors (working)
- **Password field**: Found by selectors (working)
- **Submit button**: `#btn-enter-sign-in` ✅

## Navigation Structure

### Student Switching ✅
- **Pattern**: `javascript:switchStudent(ID)` in navigation links
- **Example IDs**: 111111, 111112
- **Status**: Successfully extracting both students

### Main Pages

#### 1. Guardian Home (`/guardian/home.html`) ⏳
- **Purpose**: Main dashboard after login
- **Contains**:
  - Student switcher (navigation bar)
  - Student photo and details:
    - Official student ID number
    - State ID number
    - Grade level ⚠️ (parsing issue - shows 0 instead of 8)
    - Student portal username/email
    - Source username
  - Grades and Attendance tab (tabbed view)
  - Left navigation menu

**Grade Level Parsing Issue**: Need HTML structure to fix
- Currently using heuristic (any number 1-12 near word "grade")
- Getting wrong value (0 instead of 8)
- **TODO**: Get HTML snippet of student info section

#### 2. Grades Summary Table (home page, "Grades" tab) ⏳
**Columns**:
- `Exp` - Schedule slot/class period
- `Last Week: M T W H F` - Last week attendance by day
- `This Week: M T W H F` - This week attendance by day
- `Course` - Contains 4 pieces of info:
  - Course name (and section)
  - Teacher email link (Last, First Middle format)
  - Room number
- Quarter/Semester grades:
  - Q1, Q2, S1, Q3, Q4, S2
- `Absences` - Total absences
- `Tardies` - Total tardies

**Attendance Codes**:
- L = Late
- A = Absent
- M = Medical Exemption

**Grade Colors**: Color-coded (need to determine if CSS classes or inline styles)

**TODO**: Need HTML of this table to implement parser

#### 3. Class Score Detail (drill-down from grade) ⏳
**Accessed by**: Clicking on a grade in the summary table

**Contains 3 tables**:

**Table 1: Course Info** (reiterates summary row + adds):
- Teacher Comments
- Section Description

**Table 2: Assignment Categories**
- Category (e.g., Classwork, Quiz, Honors, Lab)
- Number of assignments
- Points possible
- Points earned
- Grade (percentage)

**Table 3: Assignments**
**Columns**:
- Due Date
- Category
- Assignment (name)
- Seven cells grouped as "Fields"
- Score (e.g., "10/15" or "10/10 (20/20)" for weighted)
- Percentage
- Grade (letter)
- View button (not useful - no content)

**Assignment Flags** (icons in score field):
- Collected
- Late
- Missing
- Exempt from Final Grade
- Absent
- Incomplete
- Excluded

**TODO**: Need HTML of assignment table

#### 4. Progress Reports (`https://psplugin.example.org/ProgressReports/Reports/Index?studentidentifier=[ID]`) 📄
- **Purpose**: Links to PDF progress reports and report cards
- **Organization**: By academic year
- **File types**: PDFs
- **Important**: Need to track when new PDFs are posted
- **Status**: Stub implemented, needs HTML structure

#### 5. Grade History (`/guardian/termgrades.html`) 📊
- **Purpose**: Transcript-like view of completed courses
- **Note**: "not marked as an official transcript"

**Columns**:
- Date Completed
- Grade Level
- School
- Course Number
- Course Name
- Credit Earned
- Credit Attempted
- Grade (letter grade, P=pass, F=fail)
  - Asterisk (*) = not used for GPA calculations

**GPA Calculation**: Can be calculated from this data
**Status**: Stub implemented, needs HTML structure

## Left Navigation Menu

**Links discovered**:
1. ✅ Grades and Attendance
2. ⏳ Performance and Progress Reports → Progress Reports PDFs
3. ⏳ Grade History → Transcript view
4. ℹ️ School Information
5. ℹ️ Assessments (popup or external)
6. ℹ️ Books, Fines and Fees (popup or external)
7. ℹ️ School Choice (popup or external)
8. ℹ️ Schoology Access Codes (popup or external)
9. ℹ️ Data Verification Form (popup or external)
10. ℹ️ SchoolPay (popup or external)
11. ℹ️ Highly Capable (popup or external)
12. ℹ️ School District Forms (popup or external)
13. ℹ️ Help

**Footer**:
- District Code: "XXXX"
- App store links (iOS, Google Play)

## Implementation Status

### ✅ Working
- Authentication
- Student discovery (finds both students)
- Session management

### ⚠️ Needs Fixing
- Grade level parsing (shows 0 instead of 8)

### ⏳ Needs HTML Structure
1. Student info section (to fix grade level)
2. Grades summary table
3. Class detail / assignments table
4. Progress reports page
5. Grade history table

### 📋 TODO
1. Get HTML snippet of student info section
2. Get HTML of grades table
3. Get HTML of assignments table
4. Get HTML of progress reports page
5. Get HTML of grade history table

## Next Steps

**Immediate**: Fix grade level parsing
- Need HTML snippet showing: "Grade Level: 8" or similar
- Location: Home page, near student photo

**Then**: Implement grades table parser
- Need the actual `<table>` element with class/id

**Then**: Implement assignments parser
- Need assignment table HTML

---

**Test Results So Far**:
- ✅ Found 2 students (111111, 111112)
- ⚠️ Grade level: 0 (should be 8 for student 111112)
- Student names: Extracted successfully
