package comm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// TwilioNode sends SMS / WhatsApp messages or initiates voice calls via the Twilio REST API.
// Type: "comm.twilio"
//
// Config fields:
//
//	"operation"    (string, required): "send_sms" | "send_whatsapp" | "make_call"
//	"account_sid"  (string, required): Twilio Account SID
//	"auth_token"   (string, required): Twilio Auth Token
//	"from"         (string, required): Twilio phone number (E.164 format)
//	"to"           (string, required): recipient phone number
//	"body"         (string): SMS / WhatsApp message body
//	"twiml"        (string): inline TwiML for voice calls
//	"url"          (string): TwiML URL for voice calls (mutually exclusive with twiml)
type TwilioNode struct{}

func (n *TwilioNode) Type() string { return "comm.twilio" }

func (n *TwilioNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	accountSID, _ := config["account_sid"].(string)
	if accountSID == "" {
		return nil, fmt.Errorf("comm.twilio: account_sid is required")
	}
	authToken, _ := config["auth_token"].(string)
	if authToken == "" {
		return nil, fmt.Errorf("comm.twilio: auth_token is required")
	}
	from, _ := config["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("comm.twilio: from is required")
	}
	to, _ := config["to"].(string)
	if to == "" {
		return nil, fmt.Errorf("comm.twilio: to is required")
	}
	operation, _ := config["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("comm.twilio: operation is required")
	}

	switch operation {
	case "send_sms":
		body, _ := config["body"].(string)
		result, err := twilioSendMessage(ctx, accountSID, authToken, from, to, body)
		if err != nil {
			return nil, fmt.Errorf("comm.twilio: send_sms: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil

	case "send_whatsapp":
		body, _ := config["body"].(string)
		// WhatsApp messages use "whatsapp:" prefix on both from and to.
		waFrom := ensurePrefix(from, "whatsapp:")
		waTo := ensurePrefix(to, "whatsapp:")
		result, err := twilioSendMessage(ctx, accountSID, authToken, waFrom, waTo, body)
		if err != nil {
			return nil, fmt.Errorf("comm.twilio: send_whatsapp: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil

	case "make_call":
		twimlURL, _ := config["url"].(string)
		twiml, _ := config["twiml"].(string)
		if twimlURL == "" && twiml == "" {
			return nil, fmt.Errorf("comm.twilio: make_call requires either url or twiml")
		}
		result, err := twilioMakeCall(ctx, accountSID, authToken, from, to, twimlURL, twiml)
		if err != nil {
			return nil, fmt.Errorf("comm.twilio: make_call: %w", err)
		}
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil

	default:
		return nil, fmt.Errorf("comm.twilio: unsupported operation %q", operation)
	}
}

// twilioSendMessage POSTs to the Twilio Messages resource.
func twilioSendMessage(ctx context.Context, accountSID, authToken, from, to, body string) (workflow.Item, error) {
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)

	formData := url.Values{}
	formData.Set("From", from)
	formData.Set("To", to)
	formData.Set("Body", body)

	respMap, err := twilioPost(ctx, endpoint, accountSID, authToken, formData)
	if err != nil {
		return workflow.Item{}, err
	}
	return workflow.NewItem(map[string]interface{}{
		"sid":    respMap["sid"],
		"status": respMap["status"],
	}), nil
}

// twilioMakeCall POSTs to the Twilio Calls resource.
func twilioMakeCall(ctx context.Context, accountSID, authToken, from, to, twimlURL, twiml string) (workflow.Item, error) {
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", accountSID)

	formData := url.Values{}
	formData.Set("From", from)
	formData.Set("To", to)
	if twimlURL != "" {
		formData.Set("Url", twimlURL)
	} else {
		formData.Set("Twiml", twiml)
	}

	respMap, err := twilioPost(ctx, endpoint, accountSID, authToken, formData)
	if err != nil {
		return workflow.Item{}, err
	}
	return workflow.NewItem(map[string]interface{}{
		"sid":    respMap["sid"],
		"status": respMap["status"],
	}), nil
}

// twilioPost performs an authenticated form POST to a Twilio REST endpoint and
// returns the decoded JSON response body.
func twilioPost(ctx context.Context, endpoint, accountSID, authToken string, formData url.Values) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(accountSID, authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg, _ := result["message"].(string)
		code, _ := result["code"].(float64)
		return nil, fmt.Errorf("HTTP %d (Twilio code %d): %s", resp.StatusCode, int(code), msg)
	}
	return result, nil
}

// ensurePrefix adds the given prefix to s if not already present.
func ensurePrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}
