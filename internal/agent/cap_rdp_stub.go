//go:build !rdp || darwin

package agent

// rdpClientAvailable stub 构建（未开 -tags rdp 或 darwin）无 RDP 客户端。
var rdpClientAvailable = func() bool { return false }
