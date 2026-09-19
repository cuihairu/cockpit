package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Compose Stack API：REST 门面转发 Agent RPC（设计见 docs/guide/stack-deploy-design.md）。
//
//	GET    /api/stacks                                     聚合所有 agent 的 stack 列表（含离线缓存）
//	GET    /api/stacks/agents/{agentId}                    单 agent 的 stack 列表
//	GET    /api/stacks/agents/{agentId}/{name}             stack 状态明细
//	GET    /api/stacks/agents/{agentId}/{name}/compose     读取 compose/.env
//	PUT    /api/stacks/agents/{agentId}/{name}/compose     保存 compose/.env（agent 侧先校验）
//	POST   /api/stacks/agents/{agentId}/{name}/up|down     部署/停止（返回 taskId，异步）
//	GET    /api/stacks/agents/{agentId}/{name}/logs        compose logs
//	GET    /api/stacks/agents/{agentId}/tasks/{taskId}     任务状态轮询
//	DELETE /api/stacks/agents/{agentId}/{name}             删除 stack（异步：先 down 再删目录）
const stacksAPIPrefix = "/api/stacks/"

// stackTasksSegment 保留路由段：stack 名正则不匹配它，但解析时仍需先于 {name} 判断
const stackTasksSegment = "tasks"

// stackAggregateTimeout 聚合视图的整体等待预算（单 agent 的 CallAgent 内部超时 30s）
const stackAggregateTimeout = 10 * time.Second

func (s *Server) registerStacksAPI(mux *http.ServeMux) {
	mux.HandleFunc(stacksAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleStacks)(w, r)
	})
}

