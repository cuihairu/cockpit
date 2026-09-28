package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
)

// scanHealthEventsFixture 六类事件全量的 agent 应答（states + events）
func scanHealthEventsFixture() (interface{}, string) {
	return map[string]interface{}{
		"states": map[string]interface{}{
			"cloudflared-ready": map[string]interface{}{
				"status": "fail", "consecutiveFails": 3, "lastError": "HTTP 502 (expect 200)"},
		},
		"events": []interface{}{
			map[string]interface{}{"probeId": "cloudflared-ready", "kind": "probe_down",
				"unit": "cloudflared.service", "detail": "3 consecutive fails: HTTP 502"},
			map[string]interface{}{"probeId": "cloudflared-ready", "kind": "heal_restarted",
				"unit": "cloudflared.service", "detail": "systemctl restart ok (fails=3)"},
			map[string]interface{}{"probeId": "cloudflared-ready", "kind": "probe_recovered",
				"unit": "cloudflared.service", "detail": "recovered after 3 fails"},
			map[string]interface{}{"probeId": "cloudflared-ready", "kind": "heal_failed",
				"unit": "cloudflared.service", "detail": "systemctl restart: exit 1"},
			map[string]interface{}{"probeId": "other", "kind": "heal_blocked",
				"unit": "rogue.service", "detail": "unit not in whitelist"},
			map[string]interface{}{"probeId": "other", "kind": "heal_backoff",
				"unit": "rogue.service", "detail": "budget exhausted"},
		},
	}, ""
}

// TestServiceHealthScanEvents 归集分流：自愈三态落审计（username=
// cockpit-agent），down/blocked/backoff 入告警，状态入缓存（D8/D9）
func TestServiceHealthScanEvents(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "service.health.status" {
			if drain, _ := params["drain"].(bool); !drain {
				t.Error("scan must drain")
			}
			return scanHealthEventsFixture()
		}
		return nil, ""
	})
	// 带通知配置的 server（s.cfg 非 nil 分支；空配置走 Generator 默认）
	s.cfg = &config.Config{Notification: &config.NotificationConfig{}}
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))

	s.scanServiceHealthOnce()

	// 审计：heal_restarted / heal_failed / heal_blocked 各一条，
	// username=cockpit-agent，resourceID=a1/<unit>
	logs, total, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	counts := map[string]int{}
	byAction := map[string]*struct {
		username, resourceID string
	}{}
	for i := range logs {
		l := logs[i]
		counts[l.Action]++
		byAction[l.Action] = &struct {
			username, resourceID string
		}{l.Username, l.ResourceID}
	}
	if total != 3 || counts[audit.ActionServiceHealthHeal] != 1 ||
		counts[audit.ActionServiceHealthHealFailed] != 1 ||
		counts[audit.ActionServiceHealthHealBlocked] != 1 {
		t.Fatalf("audit actions: %+v total=%d", counts, total)
	}
	for _, action := range []string{audit.ActionServiceHealthHeal, audit.ActionServiceHealthHealFailed, audit.ActionServiceHealthHealBlocked} {
		if byAction[action].username != "cockpit-agent" {
			t.Errorf("%s username = %q", action, byAction[action].username)
		}
		if byAction[action].resourceID != "a1/cloudflared.service" && byAction[action].resourceID != "a1/rogue.service" {
			t.Errorf("%s resourceID = %q", action, byAction[action].resourceID)
		}
	}

	// 告警：down(error)、blocked(warning)、backoff(warning)，按 probeId 独立 title
	for _, tc := range []struct {
		title, alertType string
	}{
		{"服务健康探针：cloudflared-ready 连续失败（a1）", "error"},
		{"服务健康探针：other 自愈被白名单拦截（a1）", "warning"},
		{"服务健康探针：other 自愈退避额度耗尽（a1）", "warning"},
	} {
		ok, err := s.db.HasUnreadAlert("agent", "a1", tc.title)
		if err != nil || !ok {
			t.Errorf("alert %q missing (err=%v)", tc.title, err)
		}
	}
	// recovered 不进告警
	unread, _ := s.db.ListUnreadAlerts()
	if len(unread) != 3 {
		t.Fatalf("unread alerts = %d, want 3", len(unread))
	}

	// 状态入缓存（离线灰态）
	cached := s.loadHealthStateCache("a1")
	if cached == nil || cached.States["cloudflared-ready"].LastError != "HTTP 502 (expect 200)" {
		t.Fatalf("cache: %+v", cached)
	}
}

// TestServiceHealthScanAlertDedup 同探针未读期间只报一次（D9 真去重）
func TestServiceHealthScanAlertDedup(t *testing.T) {
	s, _ := newOverlayServer(t, func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{
			"states": map[string]interface{}{},
			"events": []interface{}{map[string]interface{}{
				"probeId": "p1", "kind": "probe_down", "unit": "a.service", "detail": "x"}},
		}, ""
	})
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))

	s.scanServiceHealthOnce()
	s.scanServiceHealthOnce() // 同一未读告警还在：第二轮不重复建

	unread, err := s.db.ListUnreadAlerts()
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if len(unread) != 1 {
		t.Fatalf("unread alerts = %d, want 1 (dedup)", len(unread))
	}
}

