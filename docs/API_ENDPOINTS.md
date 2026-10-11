# PowerSchool API Endpoints

## Assignment Lookup

**Endpoint**: `/ws/xte/assignment/lookup`

**Method**: POST

**Content-Type**: application/json

**Payload Structure**:
```json
{
  "section_ids": [654321],
  "student_ids": [999999],
  "start_date": "2025-9-3",
  "end_date": "2025-11-5"
}
```

**Response Structure**:
```json
[
  {
    "_name": "assignment",
    "_id": 1109030,
    "assignmentid": 1109030,
    "hasstandards": false,
    "standardscoringmethod": "GradeScale",
    "_assignmentsections": [
      {
        "_name": "assignmentsection",
        "_id": 2492444,
        "assignmentsectionid": 2492444,
        "sectionsdcid": 654000,
        "name": "Patient 23 Case Study",
        "description": "<p>Optional HTML description</p>",
        "duedate": "2025-09-26",
        "scoretype": "PERCENT",
        "scoreentrypoints": 15.0,
        "totalpointvalue": 15.0,
        "weight": 1.0,
        "iscountedinfinalgrade": true,
        "isscorespublish": true,
        "isscoringneeded": true,
        "_assignmentcategoryassociations": [
          {
            "_name": "assignmentcategoryassoc",
            "_id": 2492122,
            "assignmentcategoryassocid": 2492122,
            "isprimary": true,
            "_teachercategory": {
              "_name": "teachercategory",
              "name": "Classwork",
              "color": "1",
              "description": "This Category is for High and Middle Schools only who use Point calculations"
            }
          }
        ],
        "_assignmentscores": [
          {
            "_name": "assignmentscore",
            "studentsdcid": 999999,
            "scorepoints": 15.0,
            "scorepercent": 100.0,
            "scorelettergrade": "A",
            "actualscoreentered": "100",
            "actualscorekind": "REAL_SCORE",
            "scoreentrydate": "2025-10-16 16:17:06",
            "whenmodified": "2025-10-16",
            "islate": false,
            "ismissing": false,
            "isincomplete": false,
            "isabsent": false,
            "isexempt": false,
            "iscollected": false,
            "authoredbyuc": false
          }
        ]
      }
    ]
  }
]
```

## Key Fields

### Assignment Level
- `assignmentid`: Unique assignment ID
- `_assignmentsections[]`: Array of section-specific assignment details (usually 1 per assignment)

### Assignment Section Level
- `name`: Assignment name
- `description`: Optional HTML description
- `duedate`: Due date (YYYY-MM-DD format)
- `scoretype`: "PERCENT" or "COLLECTED"
- `scoreentrypoints`: Points for score entry
- `totalpointvalue`: Total possible points
- `weight`: Assignment weight (usually 1.0)
- `iscountedinfinalgrade`: Whether it counts toward final grade

### Category Association
- `_teachercategory.name`: Category name (e.g., "Classwork", "Pre - Reading", "Warm Ups")
- `_teachercategory.color`: Color code (1-7)

### Assignment Score
- `scorepoints`: Points earned
- `scorepercent`: Percentage earned
- `scorelettergrade`: Letter grade (A, B, C, etc.)
- `actualscoreentered`: Raw score entered by teacher
- `actualscorekind`: "REAL_SCORE" or other kind
- `islate`, `ismissing`, `isincomplete`, `isabsent`, `isexempt`: Boolean flags
- `iscollected`: Whether assignment was collected (for COLLECTED scoretype)
- `_assignmentscorecomment.commentvalue`: The teacher's note on this score
  (see "Score Comment" below). Usually absent.

### Category Description
- `_teachercategory.description`: Optional description of the category;
  exposed as `Assignment.CategoryDescription` for the primary category.

## Special Cases

### Score Comment (the "notes" icon)

A score a teacher has left a note on carries one more member (the content
here is invented):

```json
{
  "_assignmentscores": [
    {
      "scorepoints": 8.0,
      "_assignmentscorecomment": {
        "_name": "assignmentscorecomment",
        "commentvalue": "<p>Good start.</p><p>Redo question&nbsp;3:<br>show the steps.</p>"
      }
    }
  ]
}
```

Where this comes from: the portal's own static scripts for the class score
page. The page's controller sets a `hasScoreComment` flag on each row when
`_assignmentscorecomment` and its `commentvalue` are both non-null, the row
shows a notes icon for it, and the assignment detail dialog renders
`commentvalue` with an HTML binding under `white-space: pre-wrap`. So the
value may hold markup and character references, and line breaks typed into it
are meant.

PowerSchool omits null members: a score without a note has no
`_assignmentscorecomment` key at all (38 assignments / 30 scores of a live
account were looked at, key names only; none had the key, and the captured
fixture has none either). The library accepts the key absent, the object
null, the value null, empty or not a string, and any extra members, and
reports "no comment" for all of them.

**Not yet seen live:** a response that actually contains a comment. The
decoding is fixture-tested with invented content only. In particular the
members of `_assignmentscorecomment` other than `commentvalue` (an author, a
timestamp) are unknown, so the library exposes no "when was this written".

How the library turns the value into `Assignment.TeacherComment`,
`TeacherCommentHTML`, `HasTeacherComment` and `TeacherCommentTruncated` is
described in the README ("Teacher comments on an assignment").

### More Than One Section Or Score

`GetAssignments` returns one `Assignment` per `_assignmentsections` entry and
reads that section's **first** `_assignmentscores` entry, for the points, the
flags and the comment alike. The lookup is issued for one section and one
student, so one of each is what a response normally holds; further scores are
ignored, and a comment is never taken from a different score than the points
next to it.

### Collected Assignments (No Score)
```json
{
  "scoretype": "COLLECTED",
  "_assignmentscores": [
    {
      "iscollected": true,
      "islate": false,
      "isexempt": false,
      "ismissing": false,
      "isincomplete": false,
      "isabsent": false
    }
  ]
}
```

### Exempt Assignments
```json
{
  "_assignmentscores": [
    {
      "isexempt": true,
      "iscollected": false
    }
  ]
}
```

## How to Get section_ids

From the grades table on the home page, the `frn` parameter in course links corresponds to `section_ids`.

Example: `/guardian/scores.html?frn=00111222333` means `section_ids: [111222333]`

Note: The `frn` parameter may have leading zeros that should be stripped when converting to integer for the API.
