package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
	"github.com/open-mcp-ai/termcp/internal/configcrypt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxConnectionBatch = 16 << 20

type batchImportResponse struct {
	Imported int `json:"imported"`
	Renamed  []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"renamed,omitempty"`
}

// ExportConnections asks Core for the selected profiles and saves the response
// through the native dialog. Core decides the plain/encrypted format; the
// desktop only chooses a filename and writes the bytes it returned.
func (a *App) ExportConnections(names []string, password string) (result model.ConnectionExportResult, err error) {
	done := beginInterface("ExportConnections", "requested_hosts", len(names), "encrypted", password != "")
	defer func() { done(err) }()

	response, selected, err := a.connectionExport(names, password)
	if err != nil {
		return result, err
	}
	data, err := apiResponseBytes(response)
	if err != nil {
		return result, err
	}
	filename := "termcp-connections.toml"
	if password != "" {
		filename = "termcp-connections.enc"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:                "Export host configurations",
		DefaultFilename:      filename,
		CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return model.ConnectionExportResult{Cancelled: err == nil, Count: len(selected)}, err
	}
	extension := filepath.Ext(filename)
	if !strings.HasSuffix(strings.ToLower(path), extension) {
		path += extension
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return model.ConnectionExportResult{}, err
	}
	return model.ConnectionExportResult{Path: path, Count: len(selected)}, nil
}

func (a *App) connectionExport(names []string, password string) (model.APIResponse, []string, error) {
	selected := selectedConnectionNames(names)
	if len(selected) == 0 {
		return model.APIResponse{}, nil, errors.New("select at least one remote host configuration")
	}
	path := "/api/connections/batch?names=" + url.QueryEscape(strings.Join(selected, ","))
	response, err := a.API(model.APIRequest{
		Method:   http.MethodGet,
		Path:     path,
		Password: password,
	})
	return response, selected, err
}

func selectedConnectionNames(names []string) []string {
	selected := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || strings.EqualFold(name, "internal") || seen[name] {
			continue
		}
		seen[name] = true
		selected = append(selected, name)
	}
	return selected
}

func apiResponseBytes(response model.APIResponse) ([]byte, error) {
	if response.Base64 != "" {
		data, err := base64.StdEncoding.DecodeString(response.Base64)
		if err != nil {
			return nil, fmt.Errorf("decode Core export: %w", err)
		}
		return data, nil
	}
	if response.Body == "" {
		return nil, errors.New("Core returned an empty connection export")
	}
	return []byte(response.Body), nil
}

// ImportConnections reads a normal or password-protected batch through a native
// file chooser. Parsing and import happen in importConnectionsDocument so the
// same behavior is testable without a file dialog.
func (a *App) ImportConnections(password string) (result model.ConnectionImportResult, err error) {
	done := beginInterface("ImportConnections", "encrypted", password != "")
	defer func() { done(err) }()

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import host configurations",
		Filters: []runtime.FileFilter{{
			DisplayName: "Termcp connection exports",
			Pattern:     "*.toml;*.enc",
		}},
		ShowHiddenFiles:      true,
		ResolvesAliases:      true,
		CanCreateDirectories: false,
	})
	if err != nil || path == "" {
		return model.ConnectionImportResult{Cancelled: err == nil}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if info.Size() > maxConnectionBatch {
		return result, fmt.Errorf("import file too large (max %d bytes)", maxConnectionBatch)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	return a.importConnectionsDocument(filepath.Base(path), data, password)
}

func (a *App) importConnectionsDocument(fileName string, data []byte, password string) (model.ConnectionImportResult, error) {
	result := model.ConnectionImportResult{FileName: fileName, Hosts: []model.ConnectionImportHost{}}
	if len(data) == 0 {
		return result, errors.New("the selected connection export is empty")
	}
	if len(data) > maxConnectionBatch {
		return result, fmt.Errorf("import file too large (max %d bytes)", maxConnectionBatch)
	}

	// The normal path preserves Core's all-at-once validation and rename
	// semantics. A mixed-validity document is retried one host at a time below so
	// one bad entry cannot hide the entries that could be imported.
	response, bulkErr := a.importConnectionBatch(data, password)
	if bulkErr == nil {
		profiles, err := connectionProfiles(data, password)
		if err != nil {
			return result, err
		}
		return successfulConnectionImport(result, profiles, response.Body)
	}
	if response.Status != http.StatusBadRequest {
		return result, bulkErr
	}
	profiles, parseErr := connectionProfiles(data, password)
	if parseErr != nil {
		return result, parseErr
	}
	if len(profiles) == 0 {
		return result, bulkErr
	}

	result.Hosts = make([]model.ConnectionImportHost, 0, len(profiles))
	for index, profile := range profiles {
		name, _ := profile["name"].(string)
		if name == "" {
			name = fmt.Sprintf("Host %d", index+1)
		}
		document, err := singleConnectionDocument(profile)
		if err != nil {
			result.Hosts = append(result.Hosts, model.ConnectionImportHost{Name: name, Status: "failed", Error: err.Error()})
			result.Failed++
			continue
		}
		response, err := a.importConnectionBatch(document, password)
		if err != nil {
			result.Hosts = append(result.Hosts, model.ConnectionImportHost{Name: name, Status: "failed", Error: err.Error()})
			result.Failed++
			continue
		}
		host := importedConnectionHost(name, response.Body)
		result.Hosts = append(result.Hosts, host)
		if host.Status == "renamed" {
			result.Renamed++
		}
		result.Imported++
	}
	return result, nil
}

func (a *App) importConnectionBatch(data []byte, password string) (model.APIResponse, error) {
	return a.API(model.APIRequest{
		Method:      http.MethodPost,
		Path:        "/api/connections/batch",
		Body:        string(data),
		ContentType: "application/octet-stream",
		Password:    password,
	})
}

func successfulConnectionImport(result model.ConnectionImportResult, profiles []map[string]any, body string) (model.ConnectionImportResult, error) {
	var response batchImportResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return result, fmt.Errorf("decode Core import response: %w", err)
	}
	renamed := make(map[string]string, len(response.Renamed))
	for _, item := range response.Renamed {
		renamed[item.From] = item.To
	}
	result.Imported = response.Imported
	result.Hosts = make([]model.ConnectionImportHost, 0, len(profiles))
	for index, profile := range profiles {
		name, _ := profile["name"].(string)
		if name == "" {
			name = fmt.Sprintf("Host %d", index+1)
		}
		host := model.ConnectionImportHost{Name: name, Status: "imported"}
		if final := renamed[name]; final != "" {
			host.FinalName = final
			host.Status = "renamed"
			result.Renamed++
		}
		result.Hosts = append(result.Hosts, host)
	}
	return result, nil
}

func importedConnectionHost(name, body string) model.ConnectionImportHost {
	var response batchImportResponse
	host := model.ConnectionImportHost{Name: name, Status: "imported"}
	if json.Unmarshal([]byte(body), &response) == nil {
		for _, item := range response.Renamed {
			if item.From == name {
				host.FinalName = item.To
				host.Status = "renamed"
				break
			}
		}
	}
	return host
}

func connectionProfiles(data []byte, password string) ([]map[string]any, error) {
	plain := data
	if configcrypt.IsEncrypted(data) {
		var err error
		plain, err = configcrypt.Decrypt(data, password)
		if err != nil {
			if errors.Is(err, configcrypt.ErrWrongPassword) {
				return nil, errors.New("wrong password, or the file has been altered")
			}
			return nil, err
		}
	}
	var top map[string]any
	if err := toml.Unmarshal(plain, &top); err != nil {
		return nil, fmt.Errorf("import toml: %w", err)
	}
	if len(top) != 1 {
		return nil, errors.New("import toml contains unknown top-level fields")
	}
	if _, ok := top["connections"]; !ok {
		return nil, errors.New("import toml must contain connections")
	}
	var document struct {
		Connections []map[string]any `toml:"connections"`
	}
	if err := toml.Unmarshal(plain, &document); err != nil {
		return nil, fmt.Errorf("import toml: %w", err)
	}
	if len(document.Connections) == 0 {
		return nil, errors.New("import requires at least one connection")
	}
	return document.Connections, nil
}

func singleConnectionDocument(profile map[string]any) ([]byte, error) {
	document := struct {
		Connections []map[string]any `toml:"connections"`
	}{Connections: []map[string]any{profile}}
	return toml.Marshal(document)
}
