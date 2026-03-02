package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
)

// BotAdapter defines the contract that every platform-specific bot must
// implement. It abstracts login detection, URL handling, messaging, and
// profile extraction so the orchestration layer remains platform-agnostic.
type BotAdapter interface {
	// Platform returns the canonical uppercase name of the platform
	// (e.g. "INSTAGRAM", "LINKEDIN", "X").
	Platform() string

	// LoginURL returns the URL to navigate to for user authentication.
	LoginURL() string

	// IsLoggedIn inspects the current page state to determine whether the
	// user is authenticated.
	IsLoggedIn(page *rod.Page) (bool, error)

	// ResolveURL normalizes a raw URL. If rawURL is a relative path it is
	// converted to an absolute URL for the platform.
	ResolveURL(rawURL string) string

	// ExtractUsername extracts the platform username from a profile page URL.
	ExtractUsername(pageURL string) string

	// SearchURL builds the platform search URL for the given keyword.
	SearchURL(keyword string) string

	// SendMessage sends a direct message to the specified user.
	SendMessage(ctx context.Context, page *rod.Page, username, message string) error

	// GetProfileData scrapes the currently loaded profile page and returns
	// structured data as a map.
	GetProfileData(ctx context.Context, page *rod.Page) (map[string]interface{}, error)
}

// PlatformRegistry maps platform names (uppercase) to factory functions that
// create the corresponding BotAdapter. Platform packages register themselves
// via init() functions.
var PlatformRegistry = map[string]func() BotAdapter{}

// NewBot creates a new BotAdapter for the given platform.
// The platform name is normalised to upper-case before lookup.
func NewBot(platform string) (BotAdapter, error) {
	platform = strings.ToUpper(strings.TrimSpace(platform))
	constructor, ok := PlatformRegistry[platform]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
	return constructor(), nil
}
