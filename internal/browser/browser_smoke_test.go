package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// chromeAvailable reports whether a Chrome/Chromium binary chromedp can launch
// is on PATH.
func chromeAvailable() bool {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"headless-shell", "chrome",
	} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// TestGetPageContent_RealBrowser drives a real headless Chrome against a local
// page. Nothing else in this module exercises the DevTools-protocol encoding
// at runtime, and that encoding is exactly what a chromedp / cdproto /
// go-json-experiment bump can break while still compiling. It covers the three
// things the library relies on: setting cookies, navigating, and reading back
// a DOM that JavaScript has modified.
func TestGetPageContent_RealBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser smoke test in -short mode")
	}
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary on PATH")
	}

	var sawCookie atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("smoke"); err == nil && c.Value == "cookie-value-123" {
			sawCookie.Store(true)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>before</title></head>
<body><div id="static">static-marker</div><div id="dyn"></div>
<script>document.getElementById('dyn').textContent = 'rendered-' + (6*7);</script>
</body></html>`))
	}))
	defer srv.Close()

	cookies := []*http.Cookie{{
		Name:   "smoke",
		Value:  "cookie-value-123",
		Domain: "127.0.0.1",
		Path:   "/",
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	html, err := GetPageContentWithWait(ctx, srv.URL, cookies, 45*time.Second, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("GetPageContentWithWait: %v", err)
	}
	if !strings.Contains(html, "static-marker") {
		t.Errorf("page content is missing the static marker; got %d bytes", len(html))
	}
	if !strings.Contains(html, "rendered-42") {
		t.Errorf("page content is missing the JavaScript-rendered marker; the DOM was not read after script execution")
	}
	if !sawCookie.Load() {
		t.Errorf("the server never received the cookie set through the DevTools protocol")
	}
}
