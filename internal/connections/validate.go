package connections

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ValidateConnection calls the platform-specific validator and returns the
// resolved account identifier (username/email/org name). Returns ("", nil)
// for platforms with no validation or browser-session platforms.
func ValidateConnection(ctx context.Context, c *Connection) (accountID string, err error) {
	switch c.Platform {
	case "github":
		return validateGitHub(ctx, c)
	case "notion":
		return validateNotion(ctx, c)
	case "airtable":
		return validateAirtable(ctx, c)
	case "jira":
		return validateJira(ctx, c)
	case "linear":
		return validateLinear(ctx, c)
	case "stripe":
		return validateStripe(ctx, c)
	case "slack":
		return validateSlack(ctx, c)
	case "discord":
		return validateDiscord(ctx, c)
	case "twilio":
		return validateTwilio(ctx, c)
	case "telegram":
		return validateTelegram(ctx, c)
	case "postgresql", "mysql", "mongodb", "redis":
		cs := getStr(c.Data, "connection_string")
		if cs == "" {
			return "", fmt.Errorf("validate %s: missing connection_string", c.Platform)
		}
		end := min(len(cs), 30)
		return cs[:end] + "...", nil
	default:
		return "", nil
	}
}

// validateGitHub validates a GitHub connection using the token or access_token field.
func validateGitHub(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "token")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	if token == "" {
		return "", fmt.Errorf("validateGitHub: missing token or access_token")
	}
	body, status, err := doGET(ctx, "https://api.github.com/user", "Bearer "+token)
	if err != nil {
		return "", fmt.Errorf("validateGitHub: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("validateGitHub: unexpected status %d", status)
	}
	var resp struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("validateGitHub: parse response: %w", err)
	}
	if resp.Login == "" {
		return "", fmt.Errorf("validateGitHub: empty login in response")
	}
	return resp.Login, nil
}

