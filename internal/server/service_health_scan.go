package server

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 服务健康探针归集循环（service-health-design.md D8/D9）：每 30s 扫一遍
// 带探针配置的在线 agent → service.health.status（drain=true，事件取走
// 即清）→ 状态入缓存（离线灰态）→ 事件分流：自愈三态落审计
// （username=cockpit-agent——无人值守动作的署名约定），down/recovered/
// backoff/blocked 入告警与通知。配置推送不在这里：保存时与注册上线时
// 各自触发（D3），归集只读不写配置。

var (
	serviceHealthScanTick      = 30 * time.Second // 归集节奏（固定不配置，D8）
	serviceHealthScanStartWait = 45 * time.Second // 启动后等 agent 上线再首轮
)

// serviceHealthEventView agent 事件（镜像 agent 侧 healthEvent）
type serviceHealthEventView struct {
	Time    int64  `json:"time"`
	ProbeID string `json:"probeId"`
	Kind    string `json:"kind"`
	Unit    string `json:"unit"`
	Detail  string `json:"detail"`
}

// serviceHealthScanLoop 归集循环（smart_scan 同构：每 tick 醒来即扫，
// 节奏固定无需动态间隔）
func (s *Server) serviceHealthScanLoop() {
	// 启动等待改局部量 + ctx 可中断 select（driftScanLoop 同款）：关停不
	// 留启动宽限期的僵尸 goroutine，泄漏循环也不再跨测试边界读包级节奏
	// 变量构成数据竞争（CI -race 实证 2026-10-07）
	startWait, tick := serviceHealthScanStartWait, serviceHealthScanTick
	select {
	case <-time.After(startWait):
	case <-s.ctx.Done():
		return
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.scanServiceHealthOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// listServiceHealthAgents 带探针配置的 agent 清单（Setting 键前缀扫描）
func (s *Server) listServiceHealthAgents() []string {
	keys, err := s.db.ListSettingKeys(ServiceHealthConfigPrefix)
	if err != nil {
		log.Printf("Service health scan: list configs: %v", err)
		return nil
	}
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k[len(ServiceHealthConfigPrefix):])
	}
	sort.Strings(ids)
	return ids
}

// scanServiceHealthOnce 扫一轮：仅在线 agent（离线的探针照跑、事件缓冲，
// 重连后下一轮补收，D2/D8）
func (s *Server) scanServiceHealthOnce() {
	agentIDs := s.listServiceHealthAgents()
	if len(agentIDs) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	scanned := 0
	for _, agentID := range agentIDs {
		agent, ok := s.registry.Get(agentID)
		if !ok || !agent.IsOnline(s.registry.timeouts.Heartbeat) {
			continue
		}
		states, events, err := s.drainServiceHealth(agentID)
		if err != nil {
			log.Printf("Service health scan skip agent %s: %v", agentID, err)
			continue
		}
		scanned++
		if states != nil {
			s.cacheHealthState(agentID, states)
		}
		s.processServiceHealthEvents(generator, agentID, agent.Hostname, events)
	}
	if scanned > 0 {
		log.Printf("Service health scan completed: %d agents scanned", scanned)
	}
}

