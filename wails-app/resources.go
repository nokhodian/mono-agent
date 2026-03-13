package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ResourceItem is a single listable resource (spreadsheet, channel, etc.)
type ResourceItem struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceListResult is returned by ListResources.
type ResourceListResult struct {
	Items      []ResourceItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// ResourceItemResult is returned by CreateResource.
type ResourceItemResult struct {
	Item  *ResourceItem `json:"item,omitempty"`
	Error string        `json:"error,omitempty"`
}

// ListResources lists external resources for a given platform and resource type.
// credentialID is the connection ID. query is an optional search string.
func (a *App) ListResources(platform, resourceType, credentialID, query string) ResourceListResult {
	ctx := context.Background()
	creds, err := a.getResourceCredentialData(ctx, credentialID)
	if err != nil {
		return ResourceListResult{Error: fmt.Sprintf("credential lookup: %v", err)}
	}
	switch platform {
	case "google_sheets", "google_drive":
		return listGoogleDriveResources(creds, resourceType, query)
	case "gmail":
		return listGmailResources(creds, resourceType, query)
	case "slack":
		return listSlackResources(creds, resourceType, query)
	default:
		return ResourceListResult{Error: fmt.Sprintf("platform %q not supported for resource listing", platform)}
	}
}

// CreateResource creates a new external resource and returns the created item.
func (a *App) CreateResource(platform, resourceType, credentialID, name string) ResourceItemResult {
	ctx := context.Background()
	creds, err := a.getResourceCredentialData(ctx, credentialID)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("credential lookup: %v", err)}
	}
	switch platform {
	case "google_sheets":
		return createGoogleSheet(creds, name)
	case "google_drive":
		return createGoogleDriveFolder(creds, name)
	default:
		return ResourceItemResult{Error: fmt.Sprintf("create not supported for platform %q", platform)}
	}
}

// getResourceCredentialData fetches credential data from the connections manager.
func (a *App) getResourceCredentialData(ctx context.Context, credentialID string) (map[string]interface{}, error) {
	if a.connMgr == nil {
		return nil, fmt.Errorf("connections manager not available")
	}
	conn, err := a.connMgr.Get(ctx, credentialID)
	if err != nil || conn == nil {
		return nil, fmt.Errorf("credential %s not found", credentialID)
	}
	return conn.Data, nil
}

