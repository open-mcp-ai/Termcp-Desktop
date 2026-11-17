package model

// ActivityEvent is one agent, human, or review action reported by Core.
type ActivityEvent struct {
	Time        string `json:"time"`
	UnixMS      int64  `json:"unix_ms"`
	SessionID   string `json:"session_id"`
	ShellID     string `json:"shell_id"`
	SessionName string `json:"session_name,omitempty"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	Verdict     string `json:"verdict"`
	Bytes       int64  `json:"bytes"`
}

// WebUIClient is one desktop or browser client watching Core.
type WebUIClient struct {
	ID        string `json:"id"`
	Addr      string `json:"addr"`
	UserAgent string `json:"user_agent,omitempty"`
	Watching  int    `json:"watching"`
	Since     string `json:"since"`
	Self      bool   `json:"self,omitempty"`
}

// MCPClient is one agent holding a live MCP transport session.
type MCPClient struct {
	SessionID   string `json:"session_id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Namespace   string `json:"namespace,omitempty"`
	ConnectedAt string `json:"connected_at"`
}

// Clients groups the two connection kinds Core reports separately.
type Clients struct {
	WebUI []WebUIClient `json:"webui"`
	MCP   []MCPClient   `json:"mcp"`
}
