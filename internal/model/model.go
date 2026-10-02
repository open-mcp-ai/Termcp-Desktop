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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Mode     string `json:"mode"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

type Session struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	ApprovalMode bool    `json:"approval_mode"`
	ApprovalNeed int     `json:"approval_need,omitempty"`
	SSHEndpoint  string  `json:"ssh_endpoint,omitempty"`
	CreatedAt    int64   `json:"created_at,omitempty"`
	UpdatedAt    int64   `json:"updated_at,omitempty"`
	Shells       []Shell `json:"shells"`
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

type Snapshot struct {
	Core        CoreStatus   `json:"core"`
	Connections []Connection `json:"connections"`
	Sessions    []Session    `json:"sessions"`
	History     []Session    `json:"history"`
	Forwards    []Forward    `json:"forwards"`
	FetchedAt   string       `json:"fetched_at"`
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
