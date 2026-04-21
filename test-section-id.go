// +build ignore

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
	baseURL := os.Getenv("POWERSCHOOL_URL")
	username := os.Getenv("POWERSCHOOL_USERNAME")
	password := os.Getenv("POWERSCHOOL_PASSWORD")

	if baseURL == "" || username == "" || password == "" {
		log.Fatal("Required environment variables not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create client with debug logging
	client, err := powerschool.NewClient(
		baseURL,
		powerschool.WithCredentials(username, password),
		powerschool.WithLogLevel(powerschool.LogLevelDebug),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Authenticate
	headless := true
	debugLog := false
	authOpts := &powerschool.AuthOptions{
		Headless: &headless,
		DebugLog: &debugLog,
		Timeout:  90 * time.Second,
	}

	if err := client.AuthenticateWithOptions(ctx, authOpts); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	fmt.Println("Authentication successful!")

	// FetchSectionID needs the full scores URL (frn+begdate+enddate+fg+schoolid)
	// — partial URLs redirect to the login page. In real usage you get this
	// from Grade.ScoresURL after a GetGrades call; hard-coded here for a
	// standalone probe.
	scoresURL := "/guardian/scores.html?frn=00111222333&begdate=09/03/2025&enddate=11/05/2025&fg=Q1&schoolid=109"
	fmt.Printf("\nFetching section ID from %s ...\n", scoresURL)

	sectionID, err := client.FetchSectionID(ctx, scoresURL)
	if err != nil {
		log.Fatalf("Failed to fetch section ID: %v", err)
	}

	fmt.Printf("SUCCESS! Section ID: %s\n", sectionID)
}
