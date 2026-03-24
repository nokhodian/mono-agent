package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// ShopifyNode implements the service.shopify node type.
type ShopifyNode struct{}

func (n *ShopifyNode) Type() string { return "service.shopify" }

// shopifyRequest makes an authenticated request to the Shopify Admin API.
func shopifyRequest(ctx context.Context, method, shop, accessToken, path string, body interface{}) (map[string]interface{}, error) {
	fullURL := fmt.Sprintf("https://%s.myshopify.com/admin/api/2024-01%s", shop, path)
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("shopify: marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("shopify: creating request: %w", err)
	}
	req.Header.Set("X-Shopify-Access-Token", accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("shopify %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("shopify: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("shopify HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("shopify: parsing JSON: %w", err)
	}
	return result, nil
}

func (n *ShopifyNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	shop := strVal(config, "shop")
	if shop == "" {
		return nil, fmt.Errorf("shopify: shop is required")
	}
	accessToken := strVal(config, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("shopify: access_token is required")
	}
	operation := strVal(config, "operation")
	limit := intVal(config, "limit")
	if limit == 0 {
		limit = 50
	}

	var items []workflow.Item

	switch operation {
	case "list_products":
		path := "/products.json?limit=" + strconv.Itoa(limit)
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, path, nil)
		if err != nil {
			return nil, err
		}
		items = shopifyExtractList(data, "products")

	case "get_product":
		productID := strVal(config, "product_id")
		if productID == "" {
			return nil, fmt.Errorf("shopify: product_id is required for get_product")
		}
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, "/products/"+productID+".json", nil)
		if err != nil {
			return nil, err
		}
		if product, ok := data["product"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(product)}
		}

	case "create_product":
		product := map[string]interface{}{}
		if title := strVal(config, "title"); title != "" {
			product["title"] = title
		}
		if vendor := strVal(config, "vendor"); vendor != "" {
			product["vendor"] = vendor
		}
		if productType := strVal(config, "product_type"); productType != "" {
			product["product_type"] = productType
		}
		body := map[string]interface{}{"product": product}
		data, err := shopifyRequest(ctx, "POST", shop, accessToken, "/products.json", body)
		if err != nil {
			return nil, err
		}
		if product, ok := data["product"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(product)}
		}

	case "update_product":
		productID := strVal(config, "product_id")
		if productID == "" {
			return nil, fmt.Errorf("shopify: product_id is required for update_product")
		}
		product := map[string]interface{}{"id": productID}
		if title := strVal(config, "title"); title != "" {
			product["title"] = title
		}
		if vendor := strVal(config, "vendor"); vendor != "" {
			product["vendor"] = vendor
		}
		if productType := strVal(config, "product_type"); productType != "" {
			product["product_type"] = productType
		}
		body := map[string]interface{}{"product": product}
		data, err := shopifyRequest(ctx, "PUT", shop, accessToken, "/products/"+productID+".json", body)
		if err != nil {
			return nil, err
		}
		if p, ok := data["product"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(p)}
		}

	case "list_orders":
		path := "/orders.json?limit=" + strconv.Itoa(limit)
		if status := strVal(config, "status"); status != "" {
			path += "&status=" + status
		}
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, path, nil)
		if err != nil {
			return nil, err
		}
		items = shopifyExtractList(data, "orders")

	case "get_order":
		orderID := strVal(config, "order_id")
		if orderID == "" {
			return nil, fmt.Errorf("shopify: order_id is required for get_order")
		}
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, "/orders/"+orderID+".json", nil)
		if err != nil {
			return nil, err
		}
		if order, ok := data["order"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(order)}
		}

	case "update_order":
		orderID := strVal(config, "order_id")
		if orderID == "" {
			return nil, fmt.Errorf("shopify: order_id is required for update_order")
		}
		order := map[string]interface{}{"id": orderID}
		if status := strVal(config, "status"); status != "" {
			order["financial_status"] = status
		}
		body := map[string]interface{}{"order": order}
		data, err := shopifyRequest(ctx, "PUT", shop, accessToken, "/orders/"+orderID+".json", body)
		if err != nil {
			return nil, err
		}
		if o, ok := data["order"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(o)}
		}

	case "list_customers":
		path := "/customers.json?limit=" + strconv.Itoa(limit)
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, path, nil)
		if err != nil {
			return nil, err
		}
		items = shopifyExtractList(data, "customers")

	case "get_customer":
		customerID := strVal(config, "customer_id")
		if customerID == "" {
			return nil, fmt.Errorf("shopify: customer_id is required for get_customer")
		}
		data, err := shopifyRequest(ctx, "GET", shop, accessToken, "/customers/"+customerID+".json", nil)
		if err != nil {
			return nil, err
		}
		if customer, ok := data["customer"].(map[string]interface{}); ok {
			items = []workflow.Item{workflow.NewItem(customer)}
		}

	default:
		return nil, fmt.Errorf("shopify: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// shopifyExtractList extracts a named list from a Shopify response and converts to Items.
func shopifyExtractList(data map[string]interface{}, key string) []workflow.Item {
	raw, _ := data[key].([]interface{})
	items := make([]workflow.Item, 0, len(raw))
	for _, d := range raw {
		if m, ok := d.(map[string]interface{}); ok {
			items = append(items, workflow.NewItem(m))
		}
	}
	return items
}
