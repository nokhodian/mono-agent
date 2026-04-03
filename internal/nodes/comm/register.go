package comm

import "github.com/nokhodian/mono-agent/internal/workflow"

// RegisterAll registers all communication node types in the given registry.
func RegisterAll(r *workflow.NodeTypeRegistry) {
	r.Register("comm.email_send", func() workflow.NodeExecutor { return &EmailSendNode{} })
	r.Register("comm.email_read", func() workflow.NodeExecutor { return &EmailReadNode{} })
	r.Register("comm.slack", func() workflow.NodeExecutor { return &SlackNode{} })
	r.Register("comm.telegram", func() workflow.NodeExecutor { return &TelegramNode{} })
	r.Register("comm.discord", func() workflow.NodeExecutor { return &DiscordNode{} })
	r.Register("comm.twilio", func() workflow.NodeExecutor { return &TwilioNode{} })
	r.Register("comm.whatsapp", func() workflow.NodeExecutor { return &WhatsAppNode{} })
}
