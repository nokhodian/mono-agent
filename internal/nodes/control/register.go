package control

import "github.com/nokhodian/mono-agent/internal/workflow"

// RegisterAll registers all control node types in the given registry.
func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("core.if", func() workflow.NodeExecutor { return &IfNode{} })
	r.Register("core.switch", func() workflow.NodeExecutor { return &SwitchNode{} })
	r.Register("core.merge", func() workflow.NodeExecutor { return &MergeNode{} })
	r.Register("core.split_in_batches", func() workflow.NodeExecutor { return &SplitInBatchesNode{} })
	r.Register("core.wait", func() workflow.NodeExecutor { return &WaitNode{} })
	r.Register("core.stop_error", func() workflow.NodeExecutor { return &StopErrorNode{} })
	r.Register("core.set", func() workflow.NodeExecutor { return &SetNode{} })
	r.Register("core.code", func() workflow.NodeExecutor { return &CodeNode{} })
	r.Register("core.filter", func() workflow.NodeExecutor { return &FilterNode{} })
	r.Register("core.sort", func() workflow.NodeExecutor { return &SortNode{} })
	r.Register("core.limit", func() workflow.NodeExecutor { return &LimitNode{} })
	r.Register("core.remove_duplicates", func() workflow.NodeExecutor { return &RemoveDuplicatesNode{} })
	r.Register("core.compare_datasets", func() workflow.NodeExecutor { return &CompareDatasetsNode{} })
	r.Register("core.aggregate", func() workflow.NodeExecutor { return &AggregateNode{} })
}
