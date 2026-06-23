//go:build !tray

package serve

// RunTray 空实现（未启用 tray build tag 时）。
func RunTray(serverURL string, onExit func()) {}

// InitTray 空实现。
func InitTray(port string) {}
