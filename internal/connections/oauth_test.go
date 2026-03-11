package connections

import (
	"strings"
	"testing"
)

// TestBuildAuthURL verifies that buildAuthURL produces a well-formed URL
// containing the expected query parameters without making network calls.
func TestBuildAuthURL(t *testing.T) {
	cfg := OAuthConfig{
		AuthURL:  "https://example.com/oauth/authorize",
		TokenURL: "https://example.com/oauth/token",
		ClientID: "test-client",
		Scopes:   []string{"read", "write"},
	}

	got, err := buildAuthURL(cfg, "http://localhost:9876/callback", "teststate")
	if err != nil {
		t.Fatalf("buildAuthURL returned error: %v", err)
	}

	if !strings.Contains(got, "client_id=test-client") {
		t.Errorf("URL missing client_id=test-client; got: %s", got)
	}
	if !strings.Contains(got, "state=teststate") {
		t.Errorf("URL missing state=teststate; got: %s", got)
	}
	if !strings.Contains(got, "scope=") {
		t.Errorf("URL missing scope param; got: %s", got)
	}
	if !strings.Contains(got, "response_type=code") {
		t.Errorf("missing response_type=code: %s", got)
	}
}

// TestRandomStateUnique verifies that randomState returns distinct values
// and that each is at least 16 characters long.
func TestRandomStateUnique(t *testing.T) {
	s1, err := randomState()
	if err != nil {
		t.Fatalf("randomState (1st call) error: %v", err)
	}
	s2, err := randomState()
	if err != nil {
		t.Fatalf("randomState (2nd call) error: %v", err)
	}

	if s1 == s2 {
		t.Errorf("randomState returned the same value twice: %q", s1)
	}
	if len(s1) < 16 {
		t.Errorf("randomState s1 too short: len=%d, value=%q", len(s1), s1)
	}
	if len(s2) < 16 {
		t.Errorf("randomState s2 too short: len=%d, value=%q", len(s2), s2)
	}
}
