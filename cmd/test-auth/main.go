package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/leftathome/powerschool-go"
)

func main() {
	// Command line flags
	var (
		baseURL   = flag.String("url", os.Getenv("POWERSCHOOL_URL"), "PowerSchool base URL")
		username  = flag.String("username", os.Getenv("POWERSCHOOL_USERNAME"), "Username")
		password  = flag.String("password", os.Getenv("POWERSCHOOL_PASSWORD"), "Password")
		headless  = flag.Bool("headless", false, "Run browser in headless mode")
		debugLog  = flag.Bool("debug", false, "Enable debug logging")
		saveHTML  = flag.Bool("save-html", false, "Save HTML responses to files")
		noSandbox = flag.Bool("no-sandbox", os.Geteuid() == 0, "Pass --no-sandbox to Chrome (auto-enabled when running as root)")
	)
	flag.Parse()

	// Validate inputs
	if *baseURL == "" {
		log.Fatal("PowerSchool URL is required (use -url or POWERSCHOOL_URL env var)")
	}
	if *username == "" {
		log.Fatal("Username is required (use -username or POWERSCHOOL_USERNAME env var)")
	}
	if *password == "" {
		log.Fatal("Password is required (use -password or POWERSCHOOL_PASSWORD env var)")
	}

	fmt.Println("=================================================================")
	fmt.Println("PowerSchool Authentication Test")
	fmt.Println("=================================================================")
	fmt.Printf("URL: %s\n", *baseURL)
	fmt.Printf("Username: %s\n", *username)
	fmt.Printf("Password: %s\n", maskPassword(*password))
	fmt.Printf("Headless: %v\n", *headless)
	fmt.Printf("Debug: %v\n", *debugLog)
	fmt.Println("=================================================================")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Set log level
	logLevel := powerschool.LogLevelInfo
	if *debugLog {
		logLevel = powerschool.LogLevelDebug
	}

	// Create client
	client, err := powerschool.NewClient(
		*baseURL,
		powerschool.WithCredentials(*username, *password),
		powerschool.WithLogLevel(logLevel),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fmt.Println("\nClient created successfully")
	fmt.Printf("Client: %s\n", client)

	// Configure authentication options
	authOpts := &powerschool.AuthOptions{
		Headless:  headless,
		DebugLog:  *debugLog,
		Timeout:   90 * time.Second,
		NoSandbox: *noSandbox,
	}

	// Attempt authentication
	fmt.Println("\n=================================================================")
	fmt.Println("Attempting authentication...")
	if !*headless {
		fmt.Println("Browser will be VISIBLE - watch for login process")
	}
	fmt.Println("=================================================================")

	startTime := time.Now()
	err = client.AuthenticateWithOptions(ctx, authOpts)
	duration := time.Since(startTime)

	if err != nil {
		fmt.Println("\n!!! AUTHENTICATION FAILED !!!")
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("Duration: %s\n", duration)

		// Check error type
		if powerschool.IsAuthError(err) {
			fmt.Println("\nThis appears to be an authentication error.")
			fmt.Println("Possible causes:")
			fmt.Println("  - Incorrect username or password")
			fmt.Println("  - Login form selectors don't match")
			fmt.Println("  - Additional authentication steps (2FA, captcha)")
		}

		os.Exit(1)
	}

	fmt.Println("\n=================================================================")
	fmt.Println("AUTHENTICATION SUCCESSFUL!")
	fmt.Println("=================================================================")
	fmt.Printf("Duration: %s\n", duration)

	// Display session information
	if session := client.GetSession(); session != nil {
		fmt.Println("\nSession Information:")
		fmt.Printf("  Expires at: %s\n", client.GetSessionExpiry().Format(time.RFC3339))
		fmt.Printf("  Time until expiry: %s\n", time.Until(client.GetSessionExpiry()).Round(time.Minute))
		fmt.Printf("  Number of cookies: %d\n", len(session.GetCookies()))

		if *debugLog {
			fmt.Println("\nSession Cookies:")
			for i, cookie := range session.GetCookies() {
				fmt.Printf("    %d. %s = %s (domain: %s, path: %s)\n",
					i+1, cookie.Name, maskCookieValue(cookie.Value), cookie.Domain, cookie.Path)
			}
		}
	}

	// Test basic data retrieval
	fmt.Println("\n=================================================================")
	fmt.Println("Testing data retrieval...")
	fmt.Println("=================================================================")

	fmt.Println("\nFetching students...")
	students, err := client.GetStudents(ctx)
	if err != nil {
		fmt.Printf("Failed to fetch students: %v\n", err)
		if *debugLog {
			fmt.Println("\nThis is expected - we may need to adjust HTML selectors")
		}
	} else {
		fmt.Printf("Successfully fetched %d student(s)\n", len(students))
		for i, student := range students {
			fmt.Printf("  %d. %s (ID: %s, Grade: %d)\n",
				i+1, student.Name, student.ID, student.GradeLevel)
			if student.StudentNumber != "" {
				fmt.Printf("      Student #: %s\n", student.StudentNumber)
			}
			if student.StateID != "" {
				fmt.Printf("      State ID: %s\n", student.StateID)
			}
			if student.SchoolName != "" {
				fmt.Printf("      School: %s\n", student.SchoolName)
			}
			if student.PortalUsername != "" {
				fmt.Printf("      Portal Username: %s\n", student.PortalUsername)
			}
		}

		// Fetch grades for the first student (currently displayed student)
		if len(students) > 0 {
			fmt.Println("\nFetching grades for currently-displayed student...")
			firstStudent := students[0]
			// Try to find the student with complete details (the one currently displayed)
			for _, s := range students {
				if s.StudentNumber != "" {
					firstStudent = s
					break
				}
			}

			grades, err := client.GetGrades(ctx, firstStudent.ID)
			if err != nil {
				fmt.Printf("Failed to fetch grades: %v\n", err)
			} else {
				fmt.Printf("Successfully fetched %d grade(s) for %s\n", len(grades), firstStudent.Name)
				for i, grade := range grades {
					fmt.Printf("  %d. %s (Period %s)\n", i+1, grade.CourseName, grade.Period)
					if grade.Teacher != "" {
						fmt.Printf("      Teacher: %s", grade.Teacher)
						if grade.RoomNumber != "" {
							fmt.Printf(" (Room %s)", grade.RoomNumber)
						}
						fmt.Println()
					}
					if grade.CurrentGrade != "" {
						fmt.Printf("      Current Grade: %s\n", grade.CurrentGrade)
					}
					if grade.Q1Grade != "" {
						fmt.Printf("      Q1: %s", grade.Q1Grade)
					}
					if grade.Q2Grade != "" {
						fmt.Printf(" | Q2: %s", grade.Q2Grade)
					}
					if grade.S1Grade != "" {
						fmt.Printf(" | S1: %s", grade.S1Grade)
					}
					if grade.Q1Grade != "" || grade.Q2Grade != "" || grade.S1Grade != "" {
						fmt.Println()
					}
					if grade.Absences > 0 || grade.Tardies > 0 {
						fmt.Printf("      Attendance: %d absences, %d tardies\n", grade.Absences, grade.Tardies)
					}
				}

				// Fetch assignments for each course that has one. We stop
				// after the first course that returns a non-empty list so
				// we exercise the parsing path without hammering the API.
				coursesTried := 0
				for _, grade := range grades {
					if grade.CourseID == "" {
						continue
					}
					coursesTried++
					fmt.Printf("\nFetching assignments for %s (frn=%s)...\n", grade.CourseName, grade.CourseID)
					assignments, err := client.GetAssignments(ctx, firstStudent.ID, grade)
					if err != nil {
						fmt.Printf("Failed to fetch assignments: %v\n", err)
						if coursesTried >= 4 {
							break
						}
						continue
					}
					fmt.Printf("Successfully fetched %d assignment(s)\n", len(assignments))
					for i, assignment := range assignments {
						fmt.Printf("  %d. %s\n", i+1, assignment.Title)
						if assignment.Category != "" {
							fmt.Printf("      Category: %s\n", assignment.Category)
						}
						if !assignment.DueDate.IsZero() {
							fmt.Printf("      Due Date: %s\n", assignment.DueDate.Format("01/02/2006"))
						}
						if assignment.Score != nil {
							fmt.Printf("      Score: %.0f/%.0f", *assignment.Score, assignment.MaxScore)
							if assignment.Percentage != nil {
								fmt.Printf(" (%.0f%%)", *assignment.Percentage)
							}
							if assignment.LetterGrade != "" {
								fmt.Printf(" - %s", assignment.LetterGrade)
							}
							fmt.Println()
						}
						if len(assignment.Flags) > 0 {
							fmt.Printf("      Flags: %v\n", assignment.Flags)
						}
						if i >= 4 {
							fmt.Printf("      ... and %d more assignments\n", len(assignments)-5)
							break
						}
					}
					if len(assignments) > 0 {
						break
					}
					if coursesTried >= 4 {
						fmt.Println("Tried 4 courses without finding assignments; stopping.")
						break
					}
				}
			}
		}
	}

	// Export session for reuse
	if *saveHTML {
		sessionExport := client.ExportSession()
		if sessionExport != nil {
			fmt.Println("\n=================================================================")
			fmt.Println("Session can be exported and reused")
			fmt.Println("See examples/session for session persistence example")
			fmt.Println("=================================================================")
		}
	}

	fmt.Println("\n=================================================================")
	fmt.Println("Test completed successfully!")
	fmt.Println("=================================================================")
}

func maskPassword(password string) string {
	if len(password) <= 2 {
		return "***"
	}
	return string(password[0]) + "***" + string(password[len(password)-1])
}

func maskCookieValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
