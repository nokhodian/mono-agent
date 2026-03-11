package connections

import (
	"testing"
)

// allExpectedIDs lists all 27 platform IDs that must exist in the registry.
var allExpectedIDs = []string{
	// social
	"instagram", "linkedin", "x", "tiktok", "telegram",
	// service
	"github", "notion", "airtable", "jira", "linear", "asana",
	"stripe", "shopify", "salesforce", "hubspot",
	"google_sheets", "gmail", "google_drive",
	// communication
	"slack", "discord", "twilio", "whatsapp", "smtp",
	// database
	"postgresql", "mysql", "mongodb", "redis",
}

// TestRegistryHasAllExpectedPlatforms verifies that every expected platform ID
// is present in the Registry.
func TestRegistryHasAllExpectedPlatforms(t *testing.T) {
	if len(allExpectedIDs) != 27 {
		t.Fatalf("test setup error: expected 27 IDs, got %d", len(allExpectedIDs))
	}
	for _, id := range allExpectedIDs {
		if _, ok := Registry[id]; !ok {
			t.Errorf("Registry missing platform %q", id)
		}
	}
	if got := len(Registry); got != 27 {
		t.Errorf("Registry has %d platforms, want 27", got)
	}
}

// TestPlatformMethodsNonEmpty verifies that every platform declares at least
// one authentication method.
func TestPlatformMethodsNonEmpty(t *testing.T) {
	for id, p := range Registry {
		if len(p.Methods) == 0 {
			t.Errorf("platform %q has no methods", id)
		}
	}
}

// TestAPIKeyPlatformsHaveFields verifies that any platform that lists
// MethodAPIKey, MethodConnStr, or MethodAppPass as a method also has a
// non-empty Fields entry for that method.
func TestAPIKeyPlatformsHaveFields(t *testing.T) {
	credMethods := []AuthMethod{MethodAPIKey, MethodConnStr, MethodAppPass}
	for id, p := range Registry {
		for _, m := range p.Methods {
			for _, cm := range credMethods {
				if m != cm {
					continue
				}
				fields, ok := p.Fields[m]
				if !ok || len(fields) == 0 {
					t.Errorf("platform %q uses method %q but Fields[%q] is empty", id, m, m)
				}
			}
		}
	}
}

// TestByConnectVia verifies that ByConnectVia("UI") and ByConnectVia("API")
// together account for every entry in the Registry.
func TestByConnectVia(t *testing.T) {
	ui := ByConnectVia("UI")
	api := ByConnectVia("API")
	total := len(ui) + len(api)
	if total != len(Registry) {
		t.Errorf("ByConnectVia(\"UI\")=%d + ByConnectVia(\"API\")=%d = %d, want %d",
			len(ui), len(api), total, len(Registry))
	}
}
