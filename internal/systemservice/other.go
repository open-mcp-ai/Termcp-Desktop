//go:build !darwin && !linux && !windows

package systemservice

import "runtime"

type unsupportedManager struct{ executable string }

func newPlatformManager(executable string) Manager {
	return &unsupportedManager{executable: executable}
}

func (m *unsupportedManager) Status() (Status, error) {
	return Status{Supported: false, Platform: runtime.GOOS, Label: Label, Executable: m.executable, Description: "当前平台尚未实现系统服务管理"}, nil
}
func (*unsupportedManager) Install(bool) error      { return ErrUnsupported }
func (*unsupportedManager) Uninstall() error        { return ErrUnsupported }
func (*unsupportedManager) Start() error            { return ErrUnsupported }
func (*unsupportedManager) Stop() error             { return ErrUnsupported }
func (*unsupportedManager) Restart() error          { return ErrUnsupported }
func (*unsupportedManager) SetAutostart(bool) error { return ErrUnsupported }
