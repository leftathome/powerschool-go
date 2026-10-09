package powerschool

import (
	"net/http"
	"net/url"
	"testing"
)

// A dead session is a redirect to the signed-out area (or a 401/403). Any
// other 3xx must not be read as one: doing so discards a working session.
func TestIsLoginRedirect(t *testing.T) {
	resp := func(status int, location, finalPath string) *http.Response {
		r := &http.Response{StatusCode: status, Header: http.Header{}}
		if location != "" {
			r.Header.Set("Location", location)
		}
		if finalPath != "" {
			r.Request = &http.Request{URL: &url.URL{Path: finalPath}}
		}
		return r
	}
	for _, tc := range []struct {
		name string
		r    *http.Response
		want bool
	}{
		{"302 to public login", resp(302, "/public/home.html", ""), true},
		{"302 absolute to public", resp(302, "https://ps.example.org/public/", ""), true},
		{"followed redirect landed on public", resp(200, "", "/public/home.html"), true},
		{"401", resp(401, "", ""), true},
		{"403", resp(403, "", ""), true},
		{"304 not modified", resp(304, "", ""), false},
		{"302 without location", resp(302, "", ""), false},
		{"302 to another guardian page", resp(302, "/guardian/home.html", ""), false},
		{"307 elsewhere", resp(307, "/guardian/termgrades.html", ""), false},
		{"200 guardian page", resp(200, "", "/guardian/home.html"), false},
		{"200 with no request", resp(200, "", ""), false},
	} {
		if got := isLoginRedirect(tc.r); got != tc.want {
			t.Errorf("%s: isLoginRedirect = %v, want %v", tc.name, got, tc.want)
		}
	}
}
