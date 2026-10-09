package powerschool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/leftathome/powerschool-go/internal/browser"
)

// GetProgressReports is NOT IMPLEMENTED and always returns
// ErrNotImplemented without contacting anything.
//
// Progress reports and report cards are served by a district plugin on a
// separate host whose address and page structure have never been captured.
// The earlier version of this method fetched a placeholder host and
// returned whatever PDF links it found, which for a real account is an
// empty list indistinguishable from "no reports".
func (c *Client) GetProgressReports(ctx context.Context, studentID string) ([]*ProgressReport, error) {
	return nil, ErrNotImplemented
}

// getPageHTMLFromURL retrieves HTML from a full URL (not just path)
// This is needed for the progress reports which are on a different subdomain
func (c *Client) getPageHTMLFromURL(ctx context.Context, url string) (string, error) {
	if !c.IsAuthenticated() {
		return "", ErrSessionExpired
	}

	if c.httpPages {
		return "", errBrowserDisabled("GetProgressReports")
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

// DownloadProgressReport is NOT IMPLEMENTED and always returns
// ErrNotImplemented. See GetProgressReports.
func (c *Client) DownloadProgressReport(ctx context.Context, report *ProgressReport) ([]byte, error) {
	return nil, ErrNotImplemented
}
