package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
)

// validateRemoteTarget 校验目标 host 是否被允许用于远控连接。
//
// 规则（按顺序短路）：
//  1. 配置显式启用 AllowArbitraryTarget -> 全部放行
//  2. host 命中 AllowedTargets 列表（端口不限） -> 放行
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
		if a == host {
			return true, ""
		}
	}
	return false, fmt.Sprintf("target host %q is not allowed; add it to remote_control.allowed_targets or enable remote_control.allow_arbitrary_target", host)
}

// remoteControlConfig 安全读取远控配置（容忍 nil）
func (s *Server) remoteControlConfig() *remoteControlConfigView {
	if s.cfg == nil || s.cfg.RemoteControl == nil {
		return nil
	}
	return &remoteControlConfigView{
		AllowArbitraryTarget: s.cfg.RemoteControl.AllowArbitraryTarget,
		AllowedTargets:       s.cfg.RemoteControl.AllowedTargets,
	}
}

// remoteControlConfigView 内部只读视图，避免直接暴露 *config.RemoteControlConfig
type remoteControlConfigView struct {
	AllowArbitraryTarget bool
	AllowedTargets       []string
}

// collectAllowedTargets 汇总配置白名单
func (s *Server) collectAllowedTargets() []string {
	cfg := s.remoteControlConfig()
	if cfg == nil {
		return nil
	}
	return cfg.AllowedTargets
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
