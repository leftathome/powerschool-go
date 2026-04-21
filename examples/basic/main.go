package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/leftathome/powerschool-go"
)

func main() {
	// Get credentials from environment variables
	baseURL := os.Getenv("POWERSCHOOL_URL")
	username := os.Getenv("POWERSCHOOL_USERNAME")
	password := os.Getenv("POWERSCHOOL_PASSWORD")

	if baseURL == "" || username == "" || password == "" {
		log.Fatal("Please set POWERSCHOOL_URL, POWERSCHOOL_USERNAME, and POWERSCHOOL_PASSWORD environment variables")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create client
	client, err := powerschool.NewClient(
		baseURL,
		powerschool.WithCredentials(username, password),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Authenticate
	fmt.Println("Authenticating...")
	if err := client.Authenticate(ctx); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	fmt.Println("Authenticated successfully!")

	// Get students
	fmt.Println("\nFetching students...")
	students, err := client.GetStudents(ctx)
	if err != nil {
		log.Fatalf("Failed to get students: %v", err)
	}

	if len(students) == 0 {
		fmt.Println("No students found")
		return
	}

	// Display student information
	for _, student := range students {
		fmt.Printf("\n%s\n", divider("=", 80))
		fmt.Printf("Student: %s\n", student.Name)
		fmt.Printf("ID: %s\n", student.ID)
		if student.GradeLevel > 0 {
			fmt.Printf("Grade Level: %d\n", student.GradeLevel)
		}
		if student.SchoolName != "" {
			fmt.Printf("School: %s\n", student.SchoolName)
		}

		// Get grades
		fmt.Println("\nGrades:")
		grades, err := client.GetGrades(ctx, student.ID)
		if err != nil {
			fmt.Printf("  Error fetching grades: %v\n", err)
			continue
		}

		if len(grades) == 0 {
			fmt.Println("  No grades found")
		} else {
			for _, grade := range grades {
				fmt.Printf("  %-40s %s (%.1f%%)\n",
					truncate(grade.CourseName, 40),
					grade.LetterGrade,
					grade.Percentage)
			}
		}

		// Get GPA
		gpa, err := client.GetGPA(ctx, student.ID)
		if err == nil && (gpa.Current > 0 || gpa.Cumulative > 0) {
			fmt.Println("\nGPA:")
			if gpa.Current > 0 {
				fmt.Printf("  Current: %.2f", gpa.Current)
				if gpa.Weighted {
					fmt.Print(" (weighted)")
				}
				fmt.Println()
			}
			if gpa.Cumulative > 0 {
				fmt.Printf("  Cumulative: %.2f", gpa.Cumulative)
				if gpa.Weighted {
					fmt.Print(" (weighted)")
				}
				fmt.Println()
			}
		}

		// NOTE: Assignment fetching requires a course ID (frn parameter)
		// Example commented out - see test-auth program for working example
		// assignments, err := client.GetAssignments(ctx, student.ID, courseID)
		fmt.Println("\nAssignments: (See test-auth program for implementation)")
	}

	fmt.Printf("\n%s\n", divider("=", 80))
}

func divider(char string, length int) string {
	result := ""
	for i := 0; i < length; i++ {
		result += char
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
