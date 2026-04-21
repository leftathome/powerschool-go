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

## Special Cases

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
