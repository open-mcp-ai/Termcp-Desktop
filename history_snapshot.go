package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/open-mcp-ai/termcp/gui/internal/model"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

func (a *App) ArchiveShell(shellID string) (err error) {
	done := beginInterface("ArchiveShell", "shell_id", shellID)
	defer func() { done(err) }()
	shellID = strings.TrimSpace(shellID)
	if shellID == "" {
		return errors.New("shell id is required")
	}
	if _, err := a.API(model.APIRequest{
		Method: http.MethodPost,
		Path:   "/api/shells/" + url.PathEscape(shellID) + "/archive",
	}); err != nil {
		return err
	}
	snapshot, err := a.GetSnapshot()
	if err != nil {
		return err
	}
	for _, history := range snapshot.History {
		for _, shell := range history.Shells {
			if shell.ID == shellID {
				return nil
			}
		}
	}
	return errors.New("Core accepted the archive request but the shell record is not readable")
}

// persistedShellsBySession reads Core's retained shell manifests. Core's live
// session endpoint intentionally omits archived child shells; the manifests are
// where those records remain readable while the parent session is still up.
func (a *App) persistedShellsBySession() map[string][]model.Shell {
	if a.historyStore == nil {
		return nil
	}
	records, err := a.historyStore.LoadSessions()
	if err != nil {
		return nil
	}
	result := make(map[string][]model.Shell, len(records))
	for _, record := range records {
		shells := make([]model.Shell, 0, len(record.Shells))
		for _, shell := range record.Shells {
			if shell.Status == api.SessionRunning {
				continue
			}
			shells = append(shells, model.Shell{
				ID:         shell.ID,
				Name:       shell.Name,
				Status:     string(shell.Status),
				Mode:       string(shell.Mode),
				ExitCode:   shell.ExitCode,
				ExitReason: shell.ExitReason,
				Archived:   true,
			})
		}
		if len(shells) > 0 {
			result[record.ID] = shells
		}
	}
	return result
}

func archivedShells(live []model.Shell, persisted []model.Shell) []model.Shell {
	if len(persisted) == 0 {
		return nil
	}
	liveIDs := make(map[string]bool, len(live))
	for _, shell := range live {
		liveIDs[shell.ID] = true
	}
	archived := make([]model.Shell, 0, len(persisted))
	for _, shell := range persisted {
		if strings.TrimSpace(shell.ID) == "" || liveIDs[shell.ID] {
			continue
		}
		shell.Archived = true
		archived = append(archived, shell)
	}
	return archived
}
