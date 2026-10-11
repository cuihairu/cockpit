package agent

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
)

// applyVaultNasTargets 注册响应引导（vault-design.md V6）：无载荷/不可序列
// 化/空数组均不动作；无既有 provider 补注册，有时热更新（同实例整组替换）。
func TestApplyVaultNasTargets(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})

	// 无 nasTargets 键
	a.applyVaultNasTargets(map[string]any{"status": "accepted"})
	if a.rpc.Provider("nas") != nil {
		t.Fatal("payload without nasTargets must be a no-op")
	}

	// 不可序列化载荷（函数值）
	a.applyVaultNasTargets(map[string]any{"nasTargets": func() {}})
	if a.rpc.Provider("nas") != nil {
		t.Fatal("unserializable payload must be a no-op")
	}

	// 空数组
	a.applyVaultNasTargets(map[string]any{"nasTargets": []any{}})
	if a.rpc.Provider("nas") != nil {
		t.Fatal("empty targets must be a no-op")
	}

	// 无既有 provider → 补注册（纯 vault 部署）
	a.applyVaultNasTargets(map[string]any{"nasTargets": []map[string]any{
		{"name": "b1", "type": "dsm", "addr": "https://h:5001", "username": "u", "password": "p"},
	}})
	np := a.rpc.Provider("nas")
	if np == nil {
		t.Fatal("vault targets must register nas provider")
	}
	if _, ok := np.(*rpc.NasProvider); !ok {
		t.Fatalf("registered provider type = %T", np)
	}

	// 既有 provider → 热更新（同实例，不重注册；vault 覆盖 env 引导值）
	a.applyVaultNasTargets(map[string]any{"nasTargets": []map[string]any{
		{"name": "b2", "type": "omv", "addr": "https://h2", "username": "u2", "password": "p2"},
	}})
	if a.rpc.Provider("nas") != np {
		t.Fatal("existing provider must be hot-updated, not re-registered")
	}
}
