package server

import (
	"errors"
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 升级意图状态机巡检（P0 Agent 第四片，见 docs/design/agent-upgrade-rollback.md
// §3.2/§3.3）。状态推进由 server 巡检驱动，不依赖前端轮询写状态。
// 边界：控制面只观察 agent 上报版本/能力并推进意图；agent 二进制 OS 级
// 换装需真机验收（诚实延后，本片不推包）。

// upgradeReconcileInterval 巡检周期（包级变量仅为测试可注入）。
var upgradeReconcileInterval = 5 * time.Second

// upgradeReconcileLoop 巡检循环：推进所有非终态升级意图。
func (s *Server) upgradeReconcileLoop() {
	ticker := time.NewTicker(upgradeReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.reconcileUpgrades()
		}
	}
}

// reconcileUpgrades 推进全部非终态意图（单轮，失败不中断其余）。
func (s *Server) reconcileUpgrades() {
	intents, err := s.db.ListActiveUpgrades()
	if err != nil {
		log.Printf("upgrade reconcile: list active failed: %v", err)
		return
	}
	for i := range intents {
		s.advanceUpgrade(&intents[i])
	}
}

// advanceUpgrade 推进单个意图：读取 agent 当前版本/能力，经纯判定函数
// upgradeNextStatus 决定下一状态，条件 UPDATE 推进并审计。
func (s *Server) advanceUpgrade(u *storage.AgentUpgrade) {
	agent, err := s.db.GetAgent(u.AgentID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		log.Printf("upgrade reconcile: agent %s load failed: %v", u.AgentID, err)
		return
	}
	agentVersion := ""
	capOK := upgradeCapabilityOK(nil, u.RequiredCaps)
	if agent != nil {
		agentVersion = agent.Version
		capOK = upgradeCapabilityOK(agent.Capabilities, u.RequiredCaps)
	}

	now := time.Now()
	next, reason := upgradeNextStatus(u, agentVersion, s.upgradeHealthWindow(), now, capOK)
	if next == "" {
		return // 保持现状，等下一轮
	}
	hit, err := s.db.AdvanceUpgrade(u.ID, u.Status, next, reason, now)
	if err != nil {
		log.Printf("upgrade reconcile: advance %s failed: %v", u.ID, err)
		return
	}
	if hit == 0 {
		return // 已被并发推进（另一循环/测试注入），本轮放弃
	}
	s.audit.LogResource("cockpit-agent", audit.ActionAgentUpgrade, audit.ResourceAgentUpgrade,
		u.ID, map[string]interface{}{
			"agentId": u.AgentID, "from": u.Status, "to": next,
			"reason": reason, "agentVersion": agentVersion,
		}, "", "")
}
