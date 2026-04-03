package comm

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// EmailReadNode fetches emails from an IMAP server.
// Type: "comm.email_read"
//
// Config fields:
//
//	"imap_host"   (string, required): IMAP server hostname
//	"imap_port"   (int, default 993): IMAP server port
//	"username"    (string): IMAP auth username
//	"password"    (string): IMAP auth password
//	"tls"         (bool, default true): use TLS
//	"mailbox"     (string, default "INBOX"): mailbox to read
//	"limit"       (int, default 10): max messages to fetch
//	"unread_only" (bool, default false): only return unseen messages
//
// Returns each message as an Item with fields:
// "subject", "from", "date", "body", "message_id"
//
// NOTE: This node requires the go-imap dependency.
// To enable it run: go get github.com/emersion/go-imap/v2
type EmailReadNode struct{}

func (n *EmailReadNode) Type() string { return "comm.email_read" }

func (n *EmailReadNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	return nil, fmt.Errorf("comm.email_read: go-imap dependency not installed; run go get github.com/emersion/go-imap/v2")
}
