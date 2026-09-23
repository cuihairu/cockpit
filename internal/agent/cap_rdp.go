//go:build rdp && !darwin

package agent

// rdpClientAvailable RDP 桌面客户端可用（-tags rdp 且非 darwin 构建）。
// stub 构建返回 false，UI 据 capability 禁用 RDP 入口而非连上后吃 error。
func rdpClientAvailable() bool { return true }
