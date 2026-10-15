package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// stacks webhook 触发部署（stack-deploy-design.md P2 ②）：
//
//	POST /api/stacks/webhook   头 X-Cockpit-Token == 配置 stacks.webhook_token
//
// git 仓库推送后由 forge 回调，无法走登录态——独立精确路由（先于
// /api/stacks/ 认证前缀注册，ServeMux 最长匹配胜出）。令牌空 = 端点关闭
// （默认关，显式配置开启，同 notification 红线口径）。webhook 只触发
// 「拉取已接入 origin + 部署改动 stack」，不接受 remote 参数（不重定向 origin）。
// body 可选 {"agentId":"...","branch":"..."}：指定单 agent，缺省扇出全部在线
// agent（未接入仓库的 agent RPC 报 remote required → 失败仅落日志）。
// 执行异步：响应即返回受理结果，部署结果看部署历史与审计（username=webhook）。

// handleStackWebhook webhook 入口（不走认证中间件，token 即凭据）
func (s *Server) handleStackWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token := ""
	if s.cfg != nil && s.cfg.Stacks != nil {
		token = s.cfg.Stacks.WebhookToken
	}
	if token == "" {
		s.handleError(w, r, http.StatusNotFound, "stack webhook disabled")
		return
	}
	got := r.Header.Get("X-Cockpit-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		fmt.Printf("stack webhook rejected: bad token from %s\n", s.getClientIP(r))
		s.handleError(w, r, http.StatusUnauthorized, "invalid token")
		return
	}

	// body 可选：空 body = 全量扇出 + 当前分支
	params := map[string]interface{}{}
	if r.Body != nil {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body != nil {
			params = body
		}
	}
	// remote 一律不接受：origin 以 agent 侧 .git/config 为准，
	// webhook 不重定向拉取来源（防注入内容改写部署源）
	delete(params, "remote")

	rpcParams := map[string]interface{}{}
	if b := stackString(params, "branch"); b != "" {
		rpcParams["branch"] = b
	}

	var targets []string
	if agentID := stackString(params, "agentId"); agentID != "" {
		if _, ok := s.registry.Get(agentID); !ok {
			s.handleError(w, r, http.StatusNotFound, "Agent not found")
			return
		}
		targets = []string{agentID}
	} else {
		for _, a := range s.registry.List() {
			if a.IsOnline(s.registry.timeouts.Heartbeat) {
				targets = append(targets, a.ID)
			}
		}
	}

	triggered := make([]string, 0, len(targets))
	for _, agentID := range targets {
		triggered = append(triggered, agentID)
		s.goBg(func() { s.runStackWebhookDeploy(agentID, rpcParams) })
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"triggered": triggered})
}

// runStackWebhookDeploy 单 agent 的异步拉取+部署：结果落审计与部署历史
func (s *Server) runStackWebhookDeploy(agentID string, params map[string]interface{}) {
	resp, err := s.CallAgent(agentID, "stack.git.deploy", params)
	if err != nil {
		fmt.Printf("stack webhook deploy %s: %v\n", agentID, err)
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		msg := "invalid agent response"
		if err == nil {
			msg = rpcResp.Error
		}
		fmt.Printf("stack webhook deploy %s: %s\n", agentID, msg)
		return
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	s.audit.LogResource("webhook", audit.ActionStackWebhookDeploy, audit.ResourceStack, agentID,
		map[string]interface{}{"agentId": agentID, "result": data}, "", "")
	for _, e := range stackDeployedEntries(data) {
		s.startStackDeployment(agentID, e.name, audit.ActionStackWebhookDeploy, e.taskID)
	}
}

// stackDeployedEntry result.deployed 里的一条 (stack, taskId)
type stackDeployedEntry struct {
	name   string
	taskID string
}

// stackDeployedEntries 取 result.deployed 的 (stack, taskId) 对
func stackDeployedEntries(data map[string]interface{}) []stackDeployedEntry {
	out := []stackDeployedEntry{}
	raw, ok := data["deployed"].([]interface{})
	if !ok {
		return out
	}
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stackString(m, "stack")
		taskID := stackString(m, "taskId")
		if name != "" && taskID != "" {
			out = append(out, stackDeployedEntry{name: name, taskID: taskID})
		}
	}
	return out
}
