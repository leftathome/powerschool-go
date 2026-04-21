package powerschool

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		wantErr     bool
		expectedURL string
	}{
		{
			name:        "valid HTTPS URL",
			baseURL:     "https://powerschool.example.com",
			wantErr:     false,
			expectedURL: "https://powerschool.example.com",
		},
		{
			name:        "HTTP URL upgraded to HTTPS",
			baseURL:     "http://powerschool.example.com",
			wantErr:     false,
			expectedURL: "https://powerschool.example.com",
		},
		{
			name:        "URL with trailing slash",
			baseURL:     "https://powerschool.example.com/",
			wantErr:     false,
			expectedURL: "https://powerschool.example.com",
		},
		{
			name:    "empty URL",
			baseURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client.baseURL != tt.expectedURL {
				t.Errorf("NewClient() baseURL = %v, want %v", client.baseURL, tt.expectedURL)
			}
		})
	}
}

func TestWithCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{
			name:     "valid credentials",
			username: "testuser",
			password: "testpass",
			wantErr:  false,
		},
		{
			name:     "empty username",
			username: "",
			password: "testpass",
			wantErr:  true,
		},
		{
			name:     "empty password",
			username: "testuser",
			password: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(
				"https://powerschool.example.com",
				WithCredentials(tt.username, tt.password),
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("WithCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if client.credentials == nil {
					t.Error("WithCredentials() credentials not set")
				}
				if client.credentials.Username != tt.username {
					t.Errorf("WithCredentials() username = %v, want %v", client.credentials.Username, tt.username)
				}
			}
		})
	}
}

func TestWithTimeout(t *testing.T) {
	timeout := 60 * time.Second
	client, err := NewClient(
		"https://powerschool.example.com",
		WithTimeout(timeout),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.httpClient.Timeout != timeout {
		t.Errorf("WithTimeout() timeout = %v, want %v", client.httpClient.Timeout, timeout)
	}
}

func TestClientMethods(t *testing.T) {
	client, err := NewClient(
		"https://powerschool.example.com",
		WithCredentials("testuser", "testpass"),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	t.Run("GetBaseURL", func(t *testing.T) {
		expected := "https://powerschool.example.com"
		if client.GetBaseURL() != expected {
			t.Errorf("GetBaseURL() = %v, want %v", client.GetBaseURL(), expected)
		}
	})

	t.Run("HasCredentials", func(t *testing.T) {
		if !client.HasCredentials() {
			t.Error("HasCredentials() = false, want true")
		}
	})

	t.Run("IsAuthenticated before auth", func(t *testing.T) {
		if client.IsAuthenticated() {
			t.Error("IsAuthenticated() = true, want false before authentication")
		}
	})

	t.Run("GetSession before auth", func(t *testing.T) {
		if client.GetSession() != nil {
			t.Error("GetSession() != nil, want nil before authentication")
		}
	})
}

func TestBuildURL(t *testing.T) {
	client, _ := NewClient("https://powerschool.example.com")

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "path with leading slash",
			path:     "/guardian/home.html",
			expected: "https://powerschool.example.com/guardian/home.html",
		},
		{
			name:     "path without leading slash",
			path:     "guardian/home.html",
			expected: "https://powerschool.example.com/guardian/home.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.buildURL(tt.path)
			if result != tt.expected {
				t.Errorf("buildURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}
