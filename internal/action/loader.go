package action

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/monoes/monoes-agent/data"
)

// ActionDef represents a complete action definition loaded from an embedded
// JSON file under data/actions/<platform>/<TYPE>.json.
type ActionDef struct {
	ActionType  string            `json:"actionType"`
	Platform    string            `json:"platform"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Inputs      *InputDef         `json:"inputs,omitempty"`
	Outputs     map[string][]string    `json:"outputs,omitempty"`
	Steps       []StepDef         `json:"steps"`
	Loops       []LoopDef         `json:"loops,omitempty"`
	ErrorConfig *GlobalErrorConfig `json:"errorHandling,omitempty"`
}

// InputDef lists the required and optional input variables for an action.
// Both fields accept either an array of strings (legacy) or an array of
// objects with {name, type, description, ...} (current format).
type InputDef struct {
	Required []json.RawMessage `json:"required,omitempty"`
	Optional []json.RawMessage `json:"optional,omitempty"`
}

// GlobalErrorConfig defines top-level error handling policy for the entire
// action.
type GlobalErrorConfig struct {
	GlobalRetries  int    `json:"globalRetries,omitempty"`
	RetryDelay     int    `json:"retryDelay,omitempty"`
	OnFinalFailure string `json:"onFinalFailure,omitempty"`
}

// ActionLoader loads and caches action definitions from the embedded JSON files
// in data.ActionsFS. It is safe for concurrent use.
type ActionLoader struct {
	cache sync.Map
}

var defaultLoader *ActionLoader
var loaderOnce sync.Once

// GetLoader returns the singleton ActionLoader instance.
func GetLoader() *ActionLoader {
	loaderOnce.Do(func() {
		defaultLoader = &ActionLoader{}
	})
	return defaultLoader
}

// Load returns the action definition for the given platform and actionType.
// Both platform and actionType are lower-cased to match the file naming
// convention: actions/<platform>/<action_type>.json
func (l *ActionLoader) Load(platform, actionType string) (*ActionDef, error) {
	normalPlatform := strings.ToLower(strings.TrimSpace(platform))
	normalType := strings.ToLower(strings.TrimSpace(actionType))
	key := fmt.Sprintf("%s/%s", normalPlatform, normalType)

	if cached, ok := l.cache.Load(key); ok {
		return cached.(*ActionDef), nil
	}

	path := fmt.Sprintf("actions/%s/%s.json", normalPlatform, normalType)
	fileData, err := data.ActionsFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("action definition not found: %s/%s: %w", normalPlatform, normalType, err)
	}

	var def ActionDef
	if err := json.Unmarshal(fileData, &def); err != nil {
		return nil, fmt.Errorf("failed to parse action definition %s/%s: %w", normalPlatform, normalType, err)
	}

	l.cache.Store(key, &def)
	return &def, nil
}

// ListAvailable returns all available action definitions as
// "<platform>/<ACTION_TYPE>" strings.
func (l *ActionLoader) ListAvailable() ([]string, error) {
	var result []string
	platforms := []string{"instagram", "linkedin", "x", "tiktok"}

	for _, p := range platforms {
		entries, err := data.ActionsFS.ReadDir(fmt.Sprintf("actions/%s", p))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				name := strings.TrimSuffix(e.Name(), ".json")
				result = append(result, fmt.Sprintf("%s/%s", p, name))
			}
		}
	}
	return result, nil
}

// Invalidate removes a cached action definition, forcing the next Load call
// for that key to re-read from the embedded filesystem.
func (l *ActionLoader) Invalidate(platform, actionType string) {
	key := fmt.Sprintf("%s/%s", strings.ToLower(platform), strings.ToLower(actionType))
	l.cache.Delete(key)
}

// InvalidateAll clears the entire cache.
func (l *ActionLoader) InvalidateAll() {
	l.cache.Range(func(key, _ interface{}) bool {
		l.cache.Delete(key)
		return true
	})
}
