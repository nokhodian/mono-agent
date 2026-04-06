package auth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/nokhodian/mono-agent/internal/browser"
	"github.com/rs/zerolog"
)

// SessionStore interface for database operations
type SessionStore interface {
	SaveSession(platform, username, cookiesJSON string, expiry time.Time) error
	GetSession(platform, username string) (*Session, error)
	ListSessions() ([]*Session, error)
	DeleteSession(platform, username string) error
}

// Session represents a stored browser session
type Session struct {
	ID           int
	Username     string
	Platform     string
	CookiesJSON  string
	Expiry       time.Time
	WhenAdded    time.Time
	ProfilePhoto []byte
}

// AuthManager handles authentication for social platform sessions.
type AuthManager struct {
	store  SessionStore
	logger zerolog.Logger
}

func NewAuthManager(store SessionStore, logger zerolog.Logger) *AuthManager {
	return &AuthManager{store: store, logger: logger}
}

// SaveCookies extracts cookies from the browser page and saves them.
func (am *AuthManager) SaveCookies(page browser.PageInterface, platform, username string) error {
	rawCookies, err := page.GetCookies()
	if err != nil {
		return fmt.Errorf("failed to get cookies: %w", err)
	}

	cookieJSON, err := json.Marshal(rawCookies)
	if err != nil {
		return fmt.Errorf("cookie serialization failed: %w", err)
	}

	// Find the latest expiry among all cookies.
	// The underlying type from RodPage is []*proto.NetworkCookie.
	var maxExpiry time.Time
	if cookies, ok := rawCookies.([]*proto.NetworkCookie); ok {
		for _, c := range cookies {
			if c.Expires > 0 {
				exp := c.Expires.Time()
				if exp.After(maxExpiry) {
					maxExpiry = exp
				}
			}
		}
	}
	if maxExpiry.IsZero() {
		maxExpiry = time.Now().Add(365 * 24 * time.Hour)
	}

	return am.store.SaveSession(platform, username, string(cookieJSON), maxExpiry)
}

// RestoreCookies loads cookies from storage and injects them into the browser.
func (am *AuthManager) RestoreCookies(page browser.PageInterface, platform, username string) error {
	session, err := am.store.GetSession(platform, username)
	if err != nil {
		return fmt.Errorf("no session found for %s/%s: %w", platform, username, err)
	}

	var cookies []*proto.NetworkCookieParam
	if err := json.Unmarshal([]byte(session.CookiesJSON), &cookies); err != nil {
		return fmt.Errorf("cookie deserialization failed: %w", err)
	}

	return page.SetCookies(cookies)
}

// HasValidSession checks if there's a non-expired session for the given platform/username.
func (am *AuthManager) HasValidSession(platform, username string) bool {
	session, err := am.store.GetSession(platform, username)
	if err != nil {
		return false
	}
	return session.Expiry.After(time.Now())
}

// ListActiveSessions returns all non-expired sessions.
func (am *AuthManager) ListActiveSessions() ([]*Session, error) {
	sessions, err := am.store.ListSessions()
	if err != nil {
		return nil, err
	}
	var active []*Session
	now := time.Now()
	for _, s := range sessions {
		if s.Expiry.After(now) {
			active = append(active, s)
		}
	}
	return active, nil
}
