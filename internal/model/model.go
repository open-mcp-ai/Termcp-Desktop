// Package model contains the data transfer objects exchanged between the
// desktop shell, its internal services and the frontend bindings.
package model

type CoreStatus struct {
	Running bool   `json:"running"`
	Managed bool   `json:"managed"`
	Address string `json:"address"`
	Version string `json:"version,omitempty"`
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
}

type ServiceStatus struct {
	Supported   bool       `json:"supported"`
	Platform    string     `json:"platform"`
	Installed   bool       `json:"installed"`
	Running     bool       `json:"running"`
	Autostart   bool       `json:"autostart"`
	PID         int        `json:"pid,omitempty"`
	Label       string     `json:"label"`
	Definition  string     `json:"definition,omitempty"`
	LogPath     string     `json:"log_path,omitempty"`
	Executable  string     `json:"executable,omitempty"`
	Description string     `json:"description,omitempty"`
	DataDir     string     `json:"data_dir"`
	Core        CoreStatus `json:"core"`
}

type Connection struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Description     string `json:"description,omitempty"`
	Host            string `json:"host,omitempty"`
	User            string `json:"user,omitempty"`
	Port            int    `json:"port,omitempty"`
	DefaultApproval bool   `json:"default_approval"`
}

type Shell struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Mode       string `json:"mode"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	ExitReason string `json:"exit_reason,omitempty"`
	Archived   bool   `json:"archived,omitempty"`
}

type Session struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	Namespace          string  `json:"namespace,omitempty"`
	ApprovalMode       bool    `json:"approval_mode"`
	ApprovalNeed       int     `json:"approval_need,omitempty"`
	SSHEndpoint        string  `json:"ssh_endpoint,omitempty"`
	ExitReason         string  `json:"exit_reason,omitempty"`
	CreatedAt          int64   `json:"created_at,omitempty"`
	UpdatedAt          int64   `json:"updated_at,omitempty"`
	ArchivedShellsOnly bool    `json:"archived_shells_only,omitempty"`
	Shells             []Shell `json:"shells"`
}

type Forward struct {
	ForwardID string `json:"forward_id"`
	SessionID string `json:"session_id"`
	Direction string `json:"direction"`
	SSHConfig string `json:"ssh_config"`
	Listen    string `json:"listen_addr"`
	Target    string `json:"target_addr"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type ExtensionFailure struct {
	At     int64  `json:"at"`
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

type ExtensionForward struct {
	Kind       string `json:"kind"`
	ForwardID  string `json:"forward_id,omitempty"`
	ListenAddr string `json:"listen_addr,omitempty"`
	TargetAddr string `json:"target_addr,omitempty"`
}

type Extension struct {
	Name        string             `json:"name"`
	Kind        string             `json:"kind,omitempty"`
	Target      string             `json:"target,omitempty"`
	Description string             `json:"description,omitempty"`
	Enabled     bool               `json:"enabled"`
	Status      string             `json:"status"`
	URL         string             `json:"url,omitempty"`
	SessionID   string             `json:"session_id,omitempty"`
	ShellID     string             `json:"shell_id,omitempty"`
	Forwards    []ExtensionForward `json:"forwards,omitempty"`
	Failure     *ExtensionFailure  `json:"failure,omitempty"`
	NextRetryMs int64              `json:"next_retry_ms,omitempty"`
}

type Snapshot struct {
	Core           CoreStatus   `json:"core"`
	Connections    []Connection `json:"connections"`
	Sessions       []Session    `json:"sessions"`
	History        []Session    `json:"history"`
	Forwards       []Forward    `json:"forwards"`
	Extensions     []Extension  `json:"extensions"`
	ExtensionError string       `json:"extension_error,omitempty"`
	FetchedAt      string       `json:"fetched_at"`
}

type HistorySpan struct {
	Status string `json:"status"`
	Time   int64  `json:"time"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
}

type CreateSessionRequest struct {
	Connection string `json:"connection"`
	Name       string `json:"name"`
	Command    string `json:"command"`
}

type APIRequest struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Body        string `json:"body,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Password    string `json:"password,omitempty"`
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

type ConnectionImportHost struct {
	Name      string `json:"name"`
	FinalName string `json:"final_name,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type ConnectionImportResult struct {
	FileName  string                 `json:"file_name"`
	Cancelled bool                   `json:"cancelled"`
	Imported  int                    `json:"imported"`
	Renamed   int                    `json:"renamed"`
	Failed    int                    `json:"failed"`
	Hosts     []ConnectionImportHost `json:"hosts"`
}

type ConnectionExportResult struct {
	Path      string `json:"path"`
	Count     int    `json:"count"`
	Cancelled bool   `json:"cancelled"`
}
