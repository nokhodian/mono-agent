package data

import (
	"bytes"
	"context"
	"fmt"

	"github.com/monoes/monoes-agent/internal/workflow"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// MarkdownNode converts Markdown text to HTML.
// Type: "data.markdown"
type MarkdownNode struct{}

func (n *MarkdownNode) Type() string { return "data.markdown" }

func (n *MarkdownNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	field, _ := config["field"].(string)
	outputField, _ := config["output_field"].(string)
	unsafe, _ := config["unsafe"].(bool)

	if outputField == "" {
		outputField = field
	}

	opts := []goldmark.Option{
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	}
	if unsafe {
		opts = append(opts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}
	md := goldmark.New(opts...)

	outItems := make([]workflow.Item, 0, len(input.Items))

	for _, item := range input.Items {
		newJSON := copyMap(item.JSON)

		src, _ := newJSON[field].(string)
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			return nil, fmt.Errorf("data.markdown: convert field %q: %w", field, err)
		}
		newJSON[outputField] = buf.String()

		outItems = append(outItems, workflow.Item{JSON: newJSON, Binary: item.Binary})
	}

	return []workflow.NodeOutput{{Handle: "main", Items: outItems}}, nil
}
