package powerschool

import "testing"

// A bare host must become an https URL: before this was normalised the
// client built "host/public/" and the browser rejected it as invalid.
func TestNewClient_BaseURLNormalisation(t *testing.T) {
	for in, want := range map[string]string{
		"ps.example.org":             "https://ps.example.org",
		"ps.example.org/":            "https://ps.example.org",
		"  ps.example.org \n":        "https://ps.example.org",
		"https://ps.example.org":     "https://ps.example.org",
		"https://ps.example.org/":    "https://ps.example.org",
		"http://ps.example.org":      "https://ps.example.org",
		"ps.example.org:8443":        "https://ps.example.org:8443",
		"https://ps.example.org/sub": "https://ps.example.org/sub",
	} {
		c, err := NewClient(in)
		if err != nil {
			t.Errorf("NewClient(%q): unexpected error %v", in, err)
			continue
		}
		if got := c.GetBaseURL(); got != want {
			t.Errorf("NewClient(%q).GetBaseURL() = %q, want %q", in, got, want)
		}
	}

	for _, in := range []string{"", "   ", "https://", "/just/a/path"} {
		if _, err := NewClient(in); err == nil {
			t.Errorf("NewClient(%q): expected an error for a value with no host", in)
		}
	}
}
