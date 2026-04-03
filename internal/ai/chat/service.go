package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nokhodian/mono-agent/internal/ai"
)

const systemPromptTemplate = `You are a workflow builder AI inside Mono Agent. You ONLY communicate by calling tool functions. NEVER describe what you would do — ALWAYS call the tools directly.

RULES:
1. If workflow_id is "general" or "draft", call create_workflow FIRST.
2. Then call create_nodes with all needed nodes in ONE call.
3. Then call connect_nodes for each connection.
4. Respond with a brief summary ONLY after all tool calls are done.
5. Use "main" as source_handle and target_handle for connections.
6. Space nodes: increment position_x by 250 per node.

NODE TYPES (use these exact type values in create_nodes):
Triggers: trigger.manual, trigger.schedule, trigger.webhook
Control: core.if, core.switch, core.merge, core.split_in_batches, core.wait, core.stop_error, core.set, core.code, core.filter, core.sort, core.limit, core.remove_duplicates, core.compare_datasets, core.aggregate
HTTP: http.request, http.ftp, http.ssh
Data: data.datetime, data.crypto, data.html, data.xml, data.markdown, data.spreadsheet, data.compression, data.write_binary_file
DB: db.mysql, db.postgres, db.mongodb, db.redis
Comm: comm.email_send, comm.email_read, comm.slack, comm.telegram, comm.discord, comm.twilio, comm.whatsapp
Services: service.github, service.airtable, service.notion, service.jira, service.linear, service.asana, service.stripe, service.shopify, service.salesforce, service.hubspot, service.google_sheets, service.gmail, service.google_drive
AI: ai.chat, ai.extract, ai.classify, ai.transform, ai.embed, ai.agent
Instagram: instagram.find_by_keyword, instagram.export_followers, instagram.scrape_profile_info, instagram.engage_with_posts, instagram.engage_user_posts, instagram.send_dms, instagram.auto_reply_dms, instagram.publish_post, instagram.like_posts, instagram.comment_on_posts, instagram.like_comments_on_posts, instagram.follow_users, instagram.unfollow_users, instagram.extract_post_data, instagram.watch_stories
LinkedIn: linkedin.find_by_keyword, linkedin.export_followers, linkedin.scrape_profile_info, linkedin.engage_with_posts, linkedin.send_dms, linkedin.auto_reply_dms, linkedin.publish_post
X: x.find_by_keyword, x.export_followers, x.scrape_profile_info, x.engage_with_posts, x.send_dms, x.auto_reply_dms, x.publish_post
TikTok: tiktok.find_by_keyword, tiktok.export_followers, tiktok.scrape_profile_info, tiktok.engage_with_posts, tiktok.send_dms, tiktok.auto_reply_dms, tiktok.publish_post`

// maxToolRounds limits how many tool-call round-trips we allow before stopping,
// preventing infinite loops.
const maxToolRounds = 10

// NewClientFunc creates an AIClient from a provider config. It is a field on
// ChatService so tests can inject a mock client without needing real providers.
type NewClientFunc func(provider ai.AIProvider) (ai.AIClient, error)

// ChatService orchestrates AI chat interactions for workflows.
type ChatService struct {
	aiStore      *ai.AIStore
	db           *sql.DB
	newClientFn  NewClientFunc
	canvasTools  *CanvasTools
}

// NewChatService creates a ChatService wired to the given store and database.
func NewChatService(aiStore *ai.AIStore, db *sql.DB) *ChatService {
	return &ChatService{
		aiStore:     aiStore,
		db:          db,
		newClientFn: ai.NewClient,
		canvasTools: NewCanvasTools(db),
	}
}

// SetCanvasNodeTypes provides the available node types to the canvas tools.
func (s *ChatService) SetCanvasNodeTypes(types []NodeTypeInfo) {
	s.canvasTools.SetNodeTypes(types)
}

