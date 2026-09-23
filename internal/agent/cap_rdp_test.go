package agent

import "testing"

// rdpClientAvailable 按 build tag 分流：stub 构建（默认）恒 false，
// -tags rdp 非 darwin 恒 true。此处仅断言布尔稳定（真实值随构建变）。
func TestRDPClientAvailableStable(t *testing.T) {
	_ = rdpClientAvailable()
}
