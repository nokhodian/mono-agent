package comm

import (
	"context"
	"encoding/json"
	"fmt"

	slackgo "github.com/slack-go/slack"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// SlackNode interacts with the Slack API.
// Type: "comm.slack"
//
// Config fields:
//
//	"operation"  (string, required): "post_message" | "upload_file" | "list_channels" | "get_user"
//	"token"      (string, required): Bot User OAuth Token
//	"channel"    (string): channel ID or name (for post_message, upload_file)
//	"text"       (string): message text (for post_message)
//	"blocks"     ([]interface{}): Slack Block Kit JSON blocks (for post_message)
//	"file_path"  (string): local file path (for upload_file)
//	"filename"   (string): filename to display (for upload_file)
//	"user_id"    (string): Slack user ID (for get_user)
type SlackNode struct{}

func (n *SlackNode) Type() string { return "comm.slack" }

func (n *SlackNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	token, _ := config["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("comm.slack: token is required")
	}

	operation, _ := config["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("comm.slack: operation is required")
	}

	client := slackgo.New(token)

	var resultItem workflow.Item

	switch operation {
	case "post_message":
		channel, _ := config["channel"].(string)
		if channel == "" {
			return nil, fmt.Errorf("comm.slack: channel is required for post_message")
		}
		text, _ := config["text"].(string)

		opts := []slackgo.MsgOption{slackgo.MsgOptionText(text, false)}

		if blocksRaw, ok := config["blocks"].([]interface{}); ok && len(blocksRaw) > 0 {
			blocks, err := slackBlocksFromRaw(blocksRaw)
			if err != nil {
				return nil, fmt.Errorf("comm.slack: parse blocks: %w", err)
			}
			opts = append(opts, slackgo.MsgOptionBlocks(blocks...))
		}

		ch, ts, err := client.PostMessageContext(ctx, channel, opts...)
		if err != nil {
			return nil, fmt.Errorf("comm.slack: post_message: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{
			"channel":   ch,
			"timestamp": ts,
		})

	case "upload_file":
		channel, _ := config["channel"].(string)
		filePath, _ := config["file_path"].(string)
		if filePath == "" {
			return nil, fmt.Errorf("comm.slack: file_path is required for upload_file")
		}
		filename, _ := config["filename"].(string)
		if filename == "" {
			filename = filePath
		}

		params := slackgo.UploadFileParameters{
			Filename: filename,
			File:     filePath,
			Channel:  channel,
		}

		file, err := client.UploadFileContext(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("comm.slack: upload_file: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{
			"file_id":  file.ID,
			"filename": file.Title,
		})

	case "list_channels":
		channels, _, err := client.GetConversationsContext(ctx, &slackgo.GetConversationsParameters{
			Limit: 200,
		})
		if err != nil {
			return nil, fmt.Errorf("comm.slack: list_channels: %w", err)
		}
		items := make([]workflow.Item, 0, len(channels))
		for _, ch := range channels {
			items = append(items, workflow.NewItem(map[string]interface{}{
				"id":          ch.ID,
				"name":        ch.Name,
				"is_private":  ch.IsPrivate,
				"num_members": ch.NumMembers,
			}))
		}
		return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil

	case "get_user":
		userID, _ := config["user_id"].(string)
		if userID == "" {
			return nil, fmt.Errorf("comm.slack: user_id is required for get_user")
		}
		user, err := client.GetUserInfoContext(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("comm.slack: get_user: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{
			"id":        user.ID,
			"name":      user.Name,
			"real_name": user.RealName,
			"email":     user.Profile.Email,
			"is_bot":    user.IsBot,
			"is_admin":  user.IsAdmin,
			"timezone":  user.TZ,
		})

	default:
		return nil, fmt.Errorf("comm.slack: unsupported operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{resultItem}}}, nil
}

// slackBlocksFromRaw converts raw []interface{} block data into slack.Block objects
// by round-tripping through JSON.
func slackBlocksFromRaw(raw []interface{}) ([]slackgo.Block, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal blocks: %w", err)
	}
	var blocks slackgo.Blocks
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("unmarshal blocks: %w", err)
	}
	return blocks.BlockSet, nil
}
