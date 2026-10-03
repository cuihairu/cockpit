package server

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// cov_heartbeat_presence_test.go 心跳 presence 分支（dispatcher.go
// handleHeartbeat 的 StartedAt/Services 落库段，bc1098c）：内存侧刷新、
// DB 刷新、落库失败仅 log、字段缺失时跳过。

func covDrainHeartbeatAck(t *testing.T, agent *Agent, wantID string) {
	t.Helper()
	select {
	case got := <-agent.Send:
		if got.ID != wantID || got.Type != protocol.MessageTypeHeartbeat {
			t.Fatalf("ack = %+v, want hb id %s", got, wantID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat ack")
	}
}

func TestCovHandleHeartbeatPresence(t *testing.T) {
	s := covNewServer(t)
	agent := NewAgent("agent-pres", nil)
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-pres", Hostname: "h-pres"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	st := time.Unix(1700000123, 0)
	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"agentId":   "agent-pres",
		"startedAt": st.Unix(),
		"services": []map[string]interface{}{
			{"protocol": "tcp", "host": "10.0.0.1", "port": 22, "running": true,
				"detectedAt": st.Unix()},
		},
		"systemInfo": map[string]interface{}{"cpuCores": 8, "hostname": "h-pres"},
	})
	s.handleHeartbeat(agent, msg)
	covDrainHeartbeatAck(t, agent, msg.ID)

	// 内存侧：启动时刻 + 服务面就地刷新
	if !agent.StartedAt.Equal(st) {
		t.Errorf("agent.StartedAt = %v, want %v", agent.StartedAt, st)
	}
	if len(agent.Services) != 1 || agent.Services[0].Port != 22 ||
		agent.Services[0].Protocol != "tcp" {
		t.Errorf("agent.Services = %+v", agent.Services)
	}

	// 落库：DB 同步 presence
	dbAgent, err := s.db.GetAgent("agent-pres")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !dbAgent.StartedAt.Equal(st) {
		t.Errorf("db StartedAt = %v, want %v", dbAgent.StartedAt, st)
	}
	if len(dbAgent.Services) != 1 || dbAgent.Services[0].Port != 22 {
		t.Errorf("db Services = %+v", dbAgent.Services)
	}

	// 解码成功但无 presence 字段（条件为假）→ 跳过 presence，仍 ACK
	plain := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"status": "ok",
	})
	s.handleHeartbeat(agent, plain)
	covDrainHeartbeatAck(t, agent, plain.ID)

	// DB 关闭 → 落库失败仅 log，ACK 照发（presence 更新不得打断心跳）
	if err := s.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	fail := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"startedAt": st.Unix() + 60,
	})
	s.handleHeartbeat(agent, fail)
	covDrainHeartbeatAck(t, agent, fail.ID)
}
