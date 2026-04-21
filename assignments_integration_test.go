//go:build integration

package powerschool_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/leftathome/powerschool-go"
)

// TestIntegration_GetAssignments exercises the full pipeline against a live
// PowerSchool instance — auth → students → grades → assignments for the
// first course that returns a non-empty list.
//
// Gated behind the `integration` build tag so `go test ./...` stays hermetic;
// run with:
//
//	go test -tags integration -run Integration -v ./...
//
// Requires POWERSCHOOL_HOST, POWERSCHOOL_USERNAME, POWERSCHOOL_PASSWORD in
// the environment. Assertions are shape-level only so the test survives a
// school-year rollover — we never check specific course names or scores.
func TestIntegration_GetAssignments(t *testing.T) {
	host := os.Getenv("POWERSCHOOL_HOST")
	username := os.Getenv("POWERSCHOOL_USERNAME")
	password := os.Getenv("POWERSCHOOL_PASSWORD")
	if host == "" || username == "" || password == "" {
		t.Skip("POWERSCHOOL_HOST/USERNAME/PASSWORD must be set for the integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	client, err := powerschool.NewClient(
		"https://"+host,
		powerschool.WithCredentials(username, password),
		powerschool.WithLogLevel(powerschool.LogLevelInfo),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	headless := true
	if err := client.AuthenticateWithOptions(ctx, &powerschool.AuthOptions{
		Headless:  &headless,
		Timeout:   90 * time.Second,
		NoSandbox: os.Geteuid() == 0,
	}); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	students, err := client.GetStudents(ctx)
	if err != nil {
		t.Fatalf("GetStudents: %v", err)
	}
	if len(students) == 0 {
		t.Fatal("GetStudents returned 0 students")
	}
	var student *powerschool.Student
	for _, s := range students {
		if s.StudentNumber != "" {
			student = s
			break
		}
	}
	if student == nil {
		student = students[0]
	}

	grades, err := client.GetGrades(ctx, student.ID)
	if err != nil {
		t.Fatalf("GetGrades: %v", err)
	}
	if len(grades) == 0 {
		t.Fatal("GetGrades returned 0 grades")
	}

	// Try up to 5 courses; stop at the first non-empty assignment list.
	var picked *powerschool.Grade
	var assignments []*powerschool.Assignment
	tried := 0
	for _, g := range grades {
		if g.CourseID == "" || g.ScoresURL == "" {
			continue
		}
		tried++
		as, err := client.GetAssignments(ctx, student.ID, g)
		if err != nil {
			t.Logf("GetAssignments for %q (frn=%s) failed: %v", g.CourseName, g.CourseID, err)
			if tried >= 5 {
				break
			}
			continue
		}
		if len(as) > 0 {
			picked = g
			assignments = as
			break
		}
		if tried >= 5 {
			break
		}
	}
	if picked == nil {
		t.Fatalf("tried %d courses, none returned assignments", tried)
	}

	t.Logf("picked course %q (frn=%s, section=%s), got %d assignments",
		picked.CourseName, picked.CourseID, picked.SectionID, len(assignments))

	// Shape-level checks only — resilient to data changes year over year.
	var titled, withDueDate int
	for _, a := range assignments {
		if a.Title != "" {
			titled++
		}
		if !a.DueDate.IsZero() {
			withDueDate++
		}
		if a.CourseID != picked.CourseID {
			t.Errorf("assignment %s CourseID = %q, want %q", a.ID, a.CourseID, picked.CourseID)
		}
	}
	if titled == 0 {
		t.Error("no assignment had a title")
	}
	if withDueDate == 0 {
		t.Error("no assignment had a parseable due date")
	}
	if picked.SectionID == "" {
		t.Error("GetAssignments should have cached SectionID on the grade")
	}
	if picked.StudentAPIID == "" {
		t.Error("GetAssignments should have cached StudentAPIID on the grade")
	}
}
