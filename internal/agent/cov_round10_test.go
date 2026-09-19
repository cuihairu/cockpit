package agent

// cov_round10_test.go 覆盖率：buildCapabilities 的 traefik-proxy 探测
// 分支——COCKPIT_TRAEFIK_DIR 指向存在目录时 DetectTraefik 命中，
// capability 携带 dynamicDir metadata。
// windows/darwin 的 service backend 分支按 GOOS 编译期固定，Linux 不可达。

import "testing"

// TestCovDetectTraefikCapability env 指定动态目录 → traefik-proxy 上报
func TestCovDetectTraefikCapability(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_TRAEFIK_DIR", dir)

	a := NewAgent(Config{ServerURL: "ws://test"})
	caps := a.detectCapabilities()
	found := false
	for _, c := range caps {
		if c.Type == "traefik-proxy" {
			found = true
			if c.Metadata["dynamicDir"] != dir {
				t.Fatalf("dynamicDir = %v, want %v", c.Metadata["dynamicDir"], dir)
			}
		}
	}
	if !found {
		t.Fatalf("traefik-proxy capability missing, got %v", caps)
	}
}
