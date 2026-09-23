//go:build rdp && !darwin

package agent

import "testing"

// -tags rdp 非 darwin 构建下 rdpClientAvailable 必须为 true：
// detectCapabilities 上报 rdp-client，Web 开放 RDP 入口。
func TestRDPClientAvailableEnabled(t *testing.T) {
	if !rdpClientAvailable() {
		t.Error("-tags rdp build should report rdpClientAvailable() == true")
	}
}
