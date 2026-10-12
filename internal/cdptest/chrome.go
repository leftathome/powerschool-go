// Package cdptest starts a local headless Chrome with its DevTools endpoint
// open, for tests that exercise the remote-browser sign-in against a real
// browser. It is test support only.
package cdptest

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ChromePath returns a Chrome/Chromium binary found on PATH, or "".
func ChromePath() string {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"headless-shell", "chrome",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// StartChrome starts a headless Chrome listening for DevTools connections on
// a free local port and returns its browser WebSocket URL and a function
// that kills it. The test is skipped when no browser is installed, in -short
// mode, or when the browser does not come up. The browser is killed when the
// test ends.
func StartChrome(t testing.TB) (wsURL string, kill func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	path := ChromePath()
	if path == "" {
		t.Skip("no Chrome/Chromium binary on PATH")
	}

	args := []string{
		"--headless=new",
		"--remote-debugging-port=0",
		"--remote-debugging-address=127.0.0.1",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--user-data-dir=" + t.TempDir(),
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	args = append(args, "about:blank")

	cmd := exec.Command(path, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start the browser: %v", err)
	}
	waited := make(chan struct{})
	kill = func() {
		_ = cmd.Process.Kill()
		<-waited
	}
	t.Cleanup(kill)

	found := make(chan string, 1)
	go func() {
		defer close(waited)
		const marker = "DevTools listening on "
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if i := strings.Index(sc.Text(), marker); i >= 0 {
				select {
				case found <- strings.TrimSpace(sc.Text()[i+len(marker):]):
				default:
				}
			}
		}
		_ = cmd.Wait()
	}()

	select {
	case wsURL = <-found:
		return wsURL, kill
	case <-waited:
		t.Skip("the browser exited before opening its DevTools endpoint")
	case <-time.After(45 * time.Second):
		t.Skip("the browser did not open its DevTools endpoint in time")
	}
	return "", kill
}
