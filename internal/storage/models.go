package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NewID generates a new UUID string for use as a primary key.
func NewID() string {
	return uuid.New().String()
}

// Session represents a row in the crawler_sessions table.
type Session struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Platform     string    `json:"platform"`
	CookiesJSON  string    `json:"cookies_json"`
	Expiry       time.Time `json:"expiry"`
	WhenAdded    time.Time `json:"when_added"`
	ProfilePhoto []byte    `json:"profile_photo,omitempty"`
}

// Action represents a row in the actions table.
type Action struct {
	ID                    string `json:"id"`
	CreatedAt             int64  `json:"created_at"`
	Title                 string `json:"title"`
	Type                  string `json:"type"`
	State                 string `json:"state"`
	Disabled              bool   `json:"disabled"`
	TargetPlatform        string `json:"target_platform"`
	Position              int    `json:"position"`
	ContentSubject        string `json:"content_subject,omitempty"`
	ContentMessage        string `json:"content_message,omitempty"`
	ContentBlobURLs       string `json:"content_blob_urls,omitempty"`
	ScheduledDate         string `json:"scheduled_date,omitempty"`
	ExecutionInterval     int    `json:"execution_interval,omitempty"`
	StartDate             string `json:"start_date,omitempty"`
	EndDate               string `json:"end_date,omitempty"`
	CampaignID            string `json:"campaign_id,omitempty"`
	ReachedIndex          int    `json:"reached_index"`
	Keywords              string `json:"keywords,omitempty"`
	ActionExecutionCount  int    `json:"action_execution_count"`
	CreatedAtTS           time.Time              `json:"created_at_ts"`
	UpdatedAtTS           time.Time              `json:"updated_at_ts"`
	Params                map[string]interface{} `json:"params,omitempty"`
}

