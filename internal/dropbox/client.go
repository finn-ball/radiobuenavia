package dropbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const chunkSize = 4 * 1024 * 1024

const (
	apiHost     = "https://api.dropboxapi.com"
	contentHost = "https://content.dropboxapi.com"
)

type Client struct {
	appKey       string
	appSecret    string
	refreshToken string
	accessToken  string
	client       *http.Client
}

type FileMetadata struct {
	Name           string
	PathLower      string
	ClientModified time.Time
}

func NewClient(appKey, appSecret, refreshToken string) (*Client, error) {
	c := &Client{
		appKey:       appKey,
		appSecret:    appSecret,
		refreshToken: refreshToken,
		client:       &http.Client{Timeout: 60 * time.Second},
	}
	if err := c.refreshAccessToken(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) refreshAccessToken() error {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequest("POST", apiHost+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.appKey, c.appSecret)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dropbox token request failed: %s", strings.TrimSpace(string(body)))
	}

	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if payload.AccessToken == "" {
		return errors.New("dropbox token response missing access_token")
	}
	c.accessToken = payload.AccessToken
	return nil
}

func (c *Client) ListFiles(path string) ([]FileMetadata, error) {
	type listFolderResponse struct {
		Entries []struct {
			Tag            string `json:".tag"`
			Name           string `json:"name"`
			PathLower      string `json:"path_lower"`
			ClientModified string `json:"client_modified"`
		} `json:"entries"`
		Cursor  string `json:"cursor"`
		HasMore bool   `json:"has_more"`
	}

	body := map[string]any{
		"path": path,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := c.doAPIRequest("/2/files/list_folder", payload)
	if err != nil {
		return nil, err
	}

	var out listFolderResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, err
	}
	files := extractFiles(out.Entries)

	for out.HasMore {
		nextPayload, err := json.Marshal(map[string]string{"cursor": out.Cursor})
		if err != nil {
			return nil, err
		}
		resp, err := c.doAPIRequest("/2/files/list_folder/continue", nextPayload)
		if err != nil {
			return nil, err
		}
		out = listFolderResponse{}
		if err := json.Unmarshal(resp, &out); err != nil {
			return nil, err
		}
		files = append(files, extractFiles(out.Entries)...)
	}

	return files, nil
}

func extractFiles(entries []struct {
	Tag            string `json:".tag"`
	Name           string `json:"name"`
	PathLower      string `json:"path_lower"`
	ClientModified string `json:"client_modified"`
}) []FileMetadata {
	files := []FileMetadata{}
	for _, entry := range entries {
		if entry.Tag != "file" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, entry.ClientModified)
		if err != nil {
			parsed = time.Time{}
		}
		files = append(files, FileMetadata{
			Name:           entry.Name,
			PathLower:      entry.PathLower,
			ClientModified: parsed,
		})
	}
	return files
}

func (c *Client) RenameFile(file FileMetadata) string {
	ext := filepath.Ext(file.Name)
	base := strings.TrimSuffix(file.Name, ext)
	stamp := file.ClientModified.Format("02.01.06")
	return fmt.Sprintf("%s - Radio Buena Vida %s%s", base, stamp, ext)
}

