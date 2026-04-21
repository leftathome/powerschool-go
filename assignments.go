package powerschool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

// GetAssignments returns all assignments for the given student + course via
// the /ws/xte/assignment/lookup API. The caller must pass a *Grade so that
// Grade.ScoresURL is available — the scores page requires a fully-qualified
// URL (frn+begdate+enddate+fg+schoolid) and that URL is also needed as the
// Referer on the assignment-lookup POST.
func (c *Client) GetAssignments(ctx context.Context, studentID string, grade *Grade) ([]*Assignment, error) {
	if grade == nil {
		return nil, fmt.Errorf("grade is required")
	}
	if grade.ScoresURL == "" {
		return nil, fmt.Errorf("grade.ScoresURL is empty (was this Grade obtained via GetGrades?)")
	}

	courseID := grade.CourseID
	c.logger.Info("Fetching assignments for student ID: %s, course ID: %s", studentID, courseID)

	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// The assignment-lookup API wants the studentFRN-derived DCID, not the
	// switchStudent() nav ID passed in as studentID. Pull both the section
	// and the API-facing student ID out of the scores page, caching them
	// on the grade so subsequent calls skip the (browser-rendered) fetch.
	if grade.SectionID == "" || grade.StudentAPIID == "" {
		metadata, err := c.FetchScoresMetadata(ctx, grade.ScoresURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch scores metadata: %w", err)
		}
		grade.SectionID = metadata.SectionID
		grade.StudentAPIID = metadata.StudentAPIID
	}

	sectionIDInt, err := strconv.ParseInt(grade.SectionID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid section ID %q: %w", grade.SectionID, err)
	}
	studentIDInt, err := strconv.ParseInt(grade.StudentAPIID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid student API ID %q: %w", grade.StudentAPIID, err)
	}
	_ = studentID // accepted for API parity; effective value comes from the scores page

	// Get date range (current school year - Sept 1 to June 30)
	now := time.Now()
	startDate := time.Date(now.Year(), 9, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(now.Year()+1, 6, 30, 0, 0, 0, 0, time.Local)

	// If we're before September, use previous year
	if now.Month() < 9 {
		startDate = startDate.AddDate(-1, 0, 0)
		endDate = endDate.AddDate(-1, 0, 0)
	}

	// Build API request payload
	payload := map[string]interface{}{
		"section_ids":  []int64{sectionIDInt},
		"student_ids":  []int64{studentIDInt},
		"start_date":   startDate.Format("2006-1-2"),
		"end_date":     endDate.Format("2006-1-2"),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	c.logger.Debug("API request payload: %s", string(payloadJSON))

	// The server rejects this POST with 400 + HTML when Referer is missing.
	// Any scores.html URL on the same host satisfies the check, so we use
	// the captured Grade.ScoresURL so the Referer matches what a browser
	// would send.
	path := fmt.Sprintf("/ws/xte/assignment/lookup?_=%d", time.Now().UnixMilli())
	headers := map[string]string{
		"Referer": c.baseURL + grade.ScoresURL,
	}

	resp, err := c.doRequestWithHeaders(ctx, "POST", path, payloadJSON, "application/json;charset=UTF-8", headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Debug("API error response (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, truncateString(string(body), 200))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapNetworkError(err)
	}

	c.logger.Debug("API response (first 500 chars): %s", truncateString(string(body), 500))

	// Parse JSON response
	var apiAssignments []APIAssignment
	if err := json.Unmarshal(body, &apiAssignments); err != nil {
		c.logger.Debug("Failed to parse JSON, response was: %s", truncateString(string(body), 1000))
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Convert API response to Assignment objects
	assignments := make([]*Assignment, 0)
	for _, apiAssignment := range apiAssignments {
		// Each assignment can have multiple sections, but usually just one
		for _, section := range apiAssignment.AssignmentSections {
			assignment := &Assignment{
				ID:          fmt.Sprintf("%d", apiAssignment.AssignmentID),
				CourseID:    courseID,
				Title:       section.Name,
				Description: stripHTMLTags(section.Description),
				Category:    "",
				MaxScore:    section.ScoreEntryPoints,
				Status:      StatusPending,
			}

			// Parse due date
			if section.DueDate != "" {
				dueDate, err := time.Parse("2006-01-02", section.DueDate)
				if err == nil {
					assignment.DueDate = dueDate
				}
			}

			// Get category name
			if len(section.CategoryAssociations) > 0 {
				for _, assoc := range section.CategoryAssociations {
					if assoc.IsPrimary && assoc.TeacherCategory != nil {
						assignment.Category = assoc.TeacherCategory.Name
						break
					}
				}
			}

			// Get score information (if any)
			if len(section.AssignmentScores) > 0 {
				score := section.AssignmentScores[0]

				// Build flags array
				flags := make([]AssignmentFlag, 0)
				if score.IsLate {
					flags = append(flags, FlagLate)
				}
				if score.IsMissing {
					flags = append(flags, FlagMissing)
				}
				if score.IsIncomplete {
					flags = append(flags, FlagIncomplete)
				}
				if score.IsAbsent {
					flags = append(flags, FlagAbsent)
				}
				if score.IsExempt {
					flags = append(flags, FlagExempt)
				}
				if score.IsCollected {
					flags = append(flags, FlagCollected)
				}
				assignment.Flags = flags

				// Determine status based on flags
				if score.IsExempt {
					assignment.Status = StatusExempt
				} else if score.IsMissing {
					assignment.Status = StatusMissing
				} else if score.IsIncomplete {
					assignment.Status = StatusIncomplete
				} else if score.IsAbsent {
					assignment.Status = StatusAbsent
				} else if score.IsCollected {
					assignment.Status = StatusCollected
				} else if score.ScorePoints > 0 || score.ActualScoreEntered != "" {
					assignment.Status = StatusGraded
				}

				// Set score if present
				if score.ScorePoints > 0 {
					points := score.ScorePoints
					assignment.Score = &points
				}

				// Set percentage
				if score.ScorePercent > 0 {
					pct := score.ScorePercent
					assignment.Percentage = &pct
				}

				// Set letter grade
				assignment.LetterGrade = score.ScoreLetterGrade
			}

			assignments = append(assignments, assignment)
		}
	}

	c.logger.Info("Found %d assignment(s)", len(assignments))
	return assignments, nil
}

// GetAssignmentCategories returns assignment categories with aggregate totals.
// Derived from GetAssignments, so the same *Grade requirement applies.
func (c *Client) GetAssignmentCategories(ctx context.Context, studentID string, grade *Grade) ([]*AssignmentCategory, error) {
	if grade == nil {
		return nil, fmt.Errorf("grade is required")
	}
	c.logger.Info("Fetching assignment categories for course ID: %s", grade.CourseID)

	assignments, err := c.GetAssignments(ctx, studentID, grade)
	if err != nil {
		return nil, err
	}

	// Aggregate by category
	categoryMap := make(map[string]*AssignmentCategory)

	for _, assignment := range assignments {
		if assignment.Category == "" {
			continue
		}

		cat, exists := categoryMap[assignment.Category]
		if !exists {
			cat = &AssignmentCategory{
				Name:           assignment.Category,
				Count:          0,
				PointsPossible: 0,
				PointsEarned:   0,
				Percentage:     0,
			}
			categoryMap[assignment.Category] = cat
		}

		cat.Count++
		cat.PointsPossible += assignment.MaxScore

		if assignment.Score != nil && assignment.Status == StatusGraded {
			cat.PointsEarned += *assignment.Score
		}
	}

	// Calculate percentages and convert to slice
	categories := make([]*AssignmentCategory, 0, len(categoryMap))
	for _, cat := range categoryMap {
		if cat.PointsPossible > 0 {
			cat.Percentage = (cat.PointsEarned / cat.PointsPossible) * 100
		}
		categories = append(categories, cat)
	}

	c.logger.Info("Found %d categor(ies)", len(categories))
	return categories, nil
}

// APIAssignment represents the API response structure for an assignment
type APIAssignment struct {
	Name                  string                      `json:"_name"`
	ID                    int64                       `json:"_id"`
	AssignmentID          int64                       `json:"assignmentid"`
	HasStandards          bool                        `json:"hasstandards"`
	StandardScoringMethod string                      `json:"standardscoringmethod"`
	AssignmentSections    []APIAssignmentSection      `json:"_assignmentsections"`
}

// APIAssignmentSection represents a section-specific assignment
type APIAssignmentSection struct {
	Name                   string                          `json:"name"`
	Description            string                          `json:"description"`
	DueDate                string                          `json:"duedate"`
	ScoreType              string                          `json:"scoretype"`
	ScoreEntryPoints       float64                         `json:"scoreentrypoints"`
	TotalPointValue        float64                         `json:"totalpointvalue"`
	Weight                 float64                         `json:"weight"`
	IsCountedInFinalGrade  bool                            `json:"iscountedinfinalgrade"`
	IsScoresPublish        bool                            `json:"isscorespublish"`
	IsScoringNeeded        bool                            `json:"isscoringneeded"`
	SectionsDCID           int64                           `json:"sectionsdcid"`
	AssignmentSectionID    int64                           `json:"assignmentsectionid"`
	CategoryAssociations   []APICategoryAssociation        `json:"_assignmentcategoryassociations"`
	AssignmentScores       []APIAssignmentScore            `json:"_assignmentscores"`
}

// APICategoryAssociation represents the category association
type APICategoryAssociation struct {
	IsPrimary       bool                `json:"isprimary"`
	TeacherCategory *APITeacherCategory `json:"_teachercategory"`
}

// APITeacherCategory represents a teacher-defined category
type APITeacherCategory struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// APIAssignmentScore represents a student's score on an assignment
type APIAssignmentScore struct {
	ScorePoints         float64 `json:"scorepoints"`
	ScorePercent        float64 `json:"scorepercent"`
	ScoreLetterGrade    string  `json:"scorelettergrade"`
	ActualScoreEntered  string  `json:"actualscoreentered"`
	ActualScoreKind     string  `json:"actualscorekind"`
	ScoreEntryDate      string  `json:"scoreentrydate"`
	WhenModified        string  `json:"whenmodified"`
	IsLate              bool    `json:"islate"`
	IsMissing           bool    `json:"ismissing"`
	IsIncomplete        bool    `json:"isincomplete"`
	IsAbsent            bool    `json:"isabsent"`
	IsExempt            bool    `json:"isexempt"`
	IsCollected         bool    `json:"iscollected"`
	AuthoredByUC        bool    `json:"authoredbyuc"`
	StudentsDCID        int64   `json:"studentsdcid"`
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
