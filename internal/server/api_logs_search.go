package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 跨机日志联邦检索 API（设计见 docs/guide/logs-design.md M3，D11-D16）。
// 全局端点（不属单个 agent）：对全部在线且带 logs capability 的 agent
// 并行转发**既有** logs.query，按主机分组返回——agent 零改动、零存储，
// server 纯转发不落库（保持 D1/D8 哲学）。
//
//	POST /api/logs/search
//	     {type, source, tail≤500, since_minutes, grep, agents?[]}
//	 ←  {results: [{agentId, hostname, ok, truncated?, lines?, error?}],
//	     skipped: [{agentId, reason}]}
//
// 查询浏览性质不记审计（D9 同纪律）；单 agent 失败/超时不整体失败（D13）；
// 扇出是放大器，并发与在途双重闸门（D12）。

// logsSearchMaxTail 扇出收窄：每机 tail 上限（单机 query 是 2000，D11）
const logsSearchMaxTail = 500

// logsSearchMaxConcurrency 扇出并发上限（D12）
const logsSearchMaxConcurrency = 4

// logsSearchMaxInFlight 端点全局在途请求上限，超出 429（D12）
const logsSearchMaxInFlight = 2

// logsSearchInFlight 在途计数（包级，与 logsFollowers 同款先例）
var logsSearchInFlight atomic.Int32

// logsSearchPayload 跨机检索请求体（校验复用 M1 logsQueryPayload 规则）
type logsSearchPayload struct {
	Type         string   `json:"type"`
	Source       string   `json:"source"`
	Tail         int      `json:"tail"`
	SinceMinutes int      `json:"since_minutes"`
	Grep         string   `json:"grep"`
	Agents       []string `json:"agents"`
}

// logsSearchResult 单机结果（D14：按 agent 分组，不做跨机时间归并）
type logsSearchResult struct {
	AgentID   string `json:"agentId"`
	Hostname  string `json:"hostname"`
	OK        bool   `json:"ok"`
	Truncated bool   `json:"truncated,omitempty"`
	Lines     string `json:"lines,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleLogsSearch POST /api/logs/search
func (s *Server) handleLogsSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// 在途闸门：进入即计数，返回前必降（D12）
	if logsSearchInFlight.Add(1) > logsSearchMaxInFlight {
		logsSearchInFlight.Add(-1)
		s.handleError(w, r, http.StatusTooManyRequests, "too many concurrent searches")
		return
	}
	defer logsSearchInFlight.Add(-1)

	var p logsSearchPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	// 复用 M1 校验（type 枚举/源白名单/默认 tail/grep 规则），再叠加扇出收窄（D15/D11）
	q := logsQueryPayload{Type: p.Type, Source: p.Source, Tail: p.Tail,
		SinceMinutes: p.SinceMinutes, Grep: p.Grep}
	if err := validateLogsQuery(&q); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if q.Tail > logsSearchMaxTail {
		s.handleError(w, r, http.StatusBadRequest,
			fmt.Sprintf("tail out of range [1, %d] for cross-agent search", logsSearchMaxTail))
		return
	}
	for _, id := range p.Agents {
		if id == "" {
			s.handleError(w, r, http.StatusBadRequest, "agents must not contain empty ids")
			return
		}
	}

	// 目标筛选（D12）：在线且带 logs capability；agents 指定时取交集。
	// agent 断开即从注册表注销，缺席即离线（不区分 not-found）
	all := s.registry.List()
	inScope := func(a *Agent) bool {
		if !a.HasCapability("logs") {
			return false
		}
		return len(p.Agents) == 0 || containsAgentID(p.Agents, a.ID)
	}
	var targets []*Agent
	for _, a := range all {
		if inScope(a) {
			targets = append(targets, a)
		}
	}
	// 稳定输出：按 agentId 排序（registry.List 基于	map 遍历无序）
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	// skipped 归因（D12）：不可用的目标（离线 / 无 logs capability）
	var skipped []map[string]interface{}
	noteSkipped := func(agentID, reason string) {
		skipped = append(skipped, map[string]interface{}{"agentId": agentID, "reason": reason})
	}
	if len(p.Agents) > 0 {
		for _, id := range p.Agents {
			if a, ok := s.registry.Get(id); ok {
				if !a.HasCapability("logs") {
					noteSkipped(id, "no-logs")
				}
			} else {
				noteSkipped(id, "offline")
			}
		}
	} else {
		for _, a := range all {
			if !a.HasCapability("logs") {
				noteSkipped(a.ID, "no-logs")
			}
		}
	}

	// 扇出（D12/D13）：并发 ≤4；单机失败/超时降级为 ok=false，不整体失败；
	// 全部目标不可用 → 200 空结果 + skipped 说明
	queryParams := map[string]interface{}{
		"query": map[string]interface{}{
			"type": q.Type, "source": q.Source, "tail": q.Tail,
			"since_minutes": q.SinceMinutes, "grep": q.Grep,
		},
	}
	results := make([]logsSearchResult, len(targets))
	sem := make(chan struct{}, logsSearchMaxConcurrency)
	var wg sync.WaitGroup
	for i, a := range targets {
		wg.Add(1)
		go func(i int, a *Agent) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = s.searchOneAgent(a, queryParams)
		}(i, a)
	}
	wg.Wait()

	if skipped == nil {
		skipped = []map[string]interface{}{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"skipped": skipped,
	})
}

// searchOneAgent 单机检索：转发 logs.query，失败降级为 ok=false + error
func (s *Server) searchOneAgent(a *Agent, queryParams map[string]interface{}) logsSearchResult {
	res := logsSearchResult{AgentID: a.ID, Hostname: a.Hostname}
	msg, err := s.CallAgent(a.ID, "logs.query", queryParams)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	rpcResp, err := protocol.DecodeRPCResponse(msg)
	if err != nil {
		res.Error = "agent returned invalid response"
		return res
	}
	if rpcResp.Status == "error" {
		res.Error = rpcResp.Error
		if res.Error == "" {
			res.Error = "agent rejected the operation"
		}
		return res
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	res.OK = true
	res.Lines, _ = data["lines"].(string)
	res.Truncated, _ = data["truncated"].(bool)
	return res
}

// containsAgentID 简单包含判断（agents 子集量级小，不值得建 map）
func containsAgentID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
