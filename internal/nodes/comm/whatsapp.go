package comm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// WhatsAppNode sends messages via the WhatsApp Business Cloud API.
// Type: "comm.whatsapp"
//
// Config fields:
//
//	"operation"         (string, required): "send_message" | "send_template" | "send_media"
//	"access_token"      (string, required): WhatsApp Business API bearer token
//	"phone_number_id"   (string, required): WhatsApp Business phone number ID
//	"to"                (string, required): recipient phone number (E.164, no '+')
//	"message_type"      (string): "text" | "template" | "image" (defaults to operation)
//	"text"              (string): message body text (send_message)
//	"template_name"     (string): template name (send_template)
//	"template_language" (string): template language code (default "en_US")
//	"media_url"         (string): publicly accessible media URL (send_media)
type WhatsAppNode struct{}

func (n *WhatsAppNode) Type() string { return "comm.whatsapp" }

const whatsappAPIBase = "https://graph.facebook.com/v19.0"

func (n *WhatsAppNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	accessToken, _ := config["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("comm.whatsapp: access_token is required")
	}
	phoneNumberID, _ := config["phone_number_id"].(string)
	if phoneNumberID == "" {
		return nil, fmt.Errorf("comm.whatsapp: phone_number_id is required")
	}
	to, _ := config["to"].(string)
	if to == "" {
		return nil, fmt.Errorf("comm.whatsapp: to is required")
	}
	operation, _ := config["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("comm.whatsapp: operation is required")
	}

	endpoint := fmt.Sprintf("%s/%s/messages", whatsappAPIBase, phoneNumberID)

	var payload map[string]interface{}

	switch operation {
	case "send_message":
		text, _ := config["text"].(string)
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                to,
			"type":              "text",
			"text": map[string]interface{}{
				"preview_url": false,
				"body":        text,
			},
		}

	case "send_template":
		templateName, _ := config["template_name"].(string)
		if templateName == "" {
			return nil, fmt.Errorf("comm.whatsapp: template_name is required for send_template")
		}
		lang := "en_US"
		if l, ok := config["template_language"].(string); ok && l != "" {
			lang = l
		}
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                to,
			"type":              "template",
			"template": map[string]interface{}{
				"name":     templateName,
				"language": map[string]interface{}{"code": lang},
			},
		}

	case "send_media":
		mediaURL, _ := config["media_url"].(string)
		if mediaURL == "" {
			return nil, fmt.Errorf("comm.whatsapp: media_url is required for send_media")
		}
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                to,
			"type":              "image",
			"image": map[string]interface{}{
				"link": mediaURL,
			},
		}

	default:
		return nil, fmt.Errorf("comm.whatsapp: unsupported operation %q", operation)
	}

	respMap, err := whatsappPost(ctx, endpoint, accessToken, payload)
	if err != nil {
		return nil, fmt.Errorf("comm.whatsapp: %s: %w", operation, err)
	}

	// Extract message ID from the messages array.
	messageID := ""
	if messages, ok := respMap["messages"].([]interface{}); ok && len(messages) > 0 {
		if m, ok := messages[0].(map[string]interface{}); ok {
			messageID, _ = m["id"].(string)
		}
	}

	result := workflow.NewItem(map[string]interface{}{
		"message_id": messageID,
		"status":     "sent",
	})
	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil
}

// whatsappPost sends a JSON POST request to the WhatsApp Cloud API.
func whatsappPost(ctx context.Context, endpoint, accessToken string, payload map[string]interface{}) (map[string]interface{}, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if errObj, ok := result["error"].(map[string]interface{}); ok {
			msg, _ := errObj["message"].(string)
			code, _ := errObj["code"].(float64)
			return nil, fmt.Errorf("HTTP %d (code %d): %s", resp.StatusCode, int(code), msg)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	return result, nil
}
