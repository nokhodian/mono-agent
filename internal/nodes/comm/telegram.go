package comm

import (
	"context"
	"fmt"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// TelegramNode sends messages and interacts with the Telegram Bot API.
// Type: "comm.telegram"
//
// Config fields:
//
//	"operation"  (string, required): "send_message" | "send_photo" | "get_updates"
//	"token"      (string, required): Bot API token
//	"chat_id"    (interface{}, required): chat ID (int64 or string username)
//	"text"       (string): message text (send_message, send_photo caption)
//	"photo_url"  (string): URL or local file path for photo (send_photo)
//	"parse_mode" (string): "HTML" (default) | "Markdown"
type TelegramNode struct{}

func (n *TelegramNode) Type() string { return "comm.telegram" }

func (n *TelegramNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token, _ := config["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("comm.telegram: token is required")
	}

	operation, _ := config["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("comm.telegram: operation is required")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("comm.telegram: create bot: %w", err)
	}

	parseMode := "HTML"
	if pm, ok := config["parse_mode"].(string); ok && pm != "" {
		parseMode = pm
	}

	chatID, err := resolveTelegramChatID(config["chat_id"])
	if err != nil && operation != "get_updates" {
		return nil, fmt.Errorf("comm.telegram: %w", err)
	}

	switch operation {
	case "send_message":
		text, _ := config["text"].(string)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = parseMode

		sent, err := bot.Send(msg)
		if err != nil {
			return nil, fmt.Errorf("comm.telegram: send_message: %w", err)
		}
		result := workflow.NewItem(map[string]interface{}{
			"message_id": sent.MessageID,
			"chat_id":    sent.Chat.ID,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil

	case "send_photo":
		photoURL, _ := config["photo_url"].(string)
		if photoURL == "" {
			return nil, fmt.Errorf("comm.telegram: photo_url is required for send_photo")
		}
		text, _ := config["text"].(string)

		var fileData tgbotapi.RequestFileData
		if _, err := os.Stat(photoURL); err == nil {
			// Local file.
			fileData = tgbotapi.FilePath(photoURL)
		} else {
			// Treat as URL.
			fileData = tgbotapi.FileURL(photoURL)
		}

		photo := tgbotapi.NewPhoto(chatID, fileData)
		photo.Caption = text
		photo.ParseMode = parseMode

		sent, err := bot.Send(photo)
		if err != nil {
			return nil, fmt.Errorf("comm.telegram: send_photo: %w", err)
		}
		result := workflow.NewItem(map[string]interface{}{
			"message_id": sent.MessageID,
			"chat_id":    sent.Chat.ID,
		})
		return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{result}}}, nil

	case "get_updates":
		updates, err := bot.GetUpdates(tgbotapi.UpdateConfig{
			Offset:  0,
			Limit:   100,
			Timeout: 0,
		})
		if err != nil {
			return nil, fmt.Errorf("comm.telegram: get_updates: %w", err)
		}
		items := make([]workflow.Item, 0, len(updates))
		for _, u := range updates {
			item := map[string]interface{}{
				"update_id": u.UpdateID,
			}
			if u.Message != nil {
				item["message_id"] = u.Message.MessageID
				item["chat_id"] = u.Message.Chat.ID
				item["text"] = u.Message.Text
				if u.Message.From != nil {
					item["from"] = u.Message.From.UserName
				}
			}
			items = append(items, workflow.NewItem(item))
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	default:
		return nil, fmt.Errorf("comm.telegram: unsupported operation %q", operation)
	}
}

// resolveTelegramChatID coerces the chat_id config value to int64.
func resolveTelegramChatID(v interface{}) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("chat_id is required")
	}
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		// Telegram accepts string usernames via chat_id only for certain methods;
		// this cast covers numeric string IDs passed as strings.
		var id int64
		if _, err := fmt.Sscanf(val, "%d", &id); err != nil {
			return 0, fmt.Errorf("chat_id %q is not a valid integer ID", val)
		}
		return id, nil
	}
	return 0, fmt.Errorf("chat_id must be an integer or string, got %T", v)
}
