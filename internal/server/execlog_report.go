package server

import (
	"log"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// execLogSource 审计 username/userAgent 标记：条目来自 agent 侧执行留痕
// 归集，非人工操作（同 service_health 自愈归集的 cockpit-agent 口径）。
const execLogSource = "cockpit-agent"

// handleExecLogReport 处理 agent 执行留痕上报（agent-core P5）：每条落
// server 审计表。audit_logs 为追加表、无外键去重——上行语义是尽力而为
// 一次投递（agent 发送失败不重发，见 core/execlog 批），重复投递会留
// 重复行，对账以 details.execId 为准。
func (s *Server) handleExecLogReport(agent *Agent, msg *protocol.Message) {
	if s.audit == nil || s.db == nil {
		return
	}
	p, err := protocol.DecodeExecLog(msg)
	if err != nil {
		log.Printf("exec_log decode failed from agent %s: %v", agent.ID, err)
		return
	}
	for i := range p.Entries {
		e := p.Entries[i]
		// 未知状态按 failure 记（审计宁可误报失败，不可把坏账记成成功）
		status := audit.StatusFailure
		if e.Status == audit.StatusSuccess {
			status = audit.StatusSuccess
		}
		details := map[string]interface{}{
			"agent":      agent.ID,
			"execId":     e.ID,
			"eventTime":  e.Time,
			"provider":   e.Provider,
			"target":     e.Target,
			"durationMs": e.DurationMs,
		}
		if e.Error != "" {
			details["error"] = e.Error
		}
		if len(e.Params) > 0 {
			details["params"] = e.Params
		}
		resourceID := e.Target
		if resourceID == "" {
			resourceID = e.ID
		}
		if err := s.audit.Log(&audit.LogEntry{
			Username:   execLogSource,
			Action:     audit.ActionExec,
			Resource:   e.Provider,
			ResourceID: resourceID,
			Details:    details,
			UserAgent:  execLogSource,
			Status:     status,
		}); err != nil {
			// 同表连续写大概率同因失败，止于首错（错误已带 agent 定位）
			log.Printf("exec_log audit write failed for agent %s: %v", agent.ID, err)
			return
		}
	}
}
