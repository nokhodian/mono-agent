package data

import "github.com/monoes/monoes-agent/internal/workflow"

func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("data.datetime", func() workflow.NodeExecutor { return &DateTimeNode{} })
	r.Register("data.crypto", func() workflow.NodeExecutor { return &CryptoNode{} })
	r.Register("data.html", func() workflow.NodeExecutor { return &HTMLNode{} })
	r.Register("data.xml", func() workflow.NodeExecutor { return &XMLNode{} })
	r.Register("data.markdown", func() workflow.NodeExecutor { return &MarkdownNode{} })
	r.Register("data.spreadsheet", func() workflow.NodeExecutor { return &SpreadsheetNode{} })
	r.Register("data.compression", func() workflow.NodeExecutor { return &CompressionNode{} })
	r.Register("data.write_binary_file", func() workflow.NodeExecutor { return &WriteBinaryFileNode{} })
}
