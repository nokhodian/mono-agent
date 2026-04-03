package system

import "github.com/nokhodian/mono-agent/internal/workflow"

// RegisterAll registers all system node types into the registry.
func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("system.execute_command", func() workflow.NodeExecutor { return &ExecuteCommandNode{} })
	r.Register("system.rss_read", func() workflow.NodeExecutor { return &RSSReadNode{} })
}
