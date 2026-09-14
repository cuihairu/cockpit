package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
)

// validateRemoteTarget 校验目标 host 是否被允许用于远控连接。
//
// 规则（按顺序短路）：
//  1. 配置显式启用 AllowArbitraryTarget -> 全部放行
//  2. host 命中 AllowedTargets 列表中的显式 host/IP 或 CIDR（端口不限） -> 放行
//  3. 否则拒绝
//
// 返回值：
//   - allow=true 时通过；allow=false 时拒绝，errMsg 给出可向客户端展示的原因
func (s *Server) validateRemoteTarget(host string) (allow bool, errMsg string) {
	if host == "" {
		return false, "missing target host"
	}

	cfg := s.remoteControlConfig()
	if cfg != nil && cfg.AllowArbitraryTarget {
		return true, ""
	}

	host = strings.TrimSpace(host)
	allowed := s.collectAllowedTargets()
	for _, a := range allowed {
		if targetMatchesAllowEntry(host, a) {
			return true, ""
		}
	}
	return false, fmt.Sprintf("target host %q is not allowed; add it to remote_control.allowed_targets (host/IP/CIDR) or enable remote_control.allow_arbitrary_target", host)
}

func (s *Server) validateRemoteTargetForAgent(agentID, host string, port int) (allow bool, errMsg string) {
	match, errMsg := s.matchRemoteEgress(agentID, host, port)
	return match != nil, errMsg
}

func targetMatchesAllowEntry(host, entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}

	if entry == host {
		return true
	}

	_, network, err := net.ParseCIDR(entry)
	if err != nil {
		return false
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return network.Contains(ip)
}

// remoteControlConfig 安全读取远控配置（容忍 nil）
func (s *Server) remoteControlConfig() *remoteControlConfigView {
	if s.cfg == nil || s.cfg.RemoteControl == nil {
		return nil
	}
	return &remoteControlConfigView{
		AllowArbitraryTarget: s.cfg.RemoteControl.AllowArbitraryTarget,
		AllowedTargets:       s.cfg.RemoteControl.AllowedTargets,
		EgressPolicies:       buildRemoteEgressPolicyViews(s.cfg.RemoteControl.EgressPolicies),
	}
}

// remoteControlConfigView 内部只读视图，避免直接暴露 *config.RemoteControlConfig
type remoteControlConfigView struct {
	AllowArbitraryTarget bool
	AllowedTargets       []string
	EgressPolicies       []*remoteEgressPolicyView
}

type remoteEgressPolicyView struct {
	AgentID        string
	AllowedTargets []string
	AllowedPorts   map[int]struct{}
}

type remoteEgressMatch struct {
	Mode    string
	AgentID string
	Port    int
}

// collectAllowedTargets 汇总配置白名单
func (s *Server) collectAllowedTargets() []string {
	cfg := s.remoteControlConfig()
	if cfg == nil {
		return nil
	}
	return cfg.AllowedTargets
}

func (s *Server) matchRemoteEgress(agentID, host string, port int) (*remoteEgressMatch, string) {
	if allow, errMsg := s.validateRemoteTarget(host); !allow {
		return nil, errMsg
	}

	cfg := s.remoteControlConfig()
	if cfg == nil || len(cfg.EgressPolicies) == 0 {
		return &remoteEgressMatch{Mode: "global-allow-list"}, ""
	}

	for _, policy := range cfg.EgressPolicies {
		if policy == nil || strings.TrimSpace(policy.AgentID) != strings.TrimSpace(agentID) {
			continue
		}
		if !policy.allowsTarget(host) {
			return nil, fmt.Sprintf("target host %q is not allowed for agent %q; add it to remote_control.egress[].allowed_targets", strings.TrimSpace(host), agentID)
		}
		if !policy.allowsPort(port) {
			return nil, fmt.Sprintf("target port %d is not allowed for agent %q; add it to remote_control.egress[].allowed_ports", port, agentID)
		}
		return &remoteEgressMatch{
			Mode:    "agent-egress",
			AgentID: policy.AgentID,
			Port:    port,
		}, ""
	}

	return nil, fmt.Sprintf("agent %q is not allowed for remote egress; add it to remote_control.egress", agentID)
}

func buildRemoteEgressPolicyViews(policies []*config.RemoteEgressPolicy) []*remoteEgressPolicyView {
	if len(policies) == 0 {
		return nil
	}

	result := make([]*remoteEgressPolicyView, 0, len(policies))
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		view := &remoteEgressPolicyView{
			AgentID:        strings.TrimSpace(policy.AgentID),
			AllowedTargets: policy.AllowedTargets,
			AllowedPorts:   make(map[int]struct{}, len(policy.AllowedPorts)),
		}
		for _, port := range policy.AllowedPorts {
			if port > 0 {
				view.AllowedPorts[port] = struct{}{}
			}
		}
		result = append(result, view)
	}
	return result
}

func (p *remoteEgressPolicyView) allowsTarget(host string) bool {
	if p == nil || len(p.AllowedTargets) == 0 {
		return false
	}
	for _, allowed := range p.AllowedTargets {
		if targetMatchesAllowEntry(host, allowed) {
			return true
		}
	}
	return false
}

func (p *remoteEgressPolicyView) allowsPort(port int) bool {
	if p == nil || port <= 0 || len(p.AllowedPorts) == 0 {
		return false
	}
	_, ok := p.AllowedPorts[port]
	return ok
}

func (m *remoteEgressMatch) summary() string {
	if m == nil {
		return ""
	}
	if m.Mode == "global-allow-list" {
		return "global-allow-list"
	}
	if m.Mode == "agent-egress" {
		return fmt.Sprintf("agent:%s port:%d", m.AgentID, m.Port)
	}
	return ""
}

// auditRemoteStart 记录远控会话开始审计日志
func (s *Server) auditRemoteStart(action, userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
	if s.audit == nil || details == nil {
		return
	}
	if err := s.audit.LogRemoteSession(action, userID, username, audit.StatusSuccess, ip, userAgent, details); err != nil {
		fmt.Printf("audit remote start failed: %v\n", err)
	}
}

func (s *Server) auditRemoteFailure(userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
	if s.audit == nil || details == nil {
		return
	}
	if err := s.audit.LogRemoteSession(audit.ActionRemoteStart, userID, username, audit.StatusFailure, ip, userAgent, details); err != nil {
		fmt.Printf("audit remote failure failed: %v\n", err)
	}
}

// auditRemoteEnd 记录远控会话结束审计日志
func (s *Server) auditRemoteEnd(userID, username, ip, userAgent string, details *audit.RemoteSessionDetails) {
	if s.audit == nil || details == nil {
		return
	}
	if err := s.audit.LogRemoteSession(audit.ActionRemoteEnd, userID, username, audit.StatusSuccess, ip, userAgent, details); err != nil {
		fmt.Printf("audit remote end failed: %v\n", err)
	}
}

// rejectTargetDenied 统一构造"目标未授权"的 HTTP 响应
func rejectTargetDenied(w http.ResponseWriter, reason string) {
	http.Error(w, reason, http.StatusForbidden)
}
