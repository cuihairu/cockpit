package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 反向代理管理 API（设计见 docs/guide/proxy-design.md）。
// 挂在 /api/agents/{id}/proxy/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET    /api/agents/{id}/proxy/status        概览（nginx 版本/配置目录/reload 方式）
//	GET    /api/agents/{id}/proxy/sites         列 cockpit 名下站点
//	GET    /api/agents/{id}/proxy/sites/{name}  查看站点与渲染后的配置全文
//	PUT    /api/agents/{id}/proxy/sites/{name}  应用站点（审计 proxy_apply）
//	DELETE /api/agents/{id}/proxy/sites/{name}  删除站点（审计 proxy_delete）
//
// server 纯转发不落库（D10）：站点状态以 agent 侧片段文件为唯一事实源。
// 参数校验与 agent 同规则（双端防御）；nginx -t 的 stderr 摘要原样透传——
// 那是用户修正输入的关键信息。

// 与 agent 侧 ProxySite.validate 完全一致的校验规则（双端同规则）
var (
	proxySiteNameRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	proxySiteDomainRe   = regexp.MustCompile(`^[A-Za-z0-9*.\-]{1,253}$`)
	proxySiteUpstreamRe = regexp.MustCompile(`^[A-Za-z0-9.:\-]{1,253}$`)
)

// proxyMaxExtra extra 指令直通上限（与 agent nginxMaxExtra 一致）
const proxyMaxExtra = 4 * 1024

// proxySitePayload 应用站点的请求体
type proxySitePayload struct {
	Name        string   `json:"name"`
	ServerNames []string `json:"serverNames"`
	Upstream    string   `json:"upstream"`
	Scheme      string   `json:"scheme"`
	TLSCert     string   `json:"tlsCert"`
	TLSKey      string   `json:"tlsKey"`
	Websocket   bool     `json:"websocket"`
	Extra       string   `json:"extra"`
}

// validateProxySite 校验站点参数（与 agent validate() 同规则）
func validateProxySite(s *proxySitePayload) error {
	if !proxySiteNameRe.MatchString(s.Name) {
		return fmt.Errorf("invalid site name %q", s.Name)
	}
	if len(s.ServerNames) == 0 || len(s.ServerNames) > 16 {
		return fmt.Errorf("serverNames must contain 1-16 entries")
	}
	for _, d := range s.ServerNames {
		if !proxySiteDomainRe.MatchString(d) {
			return fmt.Errorf("invalid server name %q", d)
		}
	}
	if !proxySiteUpstreamRe.MatchString(s.Upstream) {
		return fmt.Errorf("invalid upstream %q", s.Upstream)
	}
	switch s.Scheme {
	case "http":
	case "https":
		if !strings.HasPrefix(s.TLSCert, "/") || !strings.HasPrefix(s.TLSKey, "/") {
			return fmt.Errorf("https requires absolute tlsCert and tlsKey paths")
		}
	default:
		return fmt.Errorf("scheme must be http or https")
	}
	if len(s.Extra) > proxyMaxExtra {
		return fmt.Errorf("extra directives too large (max %d bytes)", proxyMaxExtra)
	}
	return nil
}

// proxyRPCPrefix 按 agent capability 选 RPC 方法前缀（M2 D16）：同一套
// REST 端点对后端透明。一机双后端时 nginx 优先（存量语义不变，traefik
// 是升级路径）；都无（旧 agent 异常形态）回退 nginx 保持兼容。
func (s *Server) proxyRPCPrefix(agentID string) string {
	if a, ok := s.registry.Get(agentID); ok {
		for _, c := range a.Capabilities {
			if c.Type == "nginx-proxy" {
				return "nginx."
			}
			if c.Type == "traefik-proxy" {
				return "traefik."
			}
		}
	}
	return "nginx."
}

// handleAgentProxyAPI 分发 /api/agents/{id}/proxy/{rest}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/proxy/sites"）
func (s *Server) handleAgentProxyAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/proxy/"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	sub := rest[idx+len(suffix):]
	if agentID == "" || sub == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if _, ok := s.registry.Get(agentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}

	switch {
	case sub == "status" && r.Method == http.MethodGet:
		s.forwardProxyRPC(w, r, agentID, s.proxyRPCPrefix(agentID)+"status", nil, "", nil)
	case sub == "sites" && r.Method == http.MethodGet:
		s.forwardProxyRPC(w, r, agentID, s.proxyRPCPrefix(agentID)+"sites", nil, "", nil)
	case sub == "sites/" || strings.HasPrefix(sub, "sites/"):
		name := strings.TrimPrefix(sub, "sites/")
		s.handleProxySite(w, r, agentID, name)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleProxySite 处理 sites/{name} 三个端点
func (s *Server) handleProxySite(w http.ResponseWriter, r *http.Request, agentID, name string) {
	if !proxySiteNameRe.MatchString(name) {
		s.handleError(w, r, http.StatusBadRequest, "invalid site name")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.forwardProxyRPC(w, r, agentID, s.proxyRPCPrefix(agentID)+"site.get",
			map[string]interface{}{"name": name}, name, nil)
	case http.MethodPut:
		var payload proxySitePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		// URL 与 body 的 name 必须一致（防两处说法不一）
		if payload.Name != name {
			s.handleError(w, r, http.StatusBadRequest, "site name in URL and body must match")
			return
		}
		if err := validateProxySite(&payload); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		details := map[string]interface{}{
			"serverNames": payload.ServerNames,
			"upstream":    payload.Upstream,
			"scheme":      payload.Scheme,
		}
		s.forwardProxyRPC(w, r, agentID, s.proxyRPCPrefix(agentID)+"site.apply",
			map[string]interface{}{"site": payload}, name, details)
	case http.MethodDelete:
		s.forwardProxyRPC(w, r, agentID, s.proxyRPCPrefix(agentID)+"site.delete",
			map[string]interface{}{"name": name}, name,
			map[string]interface{}{})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// forwardProxyRPC 转发 RPC 并透传结果；method 前缀由 agent capability 决定
// （nginx.* / traefik.*），server 不感知后端差异（M2 D16）
func (s *Server) forwardProxyRPC(w http.ResponseWriter, r *http.Request, agentID, method string,
	params map[string]interface{}, siteName string, details map[string]interface{}) {

	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "agent returned invalid response")
		return
	}
	if rpcResp.Status == "error" {
		// agent 的错误信息（如 nginx -t stderr 摘要）是用户修正输入的关键，原样透传
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}

	// 变更类操作审计：记 name/域名/上游（证书只有路径引用，无内容）
	if details != nil {
		username := "unknown"
		if userInfo, ok := auth.GetUserFromContext(r); ok {
			username = userInfo.Username
		}
		details["agent"] = agentID
		action := audit.ActionProxyApply
		if r.Method == http.MethodDelete {
			action = audit.ActionProxyDelete
		}
		s.audit.LogResource(username, action, audit.ResourceProxySite, siteName,
			details, s.getClientIP(r), r.UserAgent())
	}
	s.writeJSON(w, http.StatusOK, data)
}
