package httpnodes

import "github.com/nokhodian/mono-agent/internal/workflow"

// RegisterAll registers all HTTP node types into the registry.
func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("http.request", func() workflow.NodeExecutor { return &RequestNode{} })
	r.Register("http.ftp", func() workflow.NodeExecutor { return &FTPNode{} })
	r.Register("http.ssh", func() workflow.NodeExecutor { return &SSHNode{} })
}
