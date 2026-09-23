package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 覆盖率缺口补测（只补测试零业务改动）：registerStackProvider 的
// ComposeAvailable=false 跳过分支。

func TestCovRegisterStackProviderNoCompose(t *testing.T) {
	// PATH 为空目录：docker compose CLI 不可用 → ComposeAvailable false
	fakeBinDirCov(t, map[string]string{})
	withEnv(t, map[string]string{"DOCKER_HOST": ""})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "docker-api", Endpoint: "unix:///var/run/docker.sock"},
	}
	a.setupProviders()

	if covRegistered(a, "stack") {
		t.Errorf("stack provider should be skipped without compose CLI, got %v", a.rpc.RegisteredTypes())
	}
}