// ActionTarget represents a row in the action_targets table.
type ActionTarget struct {
	ID               string    `json:"id"`
	ActionID         string    `json:"action_id"`
	PersonID         string    `json:"person_id,omitempty"`
	Platform         string    `json:"platform"`
	Link             string    `json:"link,omitempty"`
	SourceType       string    `json:"source_type,omitempty"`
	Status           string    `json:"status"`
	LastInteractedAt string    `json:"last_interacted_at,omitempty"`
	CommentText      string    `json:"comment_text,omitempty"`
	Metadata         string    `json:"metadata,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Person represents a row in the people table.
type Person struct {
	ID               string    `json:"id"`
	PlatformUsername string    `json:"platform_username"`
	Platform         string    `json:"platform"`
	FullName         string    `json:"full_name,omitempty"`
	ImageURL         string    `json:"image_url,omitempty"`
	ContactDetails   string    `json:"contact_details,omitempty"`
	Website          string    `json:"website,omitempty"`
	ContentCount     int       `json:"content_count"`
	FollowerCount    string    `json:"follower_count,omitempty"`
	FollowingCount   int       `json:"following_count"`
	Introduction     string    `json:"introduction,omitempty"`
	IsVerified       bool      `json:"is_verified"`
	Category         string    `json:"category,omitempty"`
	JobTitle         string    `json:"job_title,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SocialList represents a row in the social_lists table.
type SocialList struct {
	ID        string    `json:"id"`
	ListType  string    `json:"list_type,omitempty"`
	Name      string    `json:"name"`
	ItemCount int       `json:"item_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SocialListItem represents a row in the social_list_items table.
type SocialListItem struct {
	ID               string    `json:"id"`
	ListID           string    `json:"list_id"`
	Platform         string    `json:"platform"`
	PlatformUsername string    `json:"platform_username"`
	ImageURL         string    `json:"image_url,omitempty"`
	URL              string    `json:"url,omitempty"`
	FullName         string    `json:"full_name,omitempty"`
	ContactDetails   string    `json:"contact_details,omitempty"`
	FollowerCount    int       `json:"follower_count"`
	CreatedAt        time.Time `json:"created_at"`
}

// Thread represents a row in the threads table.
type Thread struct {
	ID           string    `json:"id"`
	SocialUserID string    `json:"social_user_id"`
	Platform     string    `json:"platform"`
	Metadata     string    `json:"metadata,omitempty"`
	Messages     string    `json:"messages,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Template represents a row in the templates table.
type Template struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject,omitempty"`
	Body      string    `json:"body"`
	Metadata  string    `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigEntry represents a row in the configs table.
type ConfigEntry struct {
	Name       string    `json:"name"`
	ConfigData string    `json:"config_data"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// normalizeState ensures the action state string is one of the known valid
// states. If the incoming value is not recognized it defaults to "PENDING".
func normalizeState(s string) string {
	upper := strings.ToUpper(strings.TrimSpace(s))
	switch upper {
	case "PENDING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELLED":
		return upper
	default:
		return "PENDING"
	}
}

// getString safely extracts a string value from a map. Returns "" when the
// key is missing or the value is not a string.
func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// getInt safely extracts an int value from a map.
func getInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

// getBool safely extracts a bool value from a map.
func getBool(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	default:
		return false
	}
}

// ParseAction handles both legacy nested JSON format and the flat format used
// by the current schema. In the legacy format the action's content fields are
// nested under a "content" key, and scheduling fields under "schedule". The
// flat format stores all fields at the top level.
//
// Example legacy format:
//
//	{
//	  "id": "abc",
//	  "title": "My Action",
//	  "type": "BULK_MESSAGING",
//	  "target_platform": "instagram",
//	  "content": { "subject": "Hi", "message": "Hello there", "blob_urls": "..." },
//	  "schedule": { "scheduled_date": "2025-01-01", "execution_interval": 60 }
//	}
//
// Example flat format:
//
//	{
//	  "id": "abc",
//	  "title": "My Action",
//	  "type": "BULK_MESSAGING",
//	  "target_platform": "instagram",
//	  "content_subject": "Hi",
//	  "content_message": "Hello there",
//	  "scheduled_date": "2025-01-01"
//	}
func ParseAction(data []byte) (*Action, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action JSON: %w", err)
	}

	action := &Action{
		ID:             getString(raw, "id"),
		Title:          getString(raw, "title"),
		Type:           getString(raw, "type"),
		TargetPlatform: getString(raw, "target_platform"),
		State:          normalizeState(getString(raw, "state")),
		Disabled:       getBool(raw, "disabled"),
		Position:       getInt(raw, "position"),
		ReachedIndex:   getInt(raw, "reached_index"),
		Keywords:       getString(raw, "keywords"),
		CampaignID:     getString(raw, "campaign_id"),
		ActionExecutionCount: getInt(raw, "action_execution_count"),
	}

	// created_at may be an integer (unix epoch) or a string.
	if ca, ok := raw["created_at"]; ok && ca != nil {
		switch v := ca.(type) {
		case float64:
			action.CreatedAt = int64(v)
		case json.Number:
			n, _ := v.Int64()
			action.CreatedAt = n
		}
	}
	if action.CreatedAt == 0 {
		action.CreatedAt = time.Now().Unix()
	}

	if action.ID == "" {
		action.ID = NewID()
	}

	// Check for legacy nested "content" object.
	if contentRaw, ok := raw["content"]; ok && contentRaw != nil {
		if content, ok := contentRaw.(map[string]interface{}); ok {
			action.ContentSubject = getString(content, "subject")
			action.ContentMessage = getString(content, "message")
			action.ContentBlobURLs = getString(content, "blob_urls")
		}
	}

	// Flat format fields override nested ones if present.
	if v := getString(raw, "content_subject"); v != "" {
		action.ContentSubject = v
	}
	if v := getString(raw, "content_message"); v != "" {
		action.ContentMessage = v
	}
	if v := getString(raw, "content_blob_urls"); v != "" {
		action.ContentBlobURLs = v
	}

	// Check for legacy nested "schedule" object.
	if schedRaw, ok := raw["schedule"]; ok && schedRaw != nil {
		if sched, ok := schedRaw.(map[string]interface{}); ok {
			action.ScheduledDate = getString(sched, "scheduled_date")
			action.ExecutionInterval = getInt(sched, "execution_interval")
			action.StartDate = getString(sched, "start_date")
			action.EndDate = getString(sched, "end_date")
		}
	}

	// Flat format scheduling fields override nested ones if present.
	if v := getString(raw, "scheduled_date"); v != "" {
		action.ScheduledDate = v
	}
	if v := getInt(raw, "execution_interval"); v != 0 {
		action.ExecutionInterval = v
	}
	if v := getString(raw, "start_date"); v != "" {
		action.StartDate = v
	}
	if v := getString(raw, "end_date"); v != "" {
		action.EndDate = v
	}

	// Params — generic per-action parameter map.
	if paramsRaw, ok := raw["params"]; ok && paramsRaw != nil {
		if params, ok := paramsRaw.(map[string]interface{}); ok {
			action.Params = params
		}
	}
	if action.Params == nil {
		action.Params = make(map[string]interface{})
	}

	return action, nil
}
