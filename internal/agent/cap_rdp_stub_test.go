//go:build !rdp || darwin

package agent

import "testing"

// stub 构建下 rdpClientAvailable 必须为 false：
// detectCapabilities 不上报 rdp-client，Web 据此禁用 RDP 入口。
func TestRDPClientAvailableStubFalse(t *testing.T) {
	if rdpClientAvailable() {
		t.Error("stub build should report rdpClientAvailable() == false")
	}
}
