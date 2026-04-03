package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// GmailNode implements the service.gmail node type.
type GmailNode struct{}

func (n *GmailNode) Type() string { return "service.gmail" }

const gmailBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"

func (n *GmailNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	accessToken := strVal(config, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("gmail: access_token is required")
	}
	operation := strVal(config, "operation")
	maxResults := intVal(config, "max_results")
	if maxResults == 0 {
		maxResults = 10
	}

	var items []workflow.Item

	switch operation {
	case "send_message":
		to := strVal(config, "to")
		from := strVal(config, "from")
		subject := strVal(config, "subject")
		body := strVal(config, "body")
		bodyType := strVal(config, "body_type")
		if bodyType == "" {
			bodyType = "text"
		}
		raw, err := gmailBuildRFC2822(from, to, subject, body, bodyType)
		if err != nil {
			return nil, fmt.Errorf("gmail: building message: %w", err)
		}
		sendBody := map[string]interface{}{"raw": raw}
		resp, err := gmailRequest(ctx, "POST", gmailBaseURL+"/messages/send", accessToken, sendBody)
		if err != nil {
			return nil, fmt.Errorf("gmail send_message: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(resp)}

	case "list_messages":
		url := fmt.Sprintf("%s/messages?maxResults=%d", gmailBaseURL, maxResults)
		if q := strVal(config, "query"); q != "" {
			url += "&q=" + gmailURLEncode(q)
		}
		labelIDs := strSliceVal(config, "label_ids")
		for _, lid := range labelIDs {
			url += "&labelIds=" + lid
		}
		data, err := gmailRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("gmail list_messages: %w", err)
		}
		messages, _ := data["messages"].([]interface{})
		items = make([]workflow.Item, 0, len(messages))
		for _, m := range messages {
			if msg, ok := m.(map[string]interface{}); ok {
				items = append(items, workflow.NewItem(msg))
			}
		}

	case "get_message":
		messageID := strVal(config, "message_id")
		if messageID == "" {
			return nil, fmt.Errorf("gmail: message_id is required for get_message")
		}
		url := gmailBaseURL + "/messages/" + messageID
		data, err := gmailRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("gmail get_message: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "list_labels":
		data, err := gmailRequest(ctx, "GET", gmailBaseURL+"/labels", accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("gmail list_labels: %w", err)
		}
		labels, _ := data["labels"].([]interface{})
		items = make([]workflow.Item, 0, len(labels))
		for _, l := range labels {
			if label, ok := l.(map[string]interface{}); ok {
				items = append(items, workflow.NewItem(label))
			}
		}

	case "trash_message":
		messageID := strVal(config, "message_id")
		if messageID == "" {
			return nil, fmt.Errorf("gmail: message_id is required for trash_message")
		}
		url := gmailBaseURL + "/messages/" + messageID + "/trash"
		data, err := gmailRequest(ctx, "POST", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("gmail trash_message: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	default:
		return nil, fmt.Errorf("gmail: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// gmailRequest makes an authenticated request to the Gmail API.
func gmailRequest(ctx context.Context, method, url, accessToken string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("gmail: marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("gmail: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gmail: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gmail HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("gmail: parsing JSON: %w", err)
	}
	return result, nil
}

// gmailBuildRFC2822 constructs an RFC 2822 email message and returns it as a base64url-encoded string.
func gmailBuildRFC2822(from, to, subject, body, bodyType string) (string, error) {
	contentType := "text/plain"
	if bodyType == "html" {
		contentType = "text/html"
	}

	var sb strings.Builder
	if from != "" {
		sb.WriteString("From: " + from + "\r\n")
	}
	if to != "" {
		sb.WriteString("To: " + to + "\r\n")
	}
	if subject != "" {
		sb.WriteString("Subject: " + subject + "\r\n")
	}
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: " + contentType + "; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)

	encoded := base64.URLEncoding.EncodeToString([]byte(sb.String()))
	return encoded, nil
}

// gmailURLEncode encodes a string for use in a URL query parameter.
func gmailURLEncode(s string) string {
	return url.QueryEscape(s)
}
