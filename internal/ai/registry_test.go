package ai

import (
	"testing"
)

func TestRegistryNoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool, len(ProviderRegistry))
	for _, p := range ProviderRegistry {
		if seen[p.ID] {
			t.Errorf("duplicate provider ID: %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestRegistryRequiredFields(t *testing.T) {
	validTiers := map[string]bool{"known": true, "gateway": true}

	for _, p := range ProviderRegistry {
		if p.ID == "" {
			t.Error("provider has empty ID")
		}
		if p.Name == "" {
			t.Errorf("provider %q has empty Name", p.ID)
		}
		if p.Tier == "" {
			t.Errorf("provider %q has empty Tier", p.ID)
		} else if !validTiers[p.Tier] {
			t.Errorf("provider %q has invalid Tier %q (want \"known\" or \"gateway\")", p.ID, p.Tier)
		}
		if p.Adapter == "" {
			t.Errorf("provider %q has empty Adapter", p.ID)
		}
	}
}

func TestRegistryModels(t *testing.T) {
	for _, p := range ProviderRegistry {
		for i, m := range p.Models {
			if m.ID == "" {
				t.Errorf("provider %q model[%d] has empty ID", p.ID, i)
			}
			if m.Name == "" {
				t.Errorf("provider %q model[%d] has empty Name", p.ID, i)
			}
		}
	}
}

func TestGetProviderDef(t *testing.T) {
	// Existing provider
	p, ok := GetProviderDef("openai")
	if !ok {
		t.Fatal("expected to find provider \"openai\"")
	}
	if p.Name != "OpenAI" {
		t.Errorf("expected Name \"OpenAI\", got %q", p.Name)
	}

	// Non-existent provider
	_, ok = GetProviderDef("nonexistent")
	if ok {
		t.Error("expected not to find provider \"nonexistent\"")
	}
}

func TestProvidersByCategory(t *testing.T) {
	frontier := ProvidersByCategory("frontier")
	if len(frontier) == 0 {
		t.Error("expected at least one frontier provider")
	}
	for _, p := range frontier {
		if p.Category != "frontier" {
			t.Errorf("ProvidersByCategory(\"frontier\") returned provider %q with category %q", p.ID, p.Category)
		}
	}

	empty := ProvidersByCategory("nonexistent_category")
	if len(empty) != 0 {
		t.Errorf("expected empty slice for nonexistent category, got %d", len(empty))
	}
}
