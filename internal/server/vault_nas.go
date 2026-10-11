package server

import (
	"log"
	"strings"
	"time"
)

// nasVaultPrefix vault 条目前缀：nas/<name>（vault-design.md V6 第一波②）
const nasVaultPrefix = "nas/"

// vaultNasTargets 读 vault nas/ 条目，组装 agent 推送形状（与
// COCKPIT_NAS_TARGETS JSON 同形，rpc.NasTarget）。vault provider 未配置
// 返回 false（调用方区分「无 vault」与「vault 为空」）；单条读取失败降级跳过。
// 明文只在返回值内存态流转，不落日志。
func (s *Server) vaultNasTargets() ([]map[string]interface{}, bool) {
	if s.vaultProvider == nil {
		return nil, false
	}
	metas, err := s.vaultProvider.List(nasVaultPrefix)
	if err != nil {
		return nil, true
	}
	targets := make([]map[string]interface{}, 0, len(metas))
	for _, m := range metas {
		e, err := s.vaultProvider.Read(m.Path)
		if err != nil {
			log.Printf("vault: read %s failed for nas targets", m.Path)
			continue
		}
		t := map[string]interface{}{
			"name":     strings.TrimPrefix(e.Path, nasVaultPrefix),
			"type":     e.Meta["type"],
			"addr":     e.Meta["addr"],
			"username": e.Username,
			"password": e.Value,
		}
		if e.Meta["insecure_tls"] == "true" {
			t["insecureTls"] = true
		}
		targets = append(targets, t)
	}
	return targets, true
}

// pushNasTargets vault nas/ 条目推送到单 agent（nas.set_targets RPC）。
// agent 未注册 nas provider 时 RPC 报错降级 false（能力引导走注册响应）。
func (s *Server) pushNasTargets(agentID string) bool {
	targets, hasVault := s.vaultNasTargets()
	if !hasVault || len(targets) == 0 {
		return false
	}
	if _, err := s.CallAgent(agentID, "nas.set_targets", map[string]interface{}{"targets": targets}); err != nil {
		return false
	}
	return true
}

// pushNasTargetsAll 条目变更后对全部在线 agent 补推（fire-and-forget；
// 离线 agent 下次注册时经注册响应拿到最新目标）。
func (s *Server) pushNasTargetsAll() {
	for _, a := range s.registry.List() {
		s.pushNasTargets(a.ID)
	}
}

// registerRespPayload 组装注册响应载荷；vault 里有 nas/ 条目时随响应下发
// NAS 目标引导（vault-design.md V6），并在 agent 连接对象上打标（纳入巡检）。
func (s *Server) registerRespPayload(agent *Agent) map[string]interface{} {
	payload := map[string]interface{}{
		"status":            "accepted",
		"serverTime":        time.Now().Unix(),
		"heartbeatInterval": int(30),
	}
	targets, hasVault := s.vaultNasTargets()
	if hasVault && len(targets) > 0 {
		payload["nasTargets"] = targets
		agent.nasVaultTargets = true
	}
	return payload
}

// nasScanAgents NAS 巡检目标：声明 nas 能力或收到 vault 目标引导的 agent。
func (s *Server) nasScanAgents() []*Agent {
	var agents []*Agent
	for _, a := range s.registry.List() {
		if a.NasObservationReady() {
			agents = append(agents, a)
		}
	}
	return agents
}