func (c *Client) DownloadFile(localPath, dropboxPath string) error {
	arg, err := json.Marshal(map[string]string{"path": dropboxPath})
	if err != nil {
		return err
	}

	resp, err := c.doContentRequest("/2/files/download", arg, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dropbox download failed: %s", strings.TrimSpace(string(body)))
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func (c *Client) UploadFileSoundcloud(localPath, name, soundcloudPath string) error {
	remotePath := c.remotePath(soundcloudPath, name)
	return c.UploadFile(localPath, remotePath)
}

func (c *Client) CopyToArchive(name, soundcloudPath, archivePath string) error {
	fromPath := c.remotePath(soundcloudPath, name)
	toPath := c.remotePath(archivePath, name)
	payload, err := json.Marshal(map[string]string{
		"from_path": fromPath,
		"to_path":   toPath,
	})
	if err != nil {
		return err
	}
	_, err = c.doAPIRequest("/2/files/copy_v2", payload)
	return err
}

func (c *Client) UploadFile(localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	fileSize := info.Size()

	if fileSize <= chunkSize {
		buf, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		arg := map[string]any{
			"path":       remotePath,
			"mode":       "add",
			"autorename": false,
			"mute":       false,
		}
		payload, err := json.Marshal(arg)
		if err != nil {
			return err
		}
		resp, err := c.doContentRequest("/2/files/upload", payload, buf)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("dropbox upload failed: %s", strings.TrimSpace(string(body)))
		}
		return nil
	}

	startArg, err := json.Marshal(map[string]bool{"close": false})
	if err != nil {
		return err
	}
	startChunk := make([]byte, chunkSize)
	n, err := io.ReadFull(file, startChunk)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	startChunk = startChunk[:n]
	resp, err := c.doContentRequest("/2/files/upload_session/start", startArg, startChunk)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dropbox upload session start failed: %s", strings.TrimSpace(string(body)))
	}
	var startResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startResp); err != nil {
		return err
	}
	if startResp.SessionID == "" {
		return errors.New("dropbox upload session start missing session_id")
	}

	offset := int64(n)
	for offset < fileSize {
		remaining := fileSize - offset
		chunkSize64 := minInt64(remaining, int64(chunkSize))
		chunk := make([]byte, int(chunkSize64))
		read, err := io.ReadFull(file, chunk)
		if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		chunk = chunk[:read]

		if offset+int64(read) >= fileSize {
			finishArg, err := json.Marshal(map[string]any{
				"cursor": map[string]any{
					"session_id": startResp.SessionID,
					"offset":     offset,
				},
				"commit": map[string]any{
					"path":       remotePath,
					"mode":       "add",
					"autorename": false,
					"mute":       false,
				},
			})
			if err != nil {
				return err
			}
			finishResp, err := c.doContentRequest("/2/files/upload_session/finish", finishArg, chunk)
			if err != nil {
				return err
			}
			defer finishResp.Body.Close()
			if finishResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(finishResp.Body)
				return fmt.Errorf("dropbox upload session finish failed: %s", strings.TrimSpace(string(body)))
			}
			return nil
		}

		appendArg, err := json.Marshal(map[string]any{
			"cursor": map[string]any{
				"session_id": startResp.SessionID,
				"offset":     offset,
			},
			"close": false,
		})
		if err != nil {
			return err
		}
		appendResp, err := c.doContentRequest("/2/files/upload_session/append_v2", appendArg, chunk)
		if err != nil {
			return err
		}
		appendResp.Body.Close()
		if appendResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(appendResp.Body)
			return fmt.Errorf("dropbox upload session append failed: %s", strings.TrimSpace(string(body)))
		}

		offset += int64(read)
	}

	return nil
}

func (c *Client) ListFilesToProcess(preprocessPath, archivePath string) ([]FileMetadata, error) {
	preproc, err := c.ListFiles(preprocessPath)
	if err != nil {
		return nil, err
	}
	archive, err := c.ListFiles(archivePath)
	if err != nil {
		return nil, err
	}
	archiveNames := make(map[string]struct{}, len(archive))
	for _, file := range archive {
		archiveNames[file.Name] = struct{}{}
	}
	result := make([]FileMetadata, 0, len(preproc))
	for _, file := range preproc {
		if _, exists := archiveNames[c.RenameFile(file)]; !exists {
			result = append(result, file)
		}
	}
	return result, nil
}

func (c *Client) remotePath(base, name string) string {
	if strings.HasSuffix(base, "/") {
		return base + name
	}
	return base + "/" + name
}

func (c *Client) doAPIRequest(endpoint string, payload []byte) ([]byte, error) {
	resp, err := c.doAPIRequestWithJSONBody(endpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dropbox api error: %s", strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) doAPIRequestNoBody(endpoint string) ([]byte, error) {
	req, err := http.NewRequest("POST", apiHost+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dropbox api error: %s", strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) doContentRequest(endpoint string, arg []byte, body []byte) (*http.Response, error) {
	if body == nil {
		body = []byte{}
	}
	req, err := http.NewRequest("POST", contentHost+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if arg != nil {
		req.Header.Set("Dropbox-API-Arg", string(arg))
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	return c.client.Do(req)
}

func (c *Client) doAPIRequestWithJSONBody(endpoint string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", apiHost+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", mime.TypeByExtension(".json"))
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.client.Do(req)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
