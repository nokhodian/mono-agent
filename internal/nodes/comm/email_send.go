package comm

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// EmailSendNode sends an email via SMTP with optional attachments.
// Type: "comm.email_send"
//
// Config fields:
//
//	"smtp_host"  (string, required): SMTP server hostname
//	"smtp_port"  (int, default 587): SMTP server port
//	"username"   (string): SMTP auth username
//	"password"   (string): SMTP auth password
//	"from"       (string, required): sender address
//	"to"         (string or []string, required): recipient(s)
//	"cc"         (string or []string): CC recipients
//	"bcc"        (string or []string): BCC recipients
//	"subject"    (string, required): email subject
//	"body"       (string, required): email body
//	"body_type"  (string): "text" (default) or "html"
//	"attachments" ([]string): file paths to attach
type EmailSendNode struct{}

func (n *EmailSendNode) Type() string { return "comm.email_send" }

func (n *EmailSendNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	smtpHost, _ := config["smtp_host"].(string)
	if smtpHost == "" {
		return nil, fmt.Errorf("comm.email_send: smtp_host is required")
	}

	smtpPort := 587
	if p, ok := config["smtp_port"]; ok {
		switch v := p.(type) {
		case int:
			smtpPort = v
		case float64:
			smtpPort = int(v)
		}
	}

	username, _ := config["username"].(string)
	password, _ := config["password"].(string)
	from, _ := config["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("comm.email_send: from is required")
	}

	toAddrs := toStringSlice(config["to"])
	if len(toAddrs) == 0 {
		return nil, fmt.Errorf("comm.email_send: to is required")
	}

	ccAddrs := toStringSlice(config["cc"])
	bccAddrs := toStringSlice(config["bcc"])

	subject, _ := config["subject"].(string)
	if subject == "" {
		return nil, fmt.Errorf("comm.email_send: subject is required")
	}

	body, _ := config["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("comm.email_send: body is required")
	}

	bodyType := "text"
	if bt, ok := config["body_type"].(string); ok && bt != "" {
		bodyType = bt
	}

	var attachmentPaths []string
	if ap, ok := config["attachments"]; ok {
		switch v := ap.(type) {
		case []string:
			attachmentPaths = v
		case []interface{}:
			for _, a := range v {
				if s, ok := a.(string); ok {
					attachmentPaths = append(attachmentPaths, s)
				}
			}
		}
	}

	// Build all recipients for the SMTP envelope.
	allRecipients := make([]string, 0, len(toAddrs)+len(ccAddrs)+len(bccAddrs))
	allRecipients = append(allRecipients, toAddrs...)
	allRecipients = append(allRecipients, ccAddrs...)
	allRecipients = append(allRecipients, bccAddrs...)

	// Build message.
	msgBytes, err := buildMIMEMessage(from, toAddrs, ccAddrs, subject, body, bodyType, attachmentPaths)
	if err != nil {
		return nil, fmt.Errorf("comm.email_send: build message: %w", err)
	}

	addr := net.JoinHostPort(smtpHost, fmt.Sprintf("%d", smtpPort))

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, smtpHost)
	}

	if err := smtp.SendMail(addr, auth, from, allRecipients, msgBytes); err != nil {
		return nil, fmt.Errorf("comm.email_send: send: %w", err)
	}

	result := workflow.NewItem(map[string]interface{}{
		"sent": true,
		"to":   toStringInterfaceSlice(toAddrs),
	})
	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil
}

// buildMIMEMessage constructs a MIME email message with optional HTML body and file attachments.
func buildMIMEMessage(from string, to, cc []string, subject, body, bodyType string, attachments []string) ([]byte, error) {
	var buf bytes.Buffer

	// Headers
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		buf.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	if len(attachments) == 0 {
		// Simple message without attachments.
		contentType := "text/plain"
		if bodyType == "html" {
			contentType = "text/html"
		}
		buf.WriteString("Content-Type: " + contentType + "; charset=\"UTF-8\"\r\n")
		buf.WriteString("Content-Transfer-Encoding: 7bit\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(body)
		return buf.Bytes(), nil
	}

	// Multipart message with attachments.
	mw := multipart.NewWriter(&buf)
	buf.WriteString("Content-Type: multipart/mixed; boundary=\"" + mw.Boundary() + "\"\r\n")
	buf.WriteString("\r\n")

	// Body part.
	bodyContentType := "text/plain; charset=\"UTF-8\""
	if bodyType == "html" {
		bodyContentType = "text/html; charset=\"UTF-8\""
	}
	bh := textproto.MIMEHeader{}
	bh.Set("Content-Type", bodyContentType)
	bh.Set("Content-Transfer-Encoding", "7bit")
	bw, err := mw.CreatePart(bh)
	if err != nil {
		return nil, err
	}
	if _, err := bw.Write([]byte(body)); err != nil {
		return nil, err
	}

	// Attachment parts.
	for _, path := range attachments {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", path, err)
		}
		ah := textproto.MIMEHeader{}
		ah.Set("Content-Type", "application/octet-stream")
		ah.Set("Content-Transfer-Encoding", "base64")
		ah.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
		aw, err := mw.CreatePart(ah)
		if err != nil {
			return nil, err
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		// Wrap base64 at 76 chars per MIME spec.
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			if _, err := aw.Write([]byte(encoded[i:end] + "\r\n")); err != nil {
				return nil, err
			}
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// toStringSlice coerces a config value (string or []string or []interface{}) to []string.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return []string{val}
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func toStringInterfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
