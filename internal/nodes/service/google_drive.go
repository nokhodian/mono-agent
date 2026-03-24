package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// GoogleDriveNode implements the service.google_drive node type.
type GoogleDriveNode struct{}

func (n *GoogleDriveNode) Type() string { return "service.google_drive" }

const (
	driveBaseURL   = "https://www.googleapis.com/drive/v3"
	driveUploadURL = "https://www.googleapis.com/upload/drive/v3"
)

func (n *GoogleDriveNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	accessToken := strVal(config, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("google_drive: access_token is required")
	}
	operation := strVal(config, "operation")
	pageSize := intVal(config, "page_size")
	if pageSize == 0 {
		pageSize = 10
	}

	var items []workflow.Item

	switch operation {
	case "list_files":
		url := fmt.Sprintf("%s/files?pageSize=%d&fields=files(id,name,mimeType,size,createdTime,modifiedTime,parents)", driveBaseURL, pageSize)
		if q := strVal(config, "query"); q != "" {
			url += "&q=" + driveURLEncode(q)
		}
		data, err := driveRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("google_drive list_files: %w", err)
		}
		files, _ := data["files"].([]interface{})
		items = make([]workflow.Item, 0, len(files))
		for _, f := range files {
			if fm, ok := f.(map[string]interface{}); ok {
				items = append(items, workflow.NewItem(fm))
			}
		}

	case "get_file":
		fileID := strVal(config, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("google_drive: file_id is required for get_file")
		}
		url := driveBaseURL + "/files/" + fileID + "?fields=id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink,webContentLink"
		data, err := driveRequest(ctx, "GET", url, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("google_drive get_file: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "upload_file":
		filePath := strVal(config, "file_path")
		if filePath == "" {
			return nil, fmt.Errorf("google_drive: file_path is required for upload_file")
		}
		fileName := strVal(config, "file_name")
		if fileName == "" {
			fileName = filepath.Base(filePath)
		}
		mimeType := strVal(config, "mime_type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		parentFolderID := strVal(config, "parent_folder_id")
		data, err := driveUploadFile(ctx, accessToken, filePath, fileName, mimeType, parentFolderID)
		if err != nil {
			return nil, fmt.Errorf("google_drive upload_file: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "download_file":
		fileID := strVal(config, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("google_drive: file_id is required for download_file")
		}
		url := driveBaseURL + "/files/" + fileID + "?alt=media"
		content, err := driveDownload(ctx, url, accessToken)
		if err != nil {
			return nil, fmt.Errorf("google_drive download_file: %w", err)
		}
		item := workflow.Item{
			JSON:   map[string]interface{}{"file_id": fileID},
			Binary: map[string][]byte{"data": content},
		}
		items = []workflow.Item{item}

	case "create_folder":
		folderName := strVal(config, "file_name")
		if folderName == "" {
			return nil, fmt.Errorf("google_drive: file_name is required for create_folder")
		}
		meta := map[string]interface{}{
			"name":     folderName,
			"mimeType": "application/vnd.google-apps.folder",
		}
		if parentFolderID := strVal(config, "parent_folder_id"); parentFolderID != "" {
			meta["parents"] = []string{parentFolderID}
		}
		data, err := driveRequest(ctx, "POST", driveBaseURL+"/files", accessToken, meta)
		if err != nil {
			return nil, fmt.Errorf("google_drive create_folder: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	case "delete_file":
		fileID := strVal(config, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("google_drive: file_id is required for delete_file")
		}
		_, err := driveRequest(ctx, "DELETE", driveBaseURL+"/files/"+fileID, accessToken, nil)
		if err != nil {
			return nil, fmt.Errorf("google_drive delete_file: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(map[string]interface{}{
			"file_id": fileID,
			"deleted": true,
		})}

	case "share_file":
		fileID := strVal(config, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("google_drive: file_id is required for share_file")
		}
		// Share publicly (anyone with link can view)
		permission := map[string]interface{}{
			"role": "reader",
			"type": "anyone",
		}
		url := driveBaseURL + "/files/" + fileID + "/permissions"
		data, err := driveRequest(ctx, "POST", url, accessToken, permission)
		if err != nil {
			return nil, fmt.Errorf("google_drive share_file: %w", err)
		}
		items = []workflow.Item{workflow.NewItem(data)}

	default:
		return nil, fmt.Errorf("google_drive: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

// driveRequest makes an authenticated request to the Google Drive API.
func driveRequest(ctx context.Context, method, url, accessToken string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("google_drive: marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("google_drive: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google_drive: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google_drive HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("google_drive: parsing JSON: %w", err)
	}
	return result, nil
}

// driveDownload fetches the raw bytes of a Drive file.
func driveDownload(ctx context.Context, url, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("google_drive: creating download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive: downloading file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google_drive HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// driveUploadFile uploads a local file to Google Drive using multipart upload.
func driveUploadFile(ctx context.Context, accessToken, filePath, fileName, mimeType, parentFolderID string) (map[string]interface{}, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	meta := map[string]interface{}{
		"name": fileName,
	}
	if parentFolderID != "" {
		meta["parents"] = []string{parentFolderID}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshaling file metadata: %w", err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	// Metadata part
	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := w.CreatePart(metaHeader)
	if err != nil {
		return nil, fmt.Errorf("creating metadata part: %w", err)
	}
	if _, err := metaPart.Write(metaBytes); err != nil {
		return nil, fmt.Errorf("writing metadata part: %w", err)
	}

	// File content part
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Type", mimeType)
	filePart, err := w.CreatePart(fileHeader)
	if err != nil {
		return nil, fmt.Errorf("creating file part: %w", err)
	}
	if _, err := filePart.Write(fileContent); err != nil {
		return nil, fmt.Errorf("writing file part: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	uploadURL := driveUploadURL + "/files?uploadType=multipart"
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &body)
	if err != nil {
		return nil, fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+w.Boundary())
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uploading file: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upload response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("drive upload HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing upload response: %w", err)
	}
	return result, nil
}

// driveURLEncode encodes a string for use in a URL query parameter.
func driveURLEncode(s string) string {
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ':
			out = append(out, '+')
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~':
			out = append(out, c)
		default:
			out = append(out, '%', hexChar(c>>4), hexChar(c&0xf))
		}
	}
	return string(out)
}
