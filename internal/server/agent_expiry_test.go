package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// agent_expiry_test.go 过期 agent 自动判定（D-2026-10-08-3）：阈值配置
// （yaml 默认/env 覆盖/负值关闭）、expireStaleAgents 假在线标离线 + 注册表
// 资源释放、心跳回写 DB last_seen、/api/status 下发阈值。

func TestAgentExpireThresholdDefaultAndNegative(t *testing.T) {
	// Normalize 默认 5 分钟
	cfg := config.Normalize(nil)
	if cfg.Agent.ExpireMinutes != 5 {
		t.Fatalf("default expireMinutes = %d, want 5", cfg.Agent.ExpireMinutes)
	}
	s := &Server{cfg: cfg}
	if got := s.agentExpireThreshold(); got != 5*time.Minute {
		t.Errorf("threshold = %v, want 5m", got)
	}

	// 负值 = 关闭（返回 0）
	cfgOff := config.Normalize(nil)
	cfgOff.Agent.ExpireMinutes = -1
	sOff := &Server{cfg: cfgOff}
	if got := sOff.agentExpireThreshold(); got != 0 {
		t.Errorf("threshold = %v, want 0 (disabled)", got)
	}

	// nil cfg 兜底默认（测试直接构造 Server 的场景）
	sNil := &Server{}
	if got := sNil.agentExpireThreshold(); got != 5*time.Minute {
		t.Errorf("nil-cfg threshold = %v, want 5m", got)
	}
}

func TestApplyAgentExpireEnv(t *testing.T) {
	cfg := config.Normalize(nil)

	t.Setenv("AGENT_EXPIRE_MINUTES", "2")
	applyAgentExpireEnv(cfg)
	if cfg.Agent.ExpireMinutes != 2 {
		t.Errorf("env override = %d, want 2", cfg.Agent.ExpireMinutes)
	}

	// 非法值保持原配置
	t.Setenv("AGENT_EXPIRE_MINUTES", "abc")
	applyAgentExpireEnv(cfg)
	if cfg.Agent.ExpireMinutes != 2 {
		t.Errorf("invalid env should keep %d, got %d", 2, cfg.Agent.ExpireMinutes)
	}

	// 未设置时不动 yaml 值
	t.Setenv("AGENT_EXPIRE_MINUTES", "")
	cfg.Agent.ExpireMinutes = 7
	applyAgentExpireEnv(cfg)
	if cfg.Agent.ExpireMinutes != 7 {
		t.Errorf("unset env should keep 7, got %d", cfg.Agent.ExpireMinutes)
	}
}

func TestExpireStaleAgents(t *testing.T) {
	s := covNewServer(t)
	stale := time.Now().Add(-10 * time.Minute)
	fresh := time.Now()

	// 注意：Agent.BeforeCreate 会就地改写入参 LastSeen 为 now——回拨用捕获变量
	seed := []struct {
		id     string
		status string
		ts     time.Time
	}{
		{"ag-stale-online", "online", stale},
		{"ag-fresh-online", "online", fresh},
		{"ag-stale-offline", "offline", stale},
	}
	for _, r := range seed {
		if err := s.db.UpsertAgent(&storage.Agent{ID: r.id, Hostname: r.id, Status: r.status}); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
		if err := s.db.UpdateAgentStatus(r.id, r.status, r.ts); err != nil {
			t.Fatalf("backdate %s: %v", r.id, err)
		}
	}

	// 注册表里挂一台假在线（模拟心跳停了但 sweep 前注册表未清）
	ghost := NewAgent("ag-stale-online", nil)
	ghost.LastSeen = stale
	if err := s.registry.Register(ghost); err != nil {
		t.Fatalf("register ghost: %v", err)
	}

	s.expireStaleAgents()

	got, err := s.db.GetAgent("ag-stale-online")
	if err != nil {
		t.Fatalf("load stale agent: %v", err)
	}
	if got.Status != "offline" {
		t.Errorf("stale-online status = %q, want offline", got.Status)
	}
	if _, exists := s.registry.Get("ag-stale-online"); exists {
		t.Error("stale agent should be unregistered (resource release)")
	}

	freshRow, err := s.db.GetAgent("ag-fresh-online")
	if err != nil {
		t.Fatalf("load fresh agent: %v", err)
	}
	if freshRow.Status != "online" {
		t.Errorf("fresh-online status = %q, want online", freshRow.Status)
	}

	gone, err := s.db.GetAgent("ag-stale-offline")
	if err != nil {
		t.Fatalf("load offline agent: %v", err)
	}
	if gone.Status != "offline" || gone.LastSeen.Unix() != stale.Unix() {
		t.Errorf("stale-offline row mutated: status=%q lastSeen=%v", gone.Status, gone.LastSeen)
	}
}

func TestExpireStaleAgentsDisabled(t *testing.T) {
	s := covNewServer(t)
	s.cfg = config.Normalize(nil)
	s.cfg.Agent.ExpireMinutes = -1

	if err := s.db.UpsertAgent(&storage.Agent{
		ID: "ag-old", Hostname: "old", Status: "online",
		LastSeen: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.db.UpdateAgentStatus("ag-old", "online", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	s.expireStaleAgents()

	got, err := s.db.GetAgent("ag-old")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Status != "online" {
		t.Errorf("disabled expiry should keep online, got %q", got.Status)
	}
}

func TestHeartbeatTouchesDBLastSeen(t *testing.T) {
	s := covNewServer(t)
	old := time.Now().Add(-30 * time.Minute)
	if err := s.db.UpsertAgent(&storage.Agent{
		ID: "agent-touch", Hostname: "touch", Status: "online", LastSeen: old,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// BeforeCreate 把 last_seen 强制为 now——回拨，否则 touch 断言恒真
	if err := s.db.UpdateAgentStatus("agent-touch", "online", old); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	agent := NewAgent("agent-touch", nil)
	msg := protocol.NewMessage(protocol.MessageTypeHeartbeat, nil)
	s.handleHeartbeat(agent, msg)
	covDrainHeartbeatAck(t, agent, msg.ID)

	got, err := s.db.GetAgent("agent-touch")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.LastSeen.Unix() <= old.Unix() {
		t.Errorf("last_seen not touched: got %v, want > %v", got.LastSeen, old)
	}
}

func TestHandleStatusExposesExpireMinutes(t *testing.T) {
	s := covNewServer(t)
	s.cfg = config.Normalize(nil) // 默认 5 分钟

	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()
	s.serveAPI(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	minutes, ok := result["agentExpireMinutes"].(float64)
	if !ok {
		t.Fatalf("agentExpireMinutes missing in %v", result)
	}
	if int(minutes) != 5 {
		t.Errorf("agentExpireMinutes = %v, want 5", minutes)
	}
}

// TestExpireStaleAgentsSweepError sweep 存储故障分支：closed db → 记日志
// 直接返回，不 panic（对齐 api_dns_cmdb_test 的 closed-db 注入先例）
func TestExpireStaleAgentsSweepError(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s.expireStaleAgents() // 错误分支内 log+return，走到这里不 panic 即过
}
