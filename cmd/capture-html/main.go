package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leftathome/powerschool-go"
	"github.com/leftathome/powerschool-go/internal/browser"
)

func main() {
	// Command line flags
	var (
		baseURL  = flag.String("url", os.Getenv("POWERSCHOOL_URL"), "PowerSchool base URL")
		username = flag.String("username", os.Getenv("POWERSCHOOL_USERNAME"), "Username")
		password = flag.String("password", os.Getenv("POWERSCHOOL_PASSWORD"), "Password")
		outDir   = flag.String("output", "html-captures", "Output directory for HTML files")
	)
	flag.Parse()

	// Validate inputs
	if *baseURL == "" || *username == "" || *password == "" {
		log.Fatal("URL, username, and password are required")
	}

	fmt.Println("=================================================================")
	fmt.Println("PowerSchool HTML Capture Tool")
	fmt.Println("=================================================================")
	fmt.Printf("URL: %s\n", *baseURL)
	fmt.Printf("Username: %s\n", *username)
	fmt.Printf("Output directory: %s\n", *outDir)
	fmt.Println("=================================================================")

	// Create output directory
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create client
	client, err := powerschool.NewClient(
		*baseURL,
		powerschool.WithCredentials(*username, *password),
		powerschool.WithLogLevel(powerschool.LogLevelInfo),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Authenticate
	fmt.Println("\nAuthenticating...")
	headless := false
	authOpts := &powerschool.AuthOptions{
		Headless: &headless,
		Timeout:  90 * time.Second,
	}

	if err := client.AuthenticateWithOptions(ctx, authOpts); err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	fmt.Println("✓ Authentication successful")

	// List of pages to capture
	pages := []struct {
		name string
		path string
		desc string
	}{
		{"home", "/guardian/home.html", "Home/Dashboard page"},
		{"students", "/guardian/studentdata.html", "Student data page"},
		{"scores", "/guardian/scores.html", "Scores/grades page"},
	}

	fmt.Println("\n=================================================================")
	fmt.Println("Capturing HTML from pages...")
	fmt.Println("=================================================================")

	for _, page := range pages {
		fmt.Printf("\nCapturing: %s (%s)\n", page.desc, page.path)

		// Use internal method to get HTML (we'll need to expose this)
		// For now, let's use a workaround
		html, err := capturePageHTML(ctx, client, page.path)
		if err != nil {
			fmt.Printf("  ✗ Failed: %v\n", err)
			continue
		}

		// Save to file
		filename := filepath.Join(*outDir, fmt.Sprintf("%s.html", page.name))
		if err := os.WriteFile(filename, []byte(html), 0644); err != nil {
			fmt.Printf("  ✗ Failed to save: %v\n", err)
			continue
		}

		fmt.Printf("  ✓ Saved to: %s (%d bytes)\n", filename, len(html))

		// Also save a snippet for quick viewing
		snippetFile := filepath.Join(*outDir, fmt.Sprintf("%s-snippet.txt", page.name))
		snippet := extractSnippet(html, 2000)
		if err := os.WriteFile(snippetFile, []byte(snippet), 0644); err == nil {
			fmt.Printf("  ✓ Snippet saved to: %s\n", snippetFile)
		}
	}

	fmt.Println("\n=================================================================")
	fmt.Println("Capture complete!")
	fmt.Println("=================================================================")
	fmt.Printf("\nHTML files saved to: %s\n", *outDir)
	fmt.Println("\nNext steps:")
	fmt.Println("1. Open the HTML files in a text editor")
	fmt.Println("2. Search for student names, grades, assignments")
	fmt.Println("3. Identify the HTML structure (classes, ids, tags)")
	fmt.Println("4. Update the parser code with correct selectors")
}

// capturePageHTML captures HTML from a page using the client's session
func capturePageHTML(ctx context.Context, client *powerschool.Client, path string) (string, error) {
	// We need to expose getPageHTML method or use browser package directly
	// For now, let's use the internal browser package
	session := client.GetSession()
	if session == nil {
		return "", fmt.Errorf("no valid session")
	}

	fullURL := client.GetBaseURL() + path

	// Use browser package to get HTML
	html, err := getPageContent(ctx, fullURL, session.GetCookies())
	if err != nil {
		return "", fmt.Errorf("failed to get page content: %w", err)
	}

	return html, nil
}

// getPageContent is a simplified version that doesn't require importing internal package
// We'll use a simple HTTP request with cookies
func getPageContent(ctx context.Context, url string, cookies []*http.Cookie) (string, error) {
	// Import browser package functionality
	return browser.GetPageContent(ctx, url, cookies, 30*time.Second)
}

// extractSnippet extracts a readable snippet from HTML
func extractSnippet(html string, maxLen int) string {
	if len(html) <= maxLen {
		return html
	}

	snippet := html[:maxLen]

	// Try to find key sections
	sections := []string{
		"student",
		"grade",
		"assignment",
		"class",
		"course",
	}

	info := fmt.Sprintf("HTML Length: %d bytes\n\n", len(html))
	info += "Key sections found:\n"

	for _, section := range sections {
		// Simple case-insensitive search
		if containsIgnoreCase(html, section) {
			info += fmt.Sprintf("  - Contains '%s'\n", section)
		}
	}

	info += "\n--- First " + fmt.Sprintf("%d", maxLen) + " characters ---\n"
	info += snippet

	return info
}

func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
