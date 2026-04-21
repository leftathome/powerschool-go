package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/leftathome/powerschool-go"
)

const sessionFile = "powerschool_session.json"

func main() {
	baseURL := os.Getenv("POWERSCHOOL_URL")
	username := os.Getenv("POWERSCHOOL_USERNAME")
	password := os.Getenv("POWERSCHOOL_PASSWORD")

	if baseURL == "" {
		log.Fatal("Please set POWERSCHOOL_URL environment variable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var client *powerschool.Client
	var err error

	// Try to load existing session
	if sessionExport, err := loadSession(); err == nil {
		fmt.Println("Found existing session, attempting to use it...")

		client, err = powerschool.NewClient(baseURL)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}

		if err := client.ImportSession(sessionExport); err != nil {
			fmt.Printf("Failed to import session: %v\n", err)
			client = nil
		} else {
			fmt.Printf("Session imported successfully, expires at: %s\n",
				sessionExport.ExpiresAt.Format(time.RFC3339))

			// Test the session by fetching students
			if _, err := client.GetStudents(ctx); err != nil {
				fmt.Printf("Session test failed: %v\n", err)
				fmt.Println("Session appears to be invalid, will re-authenticate")
				client = nil
			} else {
				fmt.Println("Session is valid!")
			}
		}
	} else {
		fmt.Println("No existing session found")
	}

	// If no valid session, authenticate with credentials
	if client == nil {
		if username == "" || password == "" {
			log.Fatal("Please set POWERSCHOOL_USERNAME and POWERSCHOOL_PASSWORD environment variables")
		}

		fmt.Println("Creating new session with credentials...")

		client, err = powerschool.NewClient(
			baseURL,
			powerschool.WithCredentials(username, password),
		)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}

		if err := client.Authenticate(ctx); err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		fmt.Println("Authentication successful!")

		// Save the session
		if err := saveSession(client.ExportSession()); err != nil {
			fmt.Printf("Warning: Failed to save session: %v\n", err)
		} else {
			fmt.Printf("Session saved to %s\n", sessionFile)
		}
	}

	// Use the client
	fmt.Println("\nFetching students...")
	students, err := client.GetStudents(ctx)
	if err != nil {
		log.Fatalf("Failed to get students: %v", err)
	}

	fmt.Printf("Found %d student(s)\n", len(students))
	for _, student := range students {
		fmt.Printf("  - %s (ID: %s)\n", student.Name, student.ID)
	}

	// Show session info
	expiry := client.GetSessionExpiry()
	fmt.Printf("\nSession expires at: %s\n", expiry.Format(time.RFC3339))
	fmt.Printf("Time until expiry: %s\n", time.Until(expiry).Round(time.Minute))
}

func loadSession() (*powerschool.SessionExport, error) {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	var session powerschool.SessionExport
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func saveSession(session *powerschool.SessionExport) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sessionFile, data, 0600)
}
