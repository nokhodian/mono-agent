package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// StripeNode implements the service.stripe node type.
type StripeNode struct{}

func (n *StripeNode) Type() string { return "service.stripe" }

const stripeBaseURL = "https://api.stripe.com/v1"

// stripeFormEncode encodes a map to application/x-www-form-urlencoded format.
// Keys are sorted for deterministic output.
func stripeFormEncode(data map[string]interface{}) string {
	vals := url.Values{}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := data[k]
		if v == nil {
			continue
		}
		vals.Set(k, fmt.Sprintf("%v", v))
	}
	return vals.Encode()
}

// stripeGet makes an authenticated GET request to the Stripe API.
func stripeGet(ctx context.Context, endpoint, apiKey string, params map[string]interface{}) (map[string]interface{}, error) {
	fullURL := stripeBaseURL + endpoint
	if len(params) > 0 {
		fullURL = fullURL + "?" + stripeFormEncode(params)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating stripe GET request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe GET %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading stripe response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stripe HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing stripe JSON: %w", err)
	}
	return result, nil
}

// stripePost makes an authenticated POST request to the Stripe API with form-encoded body.
func stripePost(ctx context.Context, endpoint, apiKey string, params map[string]interface{}) (map[string]interface{}, error) {
	fullURL := stripeBaseURL + endpoint
	var bodyReader io.Reader
	if len(params) > 0 {
		bodyReader = bytes.NewBufferString(stripeFormEncode(params))
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating stripe POST request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe POST %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading stripe response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stripe HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing stripe JSON: %w", err)
	}
	return result, nil
}

// stripeDelete makes an authenticated DELETE request to the Stripe API.
func stripeDelete(ctx context.Context, endpoint, apiKey string) (map[string]interface{}, error) {
	fullURL := stripeBaseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, "DELETE", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating stripe DELETE request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe DELETE %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading stripe response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stripe HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing stripe JSON: %w", err)
	}
	return result, nil
}

// stripeGetList fetches a Stripe list endpoint and returns items from the "data" array.
func stripeGetList(ctx context.Context, endpoint, apiKey string, params map[string]interface{}) ([]workflow.Item, error) {
	data, err := stripeGet(ctx, endpoint, apiKey, params)
	if err != nil {
		return nil, err
	}
	rawData, _ := data["data"].([]interface{})
	items := make([]workflow.Item, 0, len(rawData))
	for _, d := range rawData {
		if m, ok := d.(map[string]interface{}); ok {
			items = append(items, workflow.NewItem(m))
		}
	}
	return items, nil
}

func (n *StripeNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	apiKey := strVal(config, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("stripe: api_key is required")
	}
	operation := strVal(config, "operation")
	limit := intVal(config, "limit")
	if limit == 0 {
		limit = 10
	}
	currency := strVal(config, "currency")
	if currency == "" {
		currency = "usd"
	}

	var items []workflow.Item

	switch operation {
	case "list_customers":
		params := map[string]interface{}{"limit": strconv.Itoa(limit)}
		data, err := stripeGetList(ctx, "/customers", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = data

	case "create_customer":
		params := map[string]interface{}{}
		if email := strVal(config, "email"); email != "" {
			params["email"] = email
		}
		if name := strVal(config, "name"); name != "" {
			params["name"] = name
		}
		data, err := stripePost(ctx, "/customers", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "get_customer":
		customerID := strVal(config, "customer_id")
		if customerID == "" {
			return nil, fmt.Errorf("stripe: customer_id is required for get_customer")
		}
		data, err := stripeGet(ctx, "/customers/"+customerID, apiKey, nil)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "list_charges":
		params := map[string]interface{}{"limit": strconv.Itoa(limit)}
		if customerID := strVal(config, "customer_id"); customerID != "" {
			params["customer"] = customerID
		}
		data, err := stripeGetList(ctx, "/charges", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = data

	case "create_charge":
		amount := intVal(config, "amount")
		if amount == 0 {
			return nil, fmt.Errorf("stripe: amount is required for create_charge")
		}
		params := map[string]interface{}{
			"amount":   strconv.Itoa(amount),
			"currency": currency,
		}
		if source := strVal(config, "source"); source != "" {
			params["source"] = source
		}
		if customerID := strVal(config, "customer_id"); customerID != "" {
			params["customer"] = customerID
		}
		data, err := stripePost(ctx, "/charges", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "list_subscriptions":
		params := map[string]interface{}{"limit": strconv.Itoa(limit)}
		if customerID := strVal(config, "customer_id"); customerID != "" {
			params["customer"] = customerID
		}
		data, err := stripeGetList(ctx, "/subscriptions", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = data

	case "create_subscription":
		customerID := strVal(config, "customer_id")
		if customerID == "" {
			return nil, fmt.Errorf("stripe: customer_id is required for create_subscription")
		}
		priceID := strVal(config, "price_id")
		if priceID == "" {
			return nil, fmt.Errorf("stripe: price_id is required for create_subscription")
		}
		params := map[string]interface{}{
			"customer":        customerID,
			"items[0][price]": priceID,
		}
		data, err := stripePost(ctx, "/subscriptions", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "cancel_subscription":
		subscriptionID := strVal(config, "subscription_id")
		if subscriptionID == "" {
			return nil, fmt.Errorf("stripe: subscription_id is required for cancel_subscription")
		}
		data, err := stripeDelete(ctx, "/subscriptions/"+subscriptionID, apiKey)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "list_products":
		params := map[string]interface{}{"limit": strconv.Itoa(limit)}
		data, err := stripeGetList(ctx, "/products", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = data

	case "create_payment_intent":
		amount := intVal(config, "amount")
		if amount == 0 {
			return nil, fmt.Errorf("stripe: amount is required for create_payment_intent")
		}
		params := map[string]interface{}{
			"amount":   strconv.Itoa(amount),
			"currency": currency,
		}
		if customerID := strVal(config, "customer_id"); customerID != "" {
			params["customer"] = customerID
		}
		data, err := stripePost(ctx, "/payment_intents", apiKey, params)
		if err != nil {
			return nil, err
		}
		items = []workflow.Item{workflow.NewItem(data)}

	default:
		return nil, fmt.Errorf("stripe: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}
