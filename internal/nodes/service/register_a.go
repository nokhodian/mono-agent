package service

import "github.com/monoes/monoes-agent/internal/workflow"

// RegisterGroupA registers all Wave 5 group A service nodes into the given registry.
func RegisterGroupA(r *workflow.NodeTypeRegistry) {
	r.Register("service.github", func() workflow.NodeExecutor { return &GitHubNode{} })
	r.Register("service.airtable", func() workflow.NodeExecutor { return &AirtableNode{} })
	r.Register("service.notion", func() workflow.NodeExecutor { return &NotionNode{} })
	r.Register("service.jira", func() workflow.NodeExecutor { return &JiraNode{} })
	r.Register("service.linear", func() workflow.NodeExecutor { return &LinearNode{} })
	r.Register("service.asana", func() workflow.NodeExecutor { return &AsanaNode{} })
}
