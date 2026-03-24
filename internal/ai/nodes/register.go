package ainodes

import (
	"github.com/monoes/monoes-agent/internal/ai"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// RegisterAll registers all AI node types into the given registry.
// The aiStore is captured by the factory closures so each node gets access to providers.
func RegisterAll(r *workflow.NodeTypeRegistry, store *ai.AIStore) {
	r.Register("ai.chat", func() workflow.NodeExecutor { return &ChatNode{Store: store} })
	r.Register("ai.extract", func() workflow.NodeExecutor { return &ExtractNode{Store: store} })
	r.Register("ai.classify", func() workflow.NodeExecutor { return &ClassifyNode{Store: store} })
	r.Register("ai.transform", func() workflow.NodeExecutor { return &TransformNode{Store: store} })
	r.Register("ai.embed", func() workflow.NodeExecutor { return &EmbedNode{Store: store} })
	r.Register("ai.agent", func() workflow.NodeExecutor { return &AgentNode{Store: store} })
}