// validateNotion validates a Notion connection using the token or access_token field.
func validateNotion(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "token")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.notion.com/v1/users/me", nil)
	if err != nil {
		return "", fmt.Errorf("validateNotion: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("validateNotion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("validateNotion: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("validateNotion: unexpected status %d", resp.StatusCode)
	}

	var r struct {
		Name string `json:"name"`
		Bot  struct {
			WorkspaceName string `json:"workspace_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("validateNotion: parse response: %w", err)
	}

	if r.Bot.WorkspaceName != "" {
		return r.Bot.WorkspaceName, nil
	}
	if r.Name != "" {
		return r.Name, nil
	}
	return "", fmt.Errorf("validateNotion: could not resolve account name from response")
}

// validateAirtable validates an Airtable connection using the api_key or access_token field.
func validateAirtable(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "api_key")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}
	body, status, err := doGET(ctx, "https://api.airtable.com/v0/meta/whoami", "Bearer "+token)
	if err != nil {
		return "", fmt.Errorf("validateAirtable: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("validateAirtable: unexpected status %d", status)
	}
	var r struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Email != "" {
		return r.Email, nil
	}
	return r.ID, nil
}

// validateJira validates a Jira connection using email, api_token, and domain fields.
func validateJira(ctx context.Context, c *Connection) (string, error) {
	email := getStr(c.Data, "email")
	apiToken := getStr(c.Data, "api_token")
	domain := getStr(c.Data, "domain")

	if email == "" {
		return "", fmt.Errorf("validateJira: missing email")
	}
	if apiToken == "" {
		return "", fmt.Errorf("validateJira: missing api_token")
	}
	if domain == "" {
		return "", fmt.Errorf("validateJira: missing domain")
	}

	url := fmt.Sprintf("https://%s/rest/api/3/myself", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("validateJira: create request: %w", err)
	}
	req.SetBasicAuth(email, apiToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("validateJira: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("validateJira: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("validateJira: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("validateJira: parse response: %w", err)
	}
	return result.DisplayName, nil
}

// validateLinear validates a Linear connection using the api_key or access_token field.
func validateLinear(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "api_key")
	if token == "" {
		token = getStr(c.Data, "access_token")
	}

	bodyStr := `{"query":"{ viewer { name email } }"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/graphql", strings.NewReader(bodyStr))
	if err != nil {
		return "", fmt.Errorf("validateLinear: create request: %w", err)
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("validateLinear: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("validateLinear: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("validateLinear: unexpected status %d", resp.StatusCode)
	}

	var r struct {
		Data struct {
			Viewer struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("validateLinear: parse response: %w", err)
	}
	if r.Data.Viewer.Email != "" {
		return r.Data.Viewer.Email, nil
	}
	if r.Data.Viewer.Name != "" {
		return r.Data.Viewer.Name, nil
	}
	return "", fmt.Errorf("validateLinear: viewer has no email or name")
}

// validateStripe validates a Stripe connection using the secret_key field.
func validateStripe(ctx context.Context, c *Connection) (string, error) {
	key := getStr(c.Data, "secret_key")
	if key == "" {
		return "", fmt.Errorf("validateStripe: missing secret_key")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com/v1/account", nil)
	if err != nil {
		return "", fmt.Errorf("validateStripe: create request: %w", err)
	}
	req.SetBasicAuth(key, "")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("validateStripe: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("validateStripe: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("validateStripe: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		BusinessProfile struct {
			Name string `json:"name"`
		} `json:"business_profile"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("validateStripe: parse response: %w", err)
	}
	if result.BusinessProfile.Name != "" {
		return result.BusinessProfile.Name, nil
	}
	return result.Email, nil
}

// validateSlack validates a Slack connection using the access_token field.
func validateSlack(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "access_token")
	body, status, err := doGET(ctx, "https://slack.com/api/auth.test", "Bearer "+token)
	if err != nil {
		return "", fmt.Errorf("validateSlack: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("validateSlack: unexpected status %d", status)
	}

	var resp struct {
		OK    bool   `json:"ok"`
		Team  string `json:"team"`
		User  string `json:"user"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("validateSlack: parse response: %w", err)
	}
	if !resp.OK {
		return "", fmt.Errorf("validateSlack: api error: %s", resp.Error)
	}
	return fmt.Sprintf("%s / %s", resp.Team, resp.User), nil
}

// validateDiscord validates a Discord bot connection using the bot_token field.
func validateDiscord(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "bot_token")
	body, status, err := doGET(ctx, "https://discord.com/api/v10/users/@me", "Bot "+token)
	if err != nil {
		return "", fmt.Errorf("validateDiscord: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("validateDiscord: unexpected status %d", status)
	}

	var resp struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("validateDiscord: parse response: %w", err)
	}
	return resp.Username, nil
}

// validateTwilio validates a Twilio connection using account_sid and auth_token fields.
func validateTwilio(ctx context.Context, c *Connection) (string, error) {
	accountSID := getStr(c.Data, "account_sid")
	authToken := getStr(c.Data, "auth_token")

	if accountSID == "" || authToken == "" {
		return "", fmt.Errorf("validateTwilio: missing account_sid or auth_token")
	}

	url := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("validateTwilio: create request: %w", err)
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("validateTwilio: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("validateTwilio: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("validateTwilio: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		FriendlyName string `json:"friendly_name"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("validateTwilio: parse response: %w", err)
	}
	return result.FriendlyName, nil
}

// validateTelegram validates a Telegram bot token using the getMe endpoint.
func validateTelegram(ctx context.Context, c *Connection) (string, error) {
	token := getStr(c.Data, "bot_token")
	if token == "" {
		return "", fmt.Errorf("validateTelegram: missing bot_token")
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	body, status, err := doGET(ctx, url, "")
	if err != nil {
		return "", fmt.Errorf("validateTelegram: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("validateTelegram: invalid token (status %d)", status)
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("validateTelegram: parse response: %w", err)
	}
	if !resp.OK {
		return "", fmt.Errorf("validateTelegram: token rejected by Telegram")
	}
	if resp.Result.Username != "" {
		return "@" + resp.Result.Username, nil
	}
	return resp.Result.FirstName, nil
}

// getStr extracts a string value from a map, returning "" if missing or not a string.
func getStr(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// doGET makes a GET request with the given Authorization header (empty = no header),
// sets Accept: application/json, and returns body bytes, HTTP status code, and any error.
func doGET(ctx context.Context, url, authHeader string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("doGET: create request: %w", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("doGET: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("doGET: read body: %w", err)
	}
	return body, resp.StatusCode, nil
}
