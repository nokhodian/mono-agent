package dbnodes

import "github.com/monoes/monoes-agent/internal/workflow"

// RegisterAll registers all database node types into the registry.
func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("db.mysql", func() workflow.NodeExecutor { return &MySQLNode{} })
	r.Register("db.postgres", func() workflow.NodeExecutor { return &PostgresNode{} })
	r.Register("db.mongodb", func() workflow.NodeExecutor { return &MongoDBNode{} })
	r.Register("db.redis", func() workflow.NodeExecutor { return &RedisNode{} })
}
