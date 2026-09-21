// Package bridge proxies Core HTTP resources for the Wails frontend and hosts
// the file dialogs used by upload/download actions.
package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/logging"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxResponse = 64 << 20

var requestSequence atomic.Uint64

type requestLog struct {
	id      uint64
	method  string
	path    string
	started time.Time
}

func beginRequest(method, rawPath string) requestLog {
	request := requestLog{id: requestSequence.Add(1), method: method, path: safeLogPath(rawPath), started: time.Now()}
	slog.Debug("Core API request started", "request_id", request.id, "method", request.method, "path", request.path)
	return request
}

func (request requestLog) finish(status int, bytes int64, err error) {
	fields := []any{
		"request_id", request.id,
		"method", request.method,
		"path", request.path,
		"status", status,
		"response_bytes", bytes,
		"duration_ms", time.Since(request.started).Milliseconds(),
	}
	if err != nil {
		slog.Error("Core API request failed", append(fields, "error", logging.ErrorText(err))...)
		return
	}
	slog.Debug("Core API request completed", fields...)
}

type Client struct {
	core     *corepkg.Service
	client   *http.Client
	transfer *http.Client
}

func New(core *corepkg.Service) *Client {
	return &Client{
		core:     core,
		client:   &http.Client{Timeout: 30 * time.Second},
		transfer: &http.Client{},
	}
}

func (c *Client) BaseURL() string { return c.core.BaseURL() }

// GetJSON performs a GET against the Core API and decodes a JSON response.
func (c *Client) GetJSON(path string, target any) error {
	return c.DoJSON(http.MethodGet, path, nil, target)
}

// DoJSON issues a JSON request against the Core API and decodes the response.
func (c *Client) DoJSON(method, path string, body any, target any) (err error) {
	requestLog := beginRequest(method, path)
	status := 0
	responseBytes := int64(0)
	defer func() { requestLog.finish(status, responseBytes, err) }()
	var reader io.Reader
	if body != nil {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
		reader = bytes.NewReader(data)
	}
	req, requestErr := http.NewRequest(method, c.core.BaseURL()+path, reader)
	if requestErr != nil {
		return requestErr
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, requestErr := c.client.Do(req)
	if requestErr != nil {
		return fmt.Errorf("Core 请求失败: %w", requestErr)
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	if resp.ContentLength > 0 {
		responseBytes = resp.ContentLength
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		responseBytes = int64(len(msg))
		return fmt.Errorf("Core 返回 %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if target != nil && resp.StatusCode != http.StatusNoContent {
		if decodeErr := json.NewDecoder(resp.Body).Decode(target); decodeErr != nil {
			return fmt.Errorf("解析 Core 响应: %w", decodeErr)
		}
	}
	return nil
}

func (c *Client) WebSocketURL() string {
	return "ws" + strings.TrimPrefix(c.core.BaseURL(), "http") + "/api/ui/ws"
}

func (c *Client) API(input model.APIRequest) (result model.APIResponse, err error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	requestLog := beginRequest(method, input.Path)
	status := 0
	responseBytes := int64(0)
	defer func() { requestLog.finish(status, responseBytes, err) }()
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return model.APIResponse{}, fmt.Errorf("不支持的请求方法 %q", method)
	}
	apiPath, validationErr := validatePath(input.Path)
	if validationErr != nil {
		return model.APIResponse{}, validationErr
	}
	var body io.Reader
	if input.Body != "" {
		body = strings.NewReader(input.Body)
	}
	req, requestErr := http.NewRequest(method, c.core.BaseURL()+apiPath, body)
	if requestErr != nil {
		return model.APIResponse{}, requestErr
	}
	if input.Body != "" {
		contentType := strings.TrimSpace(input.ContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	resp, requestErr := c.client.Do(req)
	if requestErr != nil {
		return model.APIResponse{}, fmt.Errorf("Core 请求失败: %w", requestErr)
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	responseBytes = int64(len(data))
	if readErr != nil {
		return model.APIResponse{}, readErr
	}
	if len(data) > maxResponse {
		return model.APIResponse{}, errors.New("Core 响应超过 64 MiB，请使用下载功能")
	}
	result = model.APIResponse{
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

func (c *Client) ChooseAndUploadFile(ctx context.Context, sessionID, remoteDirectory string) (result model.UploadResult, err error) {
	localPath, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:                "选择要上传的文件",
		ShowHiddenFiles:      true,
		ResolvesAliases:      true,
		CanCreateDirectories: false,
	})
	if err != nil {
		return model.UploadResult{}, err
	}
	if localPath == "" {
		return model.UploadResult{Cancelled: true}, nil
	}
	file, err := os.Open(localPath)
	if err != nil {
		return model.UploadResult{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return model.UploadResult{}, err
	}
	remotePath := strings.TrimSpace(remoteDirectory)
	if remotePath == "" {
		return model.UploadResult{}, errors.New("远程目录不能为空")
	}
	remotePath = path.Join(remotePath, filepath.Base(localPath))
	endpoint := "/api/sessions/" + url.PathEscape(sessionID) + "/files/upload?path=" + url.QueryEscape(remotePath)
	requestLog := beginRequest(http.MethodPost, endpoint)
	status := 0
	responseBytes := int64(0)
	defer func() { requestLog.finish(status, responseBytes, err) }()
	req, err := http.NewRequest(http.MethodPost, c.core.BaseURL()+endpoint, file)
	if err != nil {
		return model.UploadResult{}, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.transfer.Do(req)
	if err != nil {
		return model.UploadResult{}, err
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	responseBytes = int64(len(responseBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.UploadResult{}, fmt.Errorf("上传失败: %s", strings.TrimSpace(string(responseBody)))
	}
	return model.UploadResult{LocalName: filepath.Base(localPath), RemotePath: remotePath, Bytes: info.Size()}, nil
}

func (c *Client) SaveAPIResource(ctx context.Context, apiPath, suggestedName string) (destination string, err error) {
	validated, err := validatePath(apiPath)
	if err != nil {
		return "", err
	}
	requestLog := beginRequest(http.MethodGet, validated)
	status := 0
	responseBytes := int64(0)
	defer func() { requestLog.finish(status, responseBytes, err) }()
	resp, err := c.transfer.Get(c.core.BaseURL() + validated)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		responseBytes = int64(len(data))
		return "", fmt.Errorf("下载失败: %s", strings.TrimSpace(string(data)))
	}
	destination, err = runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
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
	written, copyErr := io.Copy(file, resp.Body)
	responseBytes = written
	if copyErr != nil {
		_ = file.Close()
		return "", copyErr
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return destination, nil
}

func validatePath(raw string) (string, error) {
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

func safeLogPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "invalid"
	}
	if parsed.RawQuery == "" {
		return parsed.Path
	}
	keys := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return parsed.Path + "?" + strings.Join(keys, "&")
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
