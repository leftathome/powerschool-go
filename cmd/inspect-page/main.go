package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	baseURL := os.Getenv("POWERSCHOOL_URL")
	username := os.Getenv("POWERSCHOOL_USERNAME")
	password := os.Getenv("POWERSCHOOL_PASSWORD")

	if baseURL == "" || username == "" || password == "" {
		log.Fatal("Required environment variables not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	courseID := "00111222333"
	scoresURL := fmt.Sprintf("%s/guardian/scores.html?frn=%s", baseURL, courseID)

	fmt.Println("Opening visible browser...")
	fmt.Println("The browser will:")
	fmt.Println("  1. Authenticate automatically")
	fmt.Println("  2. Navigate to the scores page")
	fmt.Println("  3. Stay open for your inspection")
	fmt.Println("")

	// Create persistent browser session with visible window
	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", false), // VISIBLE!
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", false),
		chromedp.WindowSize(1920, 1080),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-sync", true),
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Perform login
	loginURL := baseURL + "/public/"
	fmt.Println("Navigating to login page:", loginURL)

	err := chromedp.Run(browserCtx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`input[name="account"]`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("  → Filling in username...")
			return nil
		}),
		chromedp.SendKeys(`input[name="account"]`, username, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("  → Filling in password...")
			return nil
		}),
		chromedp.SendKeys(`input[name="pw"]`, password, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("  → Clicking submit...")
			return nil
		}),
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("  → Waiting for redirect to home page...")
			return nil
		}),
		chromedp.Sleep(3*time.Second), // Give it time to redirect
	)
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	// Check if we're actually logged in by looking for the home link
	var homeLink string
	err = chromedp.Run(browserCtx,
		chromedp.Location(&homeLink),
	)
	if err != nil {
		log.Fatalf("Failed to get current URL: %v", err)
	}

	fmt.Printf("  → Current URL: %s\n", homeLink)

	if strings.Contains(homeLink, "/public/") {
		log.Fatal("ERROR: Still on login page! Login did not succeed.")
	}

	fmt.Println("✓ Logged in!")

	// Navigate to scores page
	fmt.Println("\nNavigating to scores page...")
	fmt.Println("URL:", scoresURL)

	var htmlContent string
	err = chromedp.Run(browserCtx,
		chromedp.Navigate(scoresURL),
		chromedp.Sleep(5*time.Second), // Wait for JavaScript to render
		chromedp.ActionFunc(func(ctx context.Context) error {
			var html string
			err := chromedp.Evaluate(`document.documentElement.outerHTML`, &html).Do(ctx)
			if err != nil {
				return err
			}
			htmlContent = html
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("Failed to navigate to scores: %v", err)
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("BROWSER IS NOW OPEN - Please inspect the page!")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()
	fmt.Println("The page is loaded. Now please check:")
	fmt.Println()
	fmt.Println("  1. In the browser window:")
	fmt.Println("     Right-click → 'View Page Source'")
	fmt.Println("     Press Ctrl+F and search for: data-sectionid")
	fmt.Println("     → Is it in the STATIC HTML source?")
	fmt.Println()
	fmt.Println("  2. In the browser window:")
	fmt.Println("     Press F12 (DevTools)")
	fmt.Println("     Go to 'Elements' tab")
	fmt.Println("     Press Ctrl+F and search for: data-sectionid")
	fmt.Println("     → Is it in the RENDERED DOM?")
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	// Also check programmatically
	if contains := containsDataSectionID(htmlContent); contains {
		fmt.Println("✓ Our code CAN see 'data-sectionid' in the HTML!")
		// Try to extract it
		re := regexp.MustCompile(`data-sectionid="(\d+)"`)
		if matches := re.FindStringSubmatch(htmlContent); len(matches) > 1 {
			fmt.Printf("  Section ID found: %s\n", matches[1])
		}
	} else {
		fmt.Println("✗ Our code CANNOT see 'data-sectionid' in the HTML")
	}
	fmt.Println()

	fmt.Println("Browser will stay open for 2 minutes for your inspection...")
	fmt.Println("Press Ctrl+C to exit earlier.")
	fmt.Println()

	time.Sleep(2 * time.Minute)

	fmt.Println("\nClosing browser...")
}

func containsDataSectionID(html string) bool {
	return strings.Contains(html, "data-sectionid") || strings.Contains(html, "data-sectionId")
}
