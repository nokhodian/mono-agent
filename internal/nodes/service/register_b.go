package service

import "github.com/monoes/monoes-agent/internal/workflow"

func RegisterGroupB(r *workflow.NodeTypeRegistry) {
	r.Register("service.stripe", func() workflow.NodeExecutor { return &StripeNode{} })
	r.Register("service.shopify", func() workflow.NodeExecutor { return &ShopifyNode{} })
	r.Register("service.salesforce", func() workflow.NodeExecutor { return &SalesforceNode{} })
	r.Register("service.hubspot", func() workflow.NodeExecutor { return &HubSpotNode{} })
	r.Register("service.google_sheets", func() workflow.NodeExecutor { return &GoogleSheetsNode{} })
	r.Register("service.gmail", func() workflow.NodeExecutor { return &GmailNode{} })
	r.Register("service.google_drive", func() workflow.NodeExecutor { return &GoogleDriveNode{} })
	r.Register("service.openrouter", func() workflow.NodeExecutor { return &OpenRouterNode{} })
}
