package nodes

import "testing"

func TestNormalizeBrowserItem(t *testing.T) {
	raw := map[string]interface{}{
		"text": "Alice Wonderland\nCTO at StartupCo\nBerlin",
		"href": "https://www.linkedin.com/in/alice-wonderland/",
	}
	out := normalizeBrowserItem(raw, "linkedin")

	cases := []struct {
		key  string
		want string
	}{
		{"profile_url", "https://www.linkedin.com/in/alice-wonderland/"},
		{"url", "https://www.linkedin.com/in/alice-wonderland/"},
		{"full_name", "Alice Wonderland"},
		{"job_title", "CTO at StartupCo"},
		{"platform", "linkedin"},
	}
	for _, c := range cases {
		got, _ := out[c.key].(string)
		if got != c.want {
			t.Errorf("normalizeBrowserItem[%q]: want %q, got %q", c.key, c.want, got)
		}
	}
}

func TestNormalizeBrowserItem_PreservesExistingFields(t *testing.T) {
	raw := map[string]interface{}{
		"profile_url": "https://existing.url/",
		"full_name":   "Already Set",
		"platform":    "instagram",
		"href":        "https://other.url/",
	}
	out := normalizeBrowserItem(raw, "linkedin")

	// Should NOT overwrite existing values.
	if out["profile_url"] != "https://existing.url/" {
		t.Errorf("profile_url overwritten: %v", out["profile_url"])
	}
	if out["full_name"] != "Already Set" {
		t.Errorf("full_name overwritten: %v", out["full_name"])
	}
	if out["platform"] != "instagram" {
		t.Errorf("platform overwritten: %v", out["platform"])
	}
}
