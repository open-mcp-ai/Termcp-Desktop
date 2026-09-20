package systemservice

const (
	Label              = "ai.openmcp.termcp.desktop.core"
	WindowsServiceName = "termcp-desktop-core"
)

type Status struct {
	Supported   bool   `json:"supported"`
	Platform    string `json:"platform"`
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	Autostart   bool   `json:"autostart"`
	PID         int    `json:"pid,omitempty"`
	Label       string `json:"label"`
	Definition  string `json:"definition,omitempty"`
	LogPath     string `json:"log_path,omitempty"`
	Executable  string `json:"executable,omitempty"`
	Description string `json:"description,omitempty"`
}

type Manager interface {
	Status() (Status, error)
	Install(autostart bool) error
	Uninstall() error
	Start() error
	Stop() error
	Restart() error
	SetAutostart(enabled bool) error
}

func New(executable string) Manager {
	return newPlatformManager(executable)
}