func (s *Server) handleStacks(w http.ResponseWriter, r *http.Request) {
	segments := splitStacksPath(r.URL.Path)
	if len(segments) == 0 {
		if r.Method == http.MethodGet {
			s.handleStacksAll(w, r)
			return
		}
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if segments[0] != "agents" || len(segments) < 2 {
		s.handleError(w, r, http.StatusBadRequest, "expected /api/stacks/agents/{agentId}/...")
		return
	}

	agentID, err := pathSegment(segments[1])
	if err != nil || agentID == "" {
		s.handleError(w, r, http.StatusBadRequest, "invalid agent id")
		return
	}

	// /api/stacks/agents/{agentId}
	if len(segments) == 2 {
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleAgentStacks(w, r, agentID)
		return
	}

	// /api/stacks/agents/{agentId}/tasks/{taskId}
	if segments[2] == stackTasksSegment && len(segments) == 4 {
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		taskID, err := pathSegment(segments[3])
		if err != nil || taskID == "" {
			s.handleError(w, r, http.StatusBadRequest, "invalid task id")
			return
		}
		s.handleStackTask(w, r, agentID, taskID)
		return
	}

	name, err := pathSegment(segments[2])
	if err != nil || name == "" {
		s.handleError(w, r, http.StatusBadRequest, "invalid stack name")
		return
	}

	if len(segments) == 3 {
		switch r.Method {
		case http.MethodGet:
			s.handleStackRPC(w, r, agentID, "stack.status", map[string]interface{}{"name": name}, "")
		case http.MethodDelete:
			s.handleStackRPC(w, r, agentID, "stack.remove", map[string]interface{}{"name": name}, audit.ActionStackRemove)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(segments) != 4 {
		s.handleError(w, r, http.StatusBadRequest, "unsupported stacks route")
		return
	}

	switch segments[3] {
	case "compose":
		if r.Method == http.MethodGet {
			s.handleStackRPC(w, r, agentID, "stack.file.get", map[string]interface{}{"name": name}, "")
		} else if r.Method == http.MethodPut {
			params, err := requestParams(r)
			if err != nil {
				s.handleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			params["name"] = name
			// 记 stack_update；agent 返回 created=true 时由 auditStackAction 升级为 stack_create
			s.handleStackRPC(w, r, agentID, "stack.file.save", params, audit.ActionStackUpdate)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "up":
		if r.Method == http.MethodPost {
			s.handleStackRPC(w, r, agentID, "stack.up", map[string]interface{}{"name": name}, audit.ActionStackUp)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "down":
		if r.Method == http.MethodPost {
			s.handleStackRPC(w, r, agentID, "stack.down", map[string]interface{}{"name": name}, audit.ActionStackDown)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "restart":
		if r.Method == http.MethodPost {
			s.handleStackRPC(w, r, agentID, "stack.restart", map[string]interface{}{"name": name}, audit.ActionStackRestart)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "pull":
		if r.Method == http.MethodPost {
			s.handleStackRPC(w, r, agentID, "stack.pull", map[string]interface{}{"name": name}, audit.ActionStackPull)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "history":
		if r.Method == http.MethodGet {
			s.handleStackHistory(w, r, agentID, name)
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "logs":
		if r.Method == http.MethodGet {
			params := map[string]interface{}{"name": name}
			setQueryString(params, r, "service")
			setQueryString(params, r, "tail")
			s.handleStackRPC(w, r, agentID, "stack.logs", params, "")
		} else {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		s.handleError(w, r, http.StatusBadRequest, "unsupported stack action")
	}
}

// requireStackAgent 校验 agent 存在且具备 docker 能力
func (s *Server) requireStackAgent(agentID string) bool {
	agent, ok := s.registry.Get(agentID)
	if !ok {
		return false
	}
	return agent.HasCapability("docker-api") || agent.HasCapability("docker")
}

// handleStackRPC 转发 RPC 并按 stack 语义映射错误码；
// auditAction 非空时记录审计（.env 内容与部署输出不入审计）。
func (s *Server) handleStackRPC(w http.ResponseWriter, r *http.Request, agentID, method string, params map[string]interface{}, auditAction string) {
	if !s.requireStackAgent(agentID) {
		s.handleError(w, r, http.StatusNotFound, "Agent not found or docker capability not available")
		return
	}

	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		status := http.StatusBadGateway
		if err == ErrAgentNotFound {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "busy") {
			status = http.StatusConflict
		} else if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "timeout") {
			status = http.StatusGatewayTimeout
		}
		s.handleError(w, r, status, err.Error())
		return
	}

	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "Invalid agent response")
		return
	}
	if rpcResp.Status == "error" {
		status := http.StatusBadGateway
		if strings.Contains(rpcResp.Error, "busy") {
			status = http.StatusConflict
		} else if strings.Contains(rpcResp.Error, "not found") {
			status = http.StatusNotFound
		}
		s.handleError(w, r, status, rpcResp.Error)
		return
	}

	if auditAction != "" {
		s.auditStackAction(r, auditAction, agentID, stackString(params, "name"), rpcResp.Data)
		// 启动类动作拿到 taskId 后记录部署历史并由后台跟踪终态
		if data, ok := rpcResp.Data.(map[string]interface{}); ok {
			if taskID := stackString(data, "taskId"); taskID != "" {
				s.startStackDeployment(agentID, stackString(params, "name"), auditAction, taskID)
			}
		}
	}
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// stackTrackingActions 会产生异步任务的启动类动作（需要记部署历史）
var stackTrackingActions = map[string]string{
	audit.ActionStackUp:      "up",
	audit.ActionStackDown:    "down",
	audit.ActionStackRestart: "restart",
	audit.ActionStackPull:    "pull",
	audit.ActionStackRemove:  "remove",
}

// stackTaskTrackInterval 后台跟踪任务终态的轮询间隔；
// stackTaskTrackTimeout 跟踪超时：超过后标记 failed（防止 goroutine 无限轮询）。
// 包级变量仅为测试可注入，默认值即生产取值。
var (
	stackTaskTrackInterval = 2 * time.Second
	stackTaskTrackTimeout  = 15 * time.Minute
)

// startStackDeployment 写入 running 历史记录并启动后台终态跟踪
func (s *Server) startStackDeployment(agentID, stackName, auditAction, taskID string) {
	action, ok := stackTrackingActions[auditAction]
	if !ok || stackName == "" {
		return
	}
	rec := &storage.StackDeployment{
		AgentID:   agentID,
		StackName: stackName,
		Action:    action,
		Status:    "running",
		TaskID:    taskID,
		StartedAt: time.Now().Unix(),
	}
	if err := s.db.CreateStackDeployment(rec); err != nil {
		fmt.Printf("create stack deployment %s/%s: %v\n", agentID, stackName, err)
		return
	}
	go s.trackStackTask(agentID, taskID, rec.ID)
}

// trackStackTask 轮询 agent 任务状态直到终态/超时/server 关闭，回填历史。
// agent 离线（ErrAgentNotFound）不终止：重连后能继续拿到真实终态。
func (s *Server) trackStackTask(agentID, taskID string, recID uint) {
	deadline := time.Now().Add(stackTaskTrackTimeout)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(stackTaskTrackInterval):
		}

		status, finishedAt, done := s.pollStackTask(agentID, taskID)
		if !done {
			if time.Now().After(deadline) {
				status, finishedAt = "failed", time.Now().Unix()
				done = true
			} else {
				continue
			}
		}
		if err := s.db.FinishStackDeployment(taskID, status, finishedAt); err != nil {
			fmt.Printf("finish stack deployment %s (rec %d): %v\n", taskID, recID, err)
		}
		return
	}
}

// pollStackTask 查询一次任务状态，返回 (status, finishedAt, 是否终态)。
// unknown task（agent 重启丢失任务态）视为 failed 终态。
func (s *Server) pollStackTask(agentID, taskID string) (string, int64, bool) {
	resp, err := s.CallAgent(agentID, "stack.task.get", map[string]interface{}{"taskId": taskID})
	if err == ErrAgentNotFound {
		// agent 离线：任务态未知，保持 running 等待重连
		return "", 0, false
	}
	if err != nil {
		return "", 0, false
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return "", 0, false
	}
	if rpcResp.Status == "error" {
		if strings.Contains(rpcResp.Error, "not found") {
			return "failed", time.Now().Unix(), true
		}
		return "", 0, false
	}
	data, ok := rpcResp.Data.(map[string]interface{})
	if !ok {
		return "", 0, false
	}
	switch stackString(data, "status") {
	case "success":
		return "success", stackInt64(data, "finishedAt"), true
	case "failed":
		return "failed", stackInt64(data, "finishedAt"), true
	}
	return "", 0, false
}

func stackInt64(m map[string]interface{}, key string) int64 {
	v, _ := m[key].(float64)
	return int64(v)
}

// handleStackHistory 部署历史（server 侧记录，agent 不在线时也可查）
func (s *Server) handleStackHistory(w http.ResponseWriter, r *http.Request, agentID, name string) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.db.ListStackDeployments(agentID, name, limit)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "query stack history: "+err.Error())
		return
	}
	// gorm Find 空表恒返回非 nil 空 slice，无需兜底
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"deployments": list})
}

// auditStackAction 记录 stack 审计事件
func (s *Server) auditStackAction(r *http.Request, action, agentID, name string, data interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	details := map[string]interface{}{"agentId": agentID}
	// 保存结果带 created 标记时区分 create/update 事件
	if m, ok := data.(map[string]interface{}); ok {
		if v, ok := m["created"].(bool); ok && v && action == audit.ActionStackUpdate {
			action = audit.ActionStackCreate
		}
	}
	details["result"] = data
	s.audit.LogResource(username, action, audit.ResourceStack, agentID+"/"+name, details, s.getClientIP(r), r.UserAgent())
}

func stackString(params map[string]interface{}, key string) string {
	v, _ := params[key].(string)
	return v
}

// handleStackTask 任务轮询（只读，不记审计）
func (s *Server) handleStackTask(w http.ResponseWriter, r *http.Request, agentID, taskID string) {
	s.handleStackRPC(w, r, agentID, "stack.task.get", map[string]interface{}{"taskId": taskID}, "")
}

// ============ 聚合视图 ============

// stackView 聚合视图条目（agent 列表原样透传 + server 端元数据）
type stackView struct {
	AgentID        string                   `json:"agentId"`
	AgentName      string                   `json:"agentName"`
	Name           string                   `json:"name"`
	Running        int                      `json:"running"`
	Total          int                      `json:"total"`
	LastAction     string                   `json:"lastAction"`
	LastStatus     string                   `json:"lastStatus"`
	LastDeployedAt int64                    `json:"lastDeployedAt"`
	Online         bool                     `json:"online"`
	Services       []map[string]interface{} `json:"services,omitempty"`
}

// handleAgentStacks 单 agent 的 stack 列表（同时刷新该 agent 的缓存）
func (s *Server) handleAgentStacks(w http.ResponseWriter, r *http.Request, agentID string) {
	if !s.requireStackAgent(agentID) {
		s.handleError(w, r, http.StatusNotFound, "Agent not found or docker capability not available")
		return
	}
	agent, _ := s.registry.Get(agentID)

	resp, err := s.CallAgent(agentID, "stack.list", map[string]interface{}{})
	if err != nil {
		status := http.StatusBadGateway
		if err == ErrAgentNotFound {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "timeout") {
			status = http.StatusGatewayTimeout
		}
		s.handleError(w, r, status, err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status != "success" {
		s.handleError(w, r, http.StatusBadGateway, "Invalid agent response")
		return
	}

	items, _ := rpcResp.Data.([]interface{})
	views := make([]stackView, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		views = append(views, s.stackViewFromItem(agent, item, true))
		s.cacheStack(agent.ID, item)
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"stacks": views,
		"info":   s.fetchStackInfo(agentID),
	})
}

// fetchStackInfo 拉取 agent 侧自检信息（目录可写性/compose 版本），失败返回 nil
func (s *Server) fetchStackInfo(agentID string) map[string]interface{} {
	resp, err := s.CallAgent(agentID, "stack.info", map[string]interface{}{})
	if err != nil {
		return nil
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status != "success" {
		return nil
	}
	if m, ok := rpcResp.Data.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// handleStacksAll 聚合所有具备 docker 能力 agent 的 stack 列表；
// 拉取失败的 agent 退回数据库缓存（online=false 灰态展示）。
func (s *Server) handleStacksAll(w http.ResponseWriter, r *http.Request) {
	agents := make([]*Agent, 0)
	for _, ag := range s.registry.List() {
		if ag.HasCapability("docker-api") || ag.HasCapability("docker") {
			agents = append(agents, ag)
		}
	}

	type fetchResult struct {
		agent *Agent
		items []map[string]interface{}
		info  map[string]interface{}
	}
	results := make(chan fetchResult, len(agents))
	var wg sync.WaitGroup
	for _, ag := range agents {
		wg.Add(1)
		go func(ag *Agent) {
			defer wg.Done()
			resp, err := s.CallAgent(ag.ID, "stack.list", map[string]interface{}{})
			if err != nil {
				results <- fetchResult{agent: ag}
				return
			}
			rpcResp, err := protocol.DecodeRPCResponse(resp)
			if err != nil || rpcResp.Status != "success" {
				results <- fetchResult{agent: ag}
				return
			}
			items := make([]map[string]interface{}, 0)
			if list, ok := rpcResp.Data.([]interface{}); ok {
				for _, raw := range list {
					if item, ok := raw.(map[string]interface{}); ok {
						items = append(items, item)
					}
				}
			}
			results <- fetchResult{agent: ag, items: items, info: s.fetchStackInfo(ag.ID)}
		}(ag)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	views := make([]stackView, 0)
	agentInfo := make(map[string]interface{})
	onlineAgents := make(map[string]bool)
	deadline := time.After(stackAggregateTimeout)
loop:
	for {
		select {
		case res, ok := <-results:
			if !ok {
				break loop
			}
			onlineAgents[res.agent.ID] = true
			if res.info != nil {
				agentInfo[res.agent.ID] = res.info
			}
			for _, item := range res.items {
				views = append(views, s.stackViewFromItem(res.agent, item, true))
				s.cacheStack(res.agent.ID, item)
			}
		case <-deadline:
			break loop
		}
	}

	// 离线/失败的 agent 用缓存补齐灰态
	cached, err := s.db.ListStacks()
	if err == nil {
		for _, st := range cached {
			if onlineAgents[st.AgentID] {
				continue
			}
			views = append(views, stackView{
				AgentID:        st.AgentID,
				AgentName:      s.stackAgentName(st.AgentID),
				Name:           st.Name,
				Running:        st.Running,
				Total:          st.Total,
				LastAction:     st.LastAction,
				LastStatus:     st.LastStatus,
				LastDeployedAt: st.LastDeployedAt,
				Online:         false,
			})
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"stacks":    views,
		"agentInfo": agentInfo,
	})
}

// stackViewFromItem 把 agent 的 stack.list 条目转为聚合视图
func (s *Server) stackViewFromItem(agent *Agent, item map[string]interface{}, online bool) stackView {
	view := stackView{
		AgentID:        agent.ID,
		AgentName:      agent.Hostname,
		Name:           mapString(item, "name"),
		Running:        mapInt(item, "running"),
		Total:          mapInt(item, "total"),
		LastAction:     mapString(item, "lastAction"),
		LastStatus:     mapString(item, "lastStatus"),
		LastDeployedAt: mapInt64(item, "lastDeployedAt"),
		Online:         online,
	}
	if services, ok := item["services"].([]interface{}); ok {
		view.Services = make([]map[string]interface{}, 0, len(services))
		for _, svc := range services {
			if m, ok := svc.(map[string]interface{}); ok {
				view.Services = append(view.Services, m)
			}
		}
	}
	return view
}

// cacheStack 把 agent 上报的 stack 条目写入缓存表
func (s *Server) cacheStack(agentID string, item map[string]interface{}) {
	err := s.db.UpsertStack(&storage.Stack{
		AgentID:        agentID,
		Name:           mapString(item, "name"),
		Running:        mapInt(item, "running"),
		Total:          mapInt(item, "total"),
		LastAction:     mapString(item, "lastAction"),
		LastStatus:     mapString(item, "lastStatus"),
		LastDeployedAt: mapInt64(item, "lastDeployedAt"),
	})
	if err != nil {
		fmt.Printf("cache stack %s/%s: %v\n", agentID, mapString(item, "name"), err)
	}
}

// stackAgentName 缓存条目里补 agent 主机名（agent 不在线时 registry 可能查不到）
func (s *Server) stackAgentName(agentID string) string {
	if agent, ok := s.registry.Get(agentID); ok {
		return agent.Hostname
	}
	if st, err := s.db.GetAgent(agentID); err == nil {
		return st.Hostname
	}
	return agentID
}

// splitStacksPath 按 "/" 切分 /api/stacks/ 后的路径。
// 兼容无尾斜杠的 /api/stacks（ServeMux 会 301 重定向，直连时也可解析）。
func splitStacksPath(path string) []string {
	rest := strings.TrimPrefix(path, stacksAPIPrefix)
	if rest == path {
		rest = strings.TrimPrefix(path, "/api/stacks")
	}
	trimmed := strings.Trim(rest, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func mapString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func mapInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func mapInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}