// TestServiceHealthScanOfflineAndEmpty 离线 agent 跳过（事件留 agent 缓冲），
// 无配置空转不报错
func TestServiceHealthScanOfflineAndEmpty(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	s.registry.Unregister("a1")
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))

	s.scanServiceHealthOnce() // 离线跳过
	if s.loadHealthStateCache("a1") != nil {
		t.Fatal("offline agent must not be scanned")
	}

	// 无配置：空转
	_ = s.db.DeleteSetting(ServiceHealthConfigPrefix + "a1")
	s.scanServiceHealthOnce()
}

// TestServiceHealthScanLoopLifecycle 归集循环本体：启动等待 + tick 扫描 +
// ctx 取消退出（节奏 var 注入缩短，行为中性）
func TestServiceHealthScanLoopLifecycle(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ctx = ctx

	oldWait, oldTick := serviceHealthScanStartWait, serviceHealthScanTick
	serviceHealthScanStartWait, serviceHealthScanTick = time.Millisecond, 15*time.Millisecond
	defer func() { serviceHealthScanStartWait, serviceHealthScanTick = oldWait, oldTick }()

	done := make(chan struct{})
	go func() { s.serviceHealthScanLoop(); close(done) }()
	time.Sleep(100 * time.Millisecond) // 至少数轮空扫
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scan loop did not exit on ctx cancel")
	}
}

// TestServiceHealthScanListError 配置清单查询失败：整轮放弃不 panic
func TestServiceHealthScanListError(t *testing.T) {
	s, _ := newOverlayServer(t, nil)
	_ = s.db.Close()
	s.scanServiceHealthOnce()
}

// TestServiceHealthScanBadPayloads agent 应答形状异常两形态（Data 标量 /
// 整包不可解码）均按单 agent 失败跳过，不影响循环
func TestServiceHealthScanBadPayloads(t *testing.T) {
	calls := 0
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		calls++
		if calls == 1 {
			return 42, "" // Data 标量：重编码后反序列化不进 states/events 形状
		}
		return map[string]interface{}{"x": make(chan int)}, "" // 解码失败
	})
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))

	s.scanServiceHealthOnce()
	s.scanServiceHealthOnce()
	if s.loadHealthStateCache("a1") != nil {
		t.Fatal("bad payloads must not produce cache")
	}
}

// TestServiceHealthScanEventShapes 事件形状边界：空事件只刷缓存；缺 unit 的
// 事件退化用 probeId；未知 kind 记日志不投递
func TestServiceHealthScanEventShapes(t *testing.T) {
	calls := 0
	s, _ := newOverlayServer(t, func(_ string, _ map[string]interface{}) (interface{}, string) {
		calls++
		if calls == 1 {
			return map[string]interface{}{
				"states": map[string]interface{}{
					"p1": map[string]interface{}{"status": "ok"}},
				"events": []interface{}{}}, ""
		}
		return map[string]interface{}{
			"states": map[string]interface{}{},
			"events": []interface{}{
				map[string]interface{}{"probeId": "p1", "kind": "probe_recovered"}, // 无 unit
				map[string]interface{}{"probeId": "x", "unit": "u.service", "kind": "wat"},
			},
		}, ""
	})
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))

	s.scanServiceHealthOnce() // 空事件：只缓存
	cached := s.loadHealthStateCache("a1")
	if cached == nil || cached.States["p1"].Status != "ok" {
		t.Fatalf("cache after empty-events scan: %+v", cached)
	}
	unread, _ := s.db.ListUnreadAlerts()
	if len(unread) != 0 {
		t.Fatalf("empty events must not alert: %d", len(unread))
	}

	s.scanServiceHealthOnce() // 形状边界事件：不 panic 不告警
	unread, _ = s.db.ListUnreadAlerts()
	if len(unread) != 0 {
		t.Fatalf("recovered/unknown must not alert: %d", len(unread))
	}
}

// TestServiceHealthScanCallAgentFailure agent 通道已关但心跳仍新鲜：本轮
// 跳过，事件留 agent 侧缓冲待重连补收（D2/D8）
func TestServiceHealthScanCallAgentFailure(t *testing.T) {
	s, agent := newOverlayServer(t, nil)
	raw, _ := json.Marshal(healthAgentBody())
	_ = s.db.SetSetting(ServiceHealthConfigPrefix+"a1", string(raw))
	agent.Close() // Send 关闭但 LastSeen 新鲜：IsOnline 仍 true → CallAgent 失败
	s.scanServiceHealthOnce()
	if s.loadHealthStateCache("a1") != nil {
		t.Fatal("failed drain must not cache")
	}
}