// listGoogleDriveResources lists Google Drive/Sheets resources.
func listGoogleDriveResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceListResult{Error: "google: access_token not found in credential"}
	}
	var apiURL string
	switch resourceType {
	case "spreadsheets":
		q := "mimeType='application/vnd.google-apps.spreadsheet' and trashed=false"
		if query != "" {
			q += " and name contains '" + query + "'"
		}
		apiURL = "https://www.googleapis.com/drive/v3/files?q=" + url.QueryEscape(q) + "&fields=files(id,name,modifiedTime)&pageSize=50"
	case "folders":
		q := "mimeType='application/vnd.google-apps.folder' and trashed=false"
		if query != "" {
			q += " and name contains '" + query + "'"
		}
		apiURL = "https://www.googleapis.com/drive/v3/files?q=" + url.QueryEscape(q) + "&fields=files(id,name,modifiedTime)&pageSize=50"
	default:
		return ResourceListResult{Error: fmt.Sprintf("google_drive: unsupported resource type %q", resourceType)}
	}
	body, err := googleAPIGet(apiURL, accessToken)
	if err != nil {
		return ResourceListResult{Error: err.Error()}
	}
	var resp struct {
		Files []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ModifiedTime string `json:"modifiedTime"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("google: parse response: %v", err)}
	}
	items := make([]ResourceItem, 0, len(resp.Files))
	for _, f := range resp.Files {
		items = append(items, ResourceItem{
			ID:   f.ID,
			Name: f.Name,
			Metadata: map[string]interface{}{
				"modified_time": f.ModifiedTime,
			},
		})
	}
	return ResourceListResult{Items: items}
}

// listGmailResources lists Gmail labels.
func listGmailResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceListResult{Error: "gmail: access_token not found in credential"}
	}
	if resourceType != "labels" {
		return ResourceListResult{Error: fmt.Sprintf("gmail: unsupported resource type %q", resourceType)}
	}
	body, err := googleAPIGet("https://gmail.googleapis.com/gmail/v1/users/me/labels", accessToken)
	if err != nil {
		return ResourceListResult{Error: err.Error()}
	}
	var resp struct {
		Labels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("gmail: parse response: %v", err)}
	}
	items := make([]ResourceItem, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		items = append(items, ResourceItem{ID: l.ID, Name: l.Name})
	}
	return ResourceListResult{Items: items}
}

// listSlackResources lists Slack channels or users.
func listSlackResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	token, _ := creds["access_token"].(string)
	if token == "" {
		token, _ = creds["bot_token"].(string)
	}
	if token == "" {
		return ResourceListResult{Error: "slack: access_token or bot_token not found in credential"}
	}
	var apiURL string
	switch resourceType {
	case "channels":
		apiURL = "https://slack.com/api/conversations.list?limit=200&exclude_archived=true"
	case "users":
		apiURL = "https://slack.com/api/users.list?limit=200"
	default:
		return ResourceListResult{Error: fmt.Sprintf("slack: unsupported resource type %q", resourceType)}
	}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceListResult{Error: fmt.Sprintf("slack: http: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var slackResp struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		Members []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Profile struct {
				RealName string `json:"real_name"`
			} `json:"profile"`
		} `json:"members"`
	}
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("slack: parse: %v", err)}
	}
	if !slackResp.OK {
		return ResourceListResult{Error: fmt.Sprintf("slack: API error: %s", slackResp.Error)}
	}
	var items []ResourceItem
	for _, c := range slackResp.Channels {
		items = append(items, ResourceItem{ID: c.ID, Name: "#" + c.Name})
	}
	for _, m := range slackResp.Members {
		displayName := m.Profile.RealName
		if displayName == "" {
			displayName = m.Name
		}
		items = append(items, ResourceItem{ID: m.ID, Name: displayName})
	}
	if items == nil {
		items = []ResourceItem{}
	}
	return ResourceListResult{Items: items}
}

// createGoogleSheet creates a new Google Sheet.
func createGoogleSheet(creds map[string]interface{}, name string) ResourceItemResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceItemResult{Error: "google: access_token not found"}
	}
	payload := fmt.Sprintf(`{"properties":{"title":%q}}`, name)
	req, _ := http.NewRequest("POST", "https://sheets.googleapis.com/v4/spreadsheets",
		bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("google: create sheet: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var created struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Properties    struct {
			Title string `json:"title"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.SpreadsheetID == "" {
		return ResourceItemResult{Error: fmt.Sprintf("google: parse create response: %s", string(body))}
	}
	return ResourceItemResult{Item: &ResourceItem{
		ID:   created.SpreadsheetID,
		Name: created.Properties.Title,
	}}
}

// createGoogleDriveFolder creates a new folder in Google Drive.
func createGoogleDriveFolder(creds map[string]interface{}, name string) ResourceItemResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceItemResult{Error: "google: access_token not found"}
	}
	payload := fmt.Sprintf(`{"name":%q,"mimeType":"application/vnd.google-apps.folder"}`, name)
	req, _ := http.NewRequest("POST", "https://www.googleapis.com/drive/v3/files",
		bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("google drive: create folder: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
		return ResourceItemResult{Error: fmt.Sprintf("google drive: parse create response: %s", string(body))}
	}
	return ResourceItemResult{Item: &ResourceItem{ID: created.ID, Name: created.Name}}
}

// googleAPIGet performs a GET to a Google API endpoint with Bearer auth.
func googleAPIGet(apiURL, accessToken string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google API GET: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
