package powerschool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/leftathome/powerschool-go/internal/browser"
)

// GetProgressReports retrieves the list of progress reports and report cards for a student
// These are PDFs organized by academic year
// URL pattern: https://psplugin.example.org/ProgressReports/Reports/Index?studentidentifier=[STUDENT_ID]
func (c *Client) GetProgressReports(ctx context.Context, studentID string) ([]*ProgressReport, error) {
	c.logger.Info("Fetching progress reports for student: %s", studentID)

	// Ensure we're authenticated
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	// Note: This is a different domain (psplugin.example.org) but should use same session
	url := fmt.Sprintf("https://psplugin.example.org/ProgressReports/Reports/Index?studentidentifier=%s", studentID)

	// Get the page HTML
	html, err := c.getPageHTMLFromURL(ctx, url)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Parsing progress reports from HTML")

	reports, err := parseProgressReports(html, studentID)
	if err != nil {
		return nil, WrapParseError(err, "progress reports")
	}

	c.logger.Info("Found %d progress report(s)", len(reports))
	return reports, nil
}

// getPageHTMLFromURL retrieves HTML from a full URL (not just path)
// This is needed for the progress reports which are on a different subdomain
func (c *Client) getPageHTMLFromURL(ctx context.Context, url string) (string, error) {
	if !c.IsAuthenticated() {
		return "", ErrSessionExpired
	}

	c.logger.Debug("Fetching page: %s", url)

	// Use browser automation to get the page content with session cookies
	// The session cookies should work across subdomains
	html, err := browser.GetPageContent(ctx, url, c.session.GetCookies(), 30*time.Second)
	if err != nil {
		return "", WrapNetworkError(err)
	}

	c.logger.DebugHTML("page content", html)

	return html, nil
}

// parseProgressReports parses progress report information from the HTML
// The page is organized by academic year with links to PDF files
func parseProgressReports(html string, studentID string) ([]*ProgressReport, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var reports []*ProgressReport

	// TODO: Parse the actual HTML structure
	// The page is organized by year with PDF links
	// Need to capture:
	// - Year groupings
	// - Report title (Q1 Progress Report, Q2 Report Card, etc.)
	// - PDF URL
	// - Date posted (if available)

	// Placeholder implementation - will need actual HTML to complete
	doc.Find("a[href*='.pdf']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		title := strings.TrimSpace(s.Text())
		if title == "" {
			return
		}

		report := &ProgressReport{
			ID:        fmt.Sprintf("%s-%d", studentID, i),
			StudentID: studentID,
			Title:     title,
			URL:       href,
		}

		// Try to determine type from title
		titleLower := strings.ToLower(title)
		if strings.Contains(titleLower, "progress report") {
			report.Type = "Progress Report"
		} else if strings.Contains(titleLower, "report card") {
			report.Type = "Report Card"
		}

		reports = append(reports, report)
	})

	return reports, nil
}

// DownloadProgressReport downloads a progress report PDF
// Returns the PDF content as bytes
func (c *Client) DownloadProgressReport(ctx context.Context, report *ProgressReport) ([]byte, error) {
	c.logger.Info("Downloading progress report: %s", report.Title)

	// TODO: Implement PDF download using session cookies
	// This would use HTTP client with cookies to download the PDF

	return nil, fmt.Errorf("DownloadProgressReport not yet implemented")
}
