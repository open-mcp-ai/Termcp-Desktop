package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxBridgeResponse = 64 << 20

type APIRequest struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Body        string `json:"body,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type APIResponse struct {
	Status      int               `json:"status"`
	ContentType string            `json:"content_type,omitempty"`
	Body        string            `json:"body,omitempty"`
	Base64      string            `json:"base64,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type UploadResult struct {
	LocalName  string `json:"local_name"`
	RemotePath string `json:"remote_path"`
	Bytes      int64  `json:"bytes"`
	Cancelled  bool   `json:"cancelled"`
}

func (a *App) API(input APIRequest) (APIResponse, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return APIResponse{}, fmt.Errorf("不支持的请求方法 %q", method)
	}
	apiPath, err := validateAPIPath(input.Path)
	if err != nil {
		return APIResponse{}, err
	}
	var body io.Reader
	if input.Body != "" {
		body = strings.NewReader(input.Body)
	}
	req, err := http.NewRequest(method, a.core.BaseURL()+apiPath, body)
	if err != nil {
		return APIResponse{}, err
	}
	if input.Body != "" {
		contentType := strings.TrimSpace(input.ContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return APIResponse{}, fmt.Errorf("Core 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBridgeResponse+1))
	if err != nil {
		return APIResponse{}, err
	}
	if len(data) > maxBridgeResponse {
		return APIResponse{}, errors.New("Core 响应超过 64 MiB，请使用下载功能")
	}
	result := APIResponse{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     selectedHeaders(resp.Header),
	}
	mediaType, _, _ := mime.ParseMediaType(result.ContentType)
	if strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "" {
		result.Body = string(data)
	} else if len(data) > 0 {
		result.Base64 = base64.StdEncoding.EncodeToString(data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = resp.Status
		}
		return result, fmt.Errorf("Core 返回 %s: %s", resp.Status, message)
	}
	return result, nil
}

func (a *App) ChooseAndUploadFile(sessionID, remoteDirectory string) (UploadResult, error) {
	localPath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "选择要上传的文件",
		ShowHiddenFiles:      true,
		ResolvesAliases:      true,
		CanCreateDirectories: false,
	})
	if err != nil {
		return UploadResult{}, err
	}
	if localPath == "" {
		return UploadResult{Cancelled: true}, nil
	}
	file, err := os.Open(localPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return UploadResult{}, err
	}
	remotePath := strings.TrimSpace(remoteDirectory)
	if remotePath == "" {
		return UploadResult{}, errors.New("远程目录不能为空")
	}
	remotePath = path.Join(remotePath, filepath.Base(localPath))
	endpoint := "/api/sessions/" + url.PathEscape(sessionID) + "/files/upload?path=" + url.QueryEscape(remotePath)
	req, err := http.NewRequest(http.MethodPost, a.core.BaseURL()+endpoint, file)
	if err != nil {
		return UploadResult{}, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := a.transfer.Do(req)
	if err != nil {
		return UploadResult{}, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return UploadResult{}, fmt.Errorf("上传失败: %s", strings.TrimSpace(string(responseBody)))
	}
	return UploadResult{LocalName: filepath.Base(localPath), RemotePath: remotePath, Bytes: info.Size()}, nil
}

func (a *App) SaveAPIResource(apiPath, suggestedName string) (string, error) {
	validated, err := validateAPIPath(apiPath)
	if err != nil {
		return "", err
	}
	resp, err := a.transfer.Get(a.core.BaseURL() + validated)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("下载失败: %s", strings.TrimSpace(string(data)))
	}
	destination, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:                "保存文件",
		DefaultFilename:      filepath.Base(suggestedName),
		CanCreateDirectories: true,
	})
	if err != nil || destination == "" {
		return destination, err
	}
	file, err := os.Create(destination)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return destination, nil
}

func (a *App) CoreWebSocketURL() string {
	return "ws" + strings.TrimPrefix(a.core.BaseURL(), "http") + "/api/ui/ws"
}

func validateAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/api/") {
		return "", errors.New("仅允许访问当前 Core 的 /api/ 路径")
	}
	return parsed.RequestURI(), nil
}

func selectedHeaders(header http.Header) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"Content-Disposition", "Content-Length", "Accept-Ranges", "Content-Range"} {
		if value := header.Get(key); value != "" {
			result[key] = value
		}
	}
	return result
}