// StreamChat sends a user message to the AI provider and streams the response.
//
// onChunk is called for each streamed token. onToolCall is called whenever the
// model invokes a tool, receiving the tool name, arguments JSON, and result.
func (s *ChatService) StreamChat(
	ctx context.Context,
	workflowID, userMessage, providerID, model string,
	onChunk func(ai.StreamChunk),
	onToolCall func(name, args, result string),
) error {
	// 1. Resolve provider and create client.
	provider, err := s.aiStore.GetProvider(providerID)
	if err != nil {
		return fmt.Errorf("get provider %s: %w", providerID, err)
	}

	client, err := s.newClientFn(provider)
	if err != nil {
		return fmt.Errorf("create ai client: %w", err)
	}

	// 2. Load existing history.
	history, err := s.aiStore.GetChatHistory(workflowID)
	if err != nil {
		return fmt.Errorf("get chat history: %w", err)
	}

	// 3. Build messages array: system + history + new user message.
	systemPrompt := systemPromptTemplate + fmt.Sprintf("\n\nCurrent workflow_id: %s", workflowID)
	messages := make([]ai.Message, 0, len(history)+2)
	messages = append(messages, ai.Message{
		Role:    ai.RoleSystem,
		Content: systemPrompt,
	})
	for _, h := range history {
		msg := ai.Message{
			Role:    h.Role,
			Content: h.Content,
		}
		if h.ToolCalls != "" {
			var tc []ai.ToolCall
			if err := json.Unmarshal([]byte(h.ToolCalls), &tc); err == nil {
				msg.ToolCalls = tc
			}
		}
		if h.ToolCallID != "" {
			msg.ToolCallID = h.ToolCallID
		}
		messages = append(messages, msg)
	}
	messages = append(messages, ai.Message{
		Role:    ai.RoleUser,
		Content: userMessage,
	})

	// 4. Save user message to store.
	if err := s.aiStore.SaveChatMessage(ai.ChatMessage{
		ID:         uuid.New().String(),
		WorkflowID: workflowID,
		Role:       ai.RoleUser,
		Content:    userMessage,
	}); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}

	// 5. Build tool definitions from canvas tools.
	toolDefs := s.canvasTools.ToolDefs()

	// 6. Stream the first response.
	var accumulated string
	var toolCalls []ai.ToolCall

	req := ai.CompletionRequest{
		Model:    model,
		Messages: messages,
		Tools:    toolDefs,
		Stream:   true,
	}

	err = client.StreamComplete(ctx, req, func(chunk ai.StreamChunk) {
		accumulated += chunk.Content
		if len(chunk.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.ToolCalls...)
		}
		if onChunk != nil {
			onChunk(chunk)
		}
	})
	if err != nil {
		return fmt.Errorf("stream complete: %w", err)
	}

	// 7. Tool-call loop (up to maxToolRounds).
	for round := 0; round < maxToolRounds && len(toolCalls) > 0; round++ {
		// Save assistant message that contains tool calls.
		tcJSON, _ := json.Marshal(toolCalls)
		if err := s.aiStore.SaveChatMessage(ai.ChatMessage{
			ID:         uuid.New().String(),
			WorkflowID: workflowID,
			Role:       ai.RoleAssistant,
			Content:    accumulated,
			ToolCalls:  string(tcJSON),
			ProviderID: providerID,
			Model:      model,
		}); err != nil {
			return fmt.Errorf("save assistant tool-call message: %w", err)
		}

		// Add assistant message with tool calls to conversation.
		messages = append(messages, ai.Message{
			Role:      ai.RoleAssistant,
			Content:   accumulated,
			ToolCalls: toolCalls,
		})

		// Execute each tool call and add results.
		for _, tc := range toolCalls {
			result := s.executeTool(tc.Function.Name, tc.Function.Arguments)
			if onToolCall != nil {
				onToolCall(tc.Function.Name, tc.Function.Arguments, result)
			}

			messages = append(messages, ai.Message{
				Role:       ai.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})

			// Persist tool result.
			if err := s.aiStore.SaveChatMessage(ai.ChatMessage{
				ID:         uuid.New().String(),
				WorkflowID: workflowID,
				Role:       ai.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			}); err != nil {
				return fmt.Errorf("save tool result message: %w", err)
			}
		}

		// Continue with a non-streaming Complete call.
		accumulated = ""
		toolCalls = nil

		contReq := ai.CompletionRequest{
			Model:    model,
			Messages: messages,
			Tools:    toolDefs,
		}
		resp, err := client.Complete(ctx, contReq)
		if err != nil {
			return fmt.Errorf("complete (tool continuation round %d): %w", round+1, err)
		}

		accumulated = resp.Content
		toolCalls = resp.ToolCalls

		// Stream the continuation text to the caller.
		if onChunk != nil && accumulated != "" {
			onChunk(ai.StreamChunk{Content: accumulated})
		}
	}

	// 8. Save final assistant message.
	if err := s.aiStore.SaveChatMessage(ai.ChatMessage{
		ID:         uuid.New().String(),
		WorkflowID: workflowID,
		Role:       ai.RoleAssistant,
		Content:    accumulated,
		ProviderID: providerID,
		Model:      model,
	}); err != nil {
		return fmt.Errorf("save assistant message: %w", err)
	}

	return nil
}

// executeTool dispatches a tool call by name via CanvasTools. Returns the result string.
func (s *ChatService) executeTool(name, argsJSON string) string {
	result, err := s.canvasTools.Execute(name, argsJSON)
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return result
}

// GetHistory returns the full chat history for a workflow.
func (s *ChatService) GetHistory(workflowID string) ([]ai.ChatMessage, error) {
	return s.aiStore.GetChatHistory(workflowID)
}

// ClearHistory deletes all chat messages for a workflow.
func (s *ChatService) ClearHistory(workflowID string) error {
	return s.aiStore.ClearChatHistory(workflowID)
}