// drainServiceHealth 拉取运行态并取走事件（drain=true 语义仅归集循环使用；
// 仪表盘读路径 fetchHealthStates 恒 drain=false，防止浏览吞事件）
func (s *Server) drainServiceHealth(agentID string) (map[string]healthStateEntry, []serviceHealthEventView, error) {
	resp, err := s.CallAgent(agentID, "service.health.status", map[string]interface{}{"drain": true})
	if err != nil {
		return nil, nil, err
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return nil, nil, err
	}
	raw, err := healthJSONMarshal(rpcResp.Data)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		States map[string]healthStateEntry `json:"states"`
		Events []serviceHealthEventView    `json:"events"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}
	return payload.States, payload.Events, nil
}

// processServiceHealthEvents 事件分流（D9）：自愈三态每条落审计；
// down/blocked/backoff 入告警（title 含 probeId → 同探针未读真去重）；
// 六类全发通知（事件白名单在 notification.Service 内过滤）。
func (s *Server) processServiceHealthEvents(generator *alert.Generator, agentID, hostname string, events []serviceHealthEventView) {
	if len(events) == 0 {
		return
	}
	if hostname == "" {
		hostname = agentID
	}
	var issues []alert.ServiceHealthIssue
	for _, ev := range events {
		unit := ev.Unit
		if unit == "" {
			unit = ev.ProbeID
		}
		resourceID := agentID + "/" + unit
		label := fmt.Sprintf("%s@%s", ev.ProbeID, hostname)
		switch ev.Kind {
		case "heal_restarted":
			s.audit.LogResource("cockpit-agent", audit.ActionServiceHealthHeal, "service", resourceID,
				map[string]interface{}{"probeId": ev.ProbeID, "unit": unit,
					"detail": ev.Detail, "eventTime": ev.Time},
				"", "cockpit-agent")
			s.notifyServiceHealth(notification.ServiceHealthHealed, "info",
				"服务自愈成功："+label, ev.Detail, resourceID)
		case "heal_failed":
			s.audit.LogResource("cockpit-agent", audit.ActionServiceHealthHealFailed, "service", resourceID,
				map[string]interface{}{"probeId": ev.ProbeID, "unit": unit,
					"detail": ev.Detail, "eventTime": ev.Time},
				"", "cockpit-agent")
			s.notifyServiceHealth(notification.ServiceHealthHealFailed, "error",
				"服务自愈失败："+label, ev.Detail, resourceID)
		case "heal_blocked":
			s.audit.LogResource("cockpit-agent", audit.ActionServiceHealthHealBlocked, "service", resourceID,
				map[string]interface{}{"probeId": ev.ProbeID, "unit": unit,
					"detail": ev.Detail, "eventTime": ev.Time},
				"", "cockpit-agent")
			issues = append(issues, alert.ServiceHealthIssue{
				ProbeID:   ev.ProbeID,
				Title:     fmt.Sprintf("服务健康探针：%s 自愈被白名单拦截（%s）", ev.ProbeID, hostname),
				Message:   "自愈目标 " + unit + " 不在该机的自愈白名单，面板只告警不动作。\n" + ev.Detail,
				AlertType: "warning",
			})
			s.notifyServiceHealth(notification.ServiceHealthHealBlocked, "warning",
				"服务自愈被拦截："+label, ev.Detail, resourceID)
		case "probe_down":
			issues = append(issues, alert.ServiceHealthIssue{
				ProbeID:   ev.ProbeID,
				Title:     fmt.Sprintf("服务健康探针：%s 连续失败（%s）", ev.ProbeID, hostname),
				Message:   "探针 " + ev.ProbeID + " 连续失败达阈值，目标 " + unit + " 可能不可用。\n" + ev.Detail,
				AlertType: "error",
			})
			s.notifyServiceHealth(notification.ServiceHealthDown, "error",
				"服务健康探针失败："+label, ev.Detail, resourceID)
		case "probe_recovered":
			s.notifyServiceHealth(notification.ServiceHealthUp, "info",
				"服务健康探针恢复："+label, ev.Detail, resourceID)
		case "heal_backoff":
			issues = append(issues, alert.ServiceHealthIssue{
				ProbeID:   ev.ProbeID,
				Title:     fmt.Sprintf("服务健康探针：%s 自愈退避额度耗尽（%s）", ev.ProbeID, hostname),
				Message:   "重启窗口额度已用尽，为防重启风暴暂停自愈，仅持续告警。\n" + ev.Detail,
				AlertType: "warning",
			})
			s.notifyServiceHealth(notification.ServiceHealthBackoff, "warning",
				"服务自愈退避："+label, ev.Detail, resourceID)
		default:
			log.Printf("Service health scan: unknown event kind %q from %s", ev.Kind, agentID)
		}
	}
	if len(issues) > 0 {
		generator.CheckServiceHealth(agentID, issues)
	}
}

// notifyServiceHealth 通知扇出（非阻塞；事件开关在 notification.Service
// 内过滤，白名单显式启用见 events.go）
func (s *Server) notifyServiceHealth(eventType, level, title, message, resourceID string) {
	s.notifier.SendNonBlocking(&notification.Notification{
		EventType:    eventType,
		Title:        title,
		Message:      message,
		Level:        level,
		ResourceType: "service",
		ResourceID:   resourceID,
		Time:         time.Now(),
	})
}
